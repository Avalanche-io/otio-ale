// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

package ale

import (
	"fmt"
	"io"
	"strings"

	"github.com/mrjoshuak/gotio/opentimelineio"
)

// Encoder writes OTIO timelines as ALE files
type Encoder struct {
	w         io.Writer
	fps       float64
	dropFrame bool
	columns   []string
}

// EncoderOption configures an Encoder
type EncoderOption func(*Encoder)

// WithEncoderFPS sets the frame rate for the encoder
func WithEncoderFPS(fps float64) EncoderOption {
	return func(e *Encoder) {
		e.fps = fps
	}
}

// WithEncoderDropFrame sets whether to use drop frame timecode
func WithEncoderDropFrame(dropFrame bool) EncoderOption {
	return func(e *Encoder) {
		e.dropFrame = dropFrame
	}
}

// WithColumns sets the columns to include in the ALE file
func WithColumns(columns []string) EncoderOption {
	return func(e *Encoder) {
		e.columns = columns
	}
}

// NewEncoder creates a new ALE encoder
func NewEncoder(w io.Writer, opts ...EncoderOption) *Encoder {
	e := &Encoder{
		w:         w,
		fps:       DefaultFPS,
		dropFrame: false,
		columns:   nil, // Will be determined automatically based on timeline content
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Encode writes an OTIO Timeline as an ALE file
func (e *Encoder) Encode(timeline *opentimelineio.Timeline) error {
	if timeline == nil {
		return fmt.Errorf("timeline cannot be nil")
	}

	aleFile, err := e.timelineToALE(timeline)
	if err != nil {
		return fmt.Errorf("failed to convert timeline to ALE: %w", err)
	}

	return e.writeALE(aleFile)
}

// timelineToALE converts an OTIO Timeline to an ALEFile structure
func (e *Encoder) timelineToALE(timeline *opentimelineio.Timeline) (*ALEFile, error) {
	aleFile := NewALEFile()

	// Get clips first to infer video format
	clips := timeline.FindClips(nil, false)

	// Set headers
	aleFile.Headers[HeaderFieldDelim] = DefaultFieldDelim
	aleFile.Headers[HeaderAudioFormat] = "48kHz"
	aleFile.Headers[HeaderFPS] = fmt.Sprintf("%.2f", e.fps)

	// Infer video format from clip metadata
	videoFormat := e.inferVideoFormat(clips)
	aleFile.Headers[HeaderVideoFormat] = videoFormat

	// Determine columns from clips
	columns := e.determineColumns(timeline)
	aleFile.Columns = columns

	// Convert clips to rows
	for _, clip := range clips {
		row, err := e.clipToRow(clip, columns)
		if err != nil {
			return nil, fmt.Errorf("failed to convert clip '%s' to row: %w", clip.Name(), err)
		}
		aleFile.Rows = append(aleFile.Rows, row)
	}

	return aleFile, nil
}

// inferVideoFormat infers the Avid video format from clip metadata
func (e *Encoder) inferVideoFormat(clips []*opentimelineio.Clip) string {
	maxWidth := 0
	maxHeight := 0

	// Look for clips with "Image Size" metadata
	for _, clip := range clips {
		metadata := clip.Metadata()
		if metadata == nil {
			continue
		}

		// Check in ALE metadata
		if aleData, ok := metadata["ALE"]; ok {
			if aleMap, ok := aleData.(map[string]interface{}); ok {
				if imageSize, ok := aleMap["Image Size"]; ok {
					if imageSizeStr, ok := imageSize.(string); ok {
						w, h, ok := parseImageSize(imageSizeStr)
						if ok {
							if h > maxHeight {
								maxHeight = h
								maxWidth = w
							}
						}
					}
				}
			}
		}
	}

	// If we found dimensions, infer format
	if maxHeight > 0 {
		return videoFormatFromDimensions(maxWidth, maxHeight)
	}

	// Default to 1080
	return "1080"
}

// determineColumns determines which columns to include based on the timeline content
func (e *Encoder) determineColumns(timeline *opentimelineio.Timeline) []string {
	if len(e.columns) > 0 {
		return e.columns
	}

	// Start with essential columns
	cols := []string{ColumnName, ColumnStart, ColumnEnd, ColumnDuration}

	// Check if we have audio or video tracks
	hasVideo := len(timeline.VideoTracks()) > 0
	hasAudio := len(timeline.AudioTracks()) > 0

	if hasVideo || hasAudio {
		cols = append(cols, ColumnTracks)
	}

	// Track which extra columns we've seen
	extraColumns := make(map[string]bool)

	// Add source file if any clip has a media reference
	clips := timeline.FindClips(nil, false)
	for _, clip := range clips {
		ref := clip.MediaReference()
		if ref != nil {
			extRef, ok := ref.(*opentimelineio.ExternalReference)
			if ok && extRef.TargetURL() != "" {
				if !extraColumns[ColumnSourceFile] {
					extraColumns[ColumnSourceFile] = true
				}
			}
		}

		// Scan clip metadata for ALE columns to preserve
		metadata := clip.Metadata()
		if aleData, ok := metadata["ALE"]; ok {
			if aleMap, ok := aleData.(map[string]interface{}); ok {
				for key := range aleMap {
					extraColumns[key] = true
				}
			}
		}

		// Check for CDL metadata to add ASC_SOP and ASC_SAT columns
		if cdlData, ok := metadata["cdl"]; ok {
			if cdl, ok := cdlData.(*CDLData); ok {
				if cdl.ASCSOP != nil {
					extraColumns["ASC_SOP"] = true
				}
				if cdl.ASCSat != nil {
					extraColumns["ASC_SAT"] = true
				}
			}
		}
	}

	// Add extra columns in sorted order for consistency
	var extraCols []string
	for col := range extraColumns {
		extraCols = append(extraCols, col)
	}
	// Sort alphabetically
	for i := 0; i < len(extraCols); i++ {
		for j := i + 1; j < len(extraCols); j++ {
			if extraCols[i] > extraCols[j] {
				extraCols[i], extraCols[j] = extraCols[j], extraCols[i]
			}
		}
	}

	cols = append(cols, extraCols...)

	return cols
}

// clipToRow converts an OTIO Clip to an ALE row
func (e *Encoder) clipToRow(clip *opentimelineio.Clip, columns []string) (map[string]string, error) {
	row := make(map[string]string)

	// Get source range
	sourceRange := clip.SourceRange()
	if sourceRange == nil {
		// Try to get available range from media reference
		if ref := clip.MediaReference(); ref != nil {
			ar := ref.AvailableRange()
			sourceRange = ar
		}
	}

	// Get metadata once
	metadata := clip.Metadata()

	// Fill in column values
	for _, col := range columns {
		switch col {
		case ColumnName:
			row[col] = clip.Name()

		case ColumnStart:
			if sourceRange != nil {
				startTime := sourceRange.StartTime()
				tc, err := formatTimecode(startTime, e.fps, e.dropFrame)
				if err != nil {
					return nil, fmt.Errorf("failed to format start timecode: %w", err)
				}
				row[col] = tc
			}

		case ColumnEnd:
			if sourceRange != nil {
				endTime := sourceRange.EndTimeExclusive()
				tc, err := formatTimecode(endTime, e.fps, e.dropFrame)
				if err != nil {
					return nil, fmt.Errorf("failed to format end timecode: %w", err)
				}
				row[col] = tc
			}

		case ColumnDuration:
			if sourceRange != nil {
				duration := sourceRange.Duration()
				row[col] = formatFrameNumber(duration, e.fps)
			}

		case ColumnTracks:
			// Determine if this is video, audio, or both
			// For now, assume video
			row[col] = "V"

		case ColumnSourceFile, ColumnTape:
			if ref := clip.MediaReference(); ref != nil {
				if extRef, ok := ref.(*opentimelineio.ExternalReference); ok {
					row[col] = extRef.TargetURL()
				}
			}

		case "ASC_SOP":
			// Check for CDL metadata
			if metadata != nil {
				if cdlData, ok := metadata["cdl"]; ok {
					if cdl, ok := cdlData.(*CDLData); ok {
						if cdl.ASCSOP != nil {
							row[col] = formatASCSOP(cdl.ASCSOP)
						}
					}
				}
			}

		case "ASC_SAT":
			// Check for CDL metadata
			if metadata != nil {
				if cdlData, ok := metadata["cdl"]; ok {
					if cdl, ok := cdlData.(*CDLData); ok {
						if cdl.ASCSat != nil {
							row[col] = fmt.Sprintf("%.1f", *cdl.ASCSat)
						}
					}
				}
			}

		default:
			// Check clip metadata["ALE"] for custom columns
			if metadata != nil {
				if aleData, ok := metadata["ALE"]; ok {
					if aleMap, ok := aleData.(map[string]interface{}); ok {
						if value, ok := aleMap[col]; ok {
							if strValue, ok := value.(string); ok {
								row[col] = strValue
							} else {
								row[col] = fmt.Sprintf("%v", value)
							}
						}
					}
				}
			}
		}
	}

	return row, nil
}

// writeALE writes the ALEFile structure to the output writer
func (e *Encoder) writeALE(aleFile *ALEFile) error {
	var lines []string

	// Write Heading section
	lines = append(lines, HeaderHeading)
	for key, value := range aleFile.Headers {
		lines = append(lines, fmt.Sprintf("%s\t%s", key, value))
	}
	lines = append(lines, "")

	// Write Column section
	lines = append(lines, HeaderColumn)
	lines = append(lines, joinTabs(aleFile.Columns))
	lines = append(lines, "")

	// Write Data section
	lines = append(lines, HeaderData)
	for _, row := range aleFile.Rows {
		values := make([]string, len(aleFile.Columns))
		for i, col := range aleFile.Columns {
			values[i] = row[col]
		}
		lines = append(lines, joinTabs(values))
	}

	// Write all lines to the writer
	output := strings.Join(lines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	_, err := e.w.Write([]byte(output))
	return err
}
