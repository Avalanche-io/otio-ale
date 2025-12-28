// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

package ale

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/Avalanche-io/gotio/opentime"
	"github.com/Avalanche-io/gotio"
)

// Decoder reads and decodes ALE files into OTIO timelines
type Decoder struct {
	r              io.Reader
	fps            float64
	nameColumnKey  string
	dropFrame      bool
}

// DecoderOption configures a Decoder
type DecoderOption func(*Decoder)

// WithFPS sets the frame rate for the decoder
func WithFPS(fps float64) DecoderOption {
	return func(d *Decoder) {
		d.fps = fps
	}
}

// WithNameColumn sets the column name to use for clip names
func WithNameColumn(key string) DecoderOption {
	return func(d *Decoder) {
		d.nameColumnKey = key
	}
}

// WithDropFrame sets whether to use drop frame timecode
func WithDropFrame(dropFrame bool) DecoderOption {
	return func(d *Decoder) {
		d.dropFrame = dropFrame
	}
}

// NewDecoder creates a new ALE decoder
func NewDecoder(r io.Reader, opts ...DecoderOption) *Decoder {
	d := &Decoder{
		r:             r,
		fps:           DefaultFPS,
		nameColumnKey: ColumnName,
		dropFrame:     false,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Decode parses an ALE file and returns an OTIO Timeline
func (d *Decoder) Decode() (*gotio.Timeline, error) {
	aleFile, err := d.parseALE()
	if err != nil {
		return nil, fmt.Errorf("failed to parse ALE: %w", err)
	}

	// Extract FPS from headers if present
	if fpsStr, ok := aleFile.Headers[HeaderFPS]; ok {
		if fps, err := parseFPS(fpsStr); err == nil {
			d.fps = fps
			d.dropFrame = isDropFrame(fps)
		}
	}

	return d.aleToTimeline(aleFile)
}

// parseALE reads and parses the ALE file structure
func (d *Decoder) parseALE() (*ALEFile, error) {
	aleFile := NewALEFile()
	scanner := bufio.NewScanner(d.r)

	// Parse heading section
	inHeading := false
	inData := false
	var columns []string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines
		if trimmed == "" {
			continue
		}

		// Check for section markers
		if strings.HasPrefix(trimmed, HeaderHeading) {
			inHeading = true
			continue
		}

		if strings.HasPrefix(trimmed, HeaderColumn) {
			inHeading = false
			// The next line should be column headers
			if scanner.Scan() {
				columnLine := scanner.Text()
				columns = splitTabs(columnLine)
				// Trim whitespace from column names
				for i, col := range columns {
					columns[i] = strings.TrimSpace(col)
				}
				aleFile.Columns = columns
			}
			continue
		}

		if strings.HasPrefix(trimmed, HeaderData) {
			inData = true
			continue
		}

		// Parse heading section
		if inHeading {
			parts := splitTabs(line)
			if len(parts) >= 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				aleFile.Headers[key] = value
			}
			continue
		}

		// Parse data rows
		if inData && len(columns) > 0 {
			values := splitTabs(line)
			if len(values) > 0 {
				row := make(map[string]string)
				for i, col := range columns {
					if i < len(values) {
						row[col] = strings.TrimSpace(values[i])
					} else {
						row[col] = ""
					}
				}
				aleFile.Rows = append(aleFile.Rows, row)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading ALE file: %w", err)
	}

	return aleFile, nil
}

// aleToTimeline converts an ALEFile structure to an OTIO Timeline
func (d *Decoder) aleToTimeline(aleFile *ALEFile) (*gotio.Timeline, error) {
	if len(aleFile.Rows) == 0 {
		return nil, fmt.Errorf("no data rows in ALE file")
	}

	// Create timeline
	timeline := gotio.NewTimeline(
		"ALE Timeline",
		nil,
		nil,
	)

	// Check if we have a Tracks column to determine track types
	hasTracksColumn := false
	for _, col := range aleFile.Columns {
		if col == ColumnTracks {
			hasTracksColumn = true
			break
		}
	}

	if hasTracksColumn {
		// Parse tracks from Tracks column - group clips by track type
		trackMap := make(map[string]*gotio.Track)

		for i, row := range aleFile.Rows {
			clip, err := d.rowToClip(row, i)
			if err != nil {
				return nil, fmt.Errorf("failed to convert row %d to clip: %w", i, err)
			}
			if clip == nil {
				continue
			}

			// Get track info from Tracks column
			tracksValue := row[ColumnTracks]
			trackKind := d.parseTrackKind(tracksValue)

			// Create track key (e.g., "V1", "A1", "VA1")
			trackKey := tracksValue
			if trackKey == "" {
				trackKey = "V" // Default to video
			}

			// Get or create track
			track, exists := trackMap[trackKey]
			if !exists {
				trackName := trackKey
				track = gotio.NewTrack(
					trackName,
					nil,
					trackKind,
					nil,
					nil,
				)
				trackMap[trackKey] = track
			}

			if err := track.AppendChild(clip); err != nil {
				return nil, fmt.Errorf("failed to append clip to track: %w", err)
			}
		}

		// Add all tracks to timeline in a consistent order
		// Sort track keys for consistent output
		var trackKeys []string
		for key := range trackMap {
			trackKeys = append(trackKeys, key)
		}
		// Sort: V before A, then by number
		sortTrackKeys(trackKeys)

		for _, key := range trackKeys {
			if err := timeline.Tracks().AppendChild(trackMap[key]); err != nil {
				return nil, fmt.Errorf("failed to add track to timeline: %w", err)
			}
		}
	} else {
		// No Tracks column - create a single video track like before
		videoTrack := gotio.NewTrack(
			"Video",
			nil,
			gotio.TrackKindVideo,
			nil,
			nil,
		)

		for i, row := range aleFile.Rows {
			clip, err := d.rowToClip(row, i)
			if err != nil {
				return nil, fmt.Errorf("failed to convert row %d to clip: %w", i, err)
			}
			if clip != nil {
				if err := videoTrack.AppendChild(clip); err != nil {
					return nil, fmt.Errorf("failed to append clip to track: %w", err)
				}
			}
		}

		if err := timeline.Tracks().AppendChild(videoTrack); err != nil {
			return nil, fmt.Errorf("failed to add track to timeline: %w", err)
		}
	}

	return timeline, nil
}

// parseTrackKind determines the track kind from a Tracks column value
func (d *Decoder) parseTrackKind(tracksValue string) string {
	// Tracks column can be: V, A, VA, V1, A1, VA1, etc.
	tracksValue = strings.TrimSpace(strings.ToUpper(tracksValue))

	if strings.Contains(tracksValue, "V") && strings.Contains(tracksValue, "A") {
		// Both video and audio - use video for primary
		return gotio.TrackKindVideo
	} else if strings.Contains(tracksValue, "A") {
		return gotio.TrackKindAudio
	}

	// Default to video
	return gotio.TrackKindVideo
}

// sortTrackKeys sorts track keys: V before A, then by number
func sortTrackKeys(keys []string) {
	// Simple bubble sort for track keys
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if compareTrackKeys(keys[i], keys[j]) > 0 {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
}

// compareTrackKeys compares two track keys for sorting
func compareTrackKeys(a, b string) int {
	// V comes before A, VA comes before both
	aHasV := strings.Contains(a, "V")
	aHasA := strings.Contains(a, "A")
	bHasV := strings.Contains(b, "V")
	bHasA := strings.Contains(b, "A")

	// Both have V and A
	if aHasV && aHasA && bHasV && bHasA {
		return strings.Compare(a, b)
	}

	// Only a has both
	if aHasV && aHasA {
		return -1
	}

	// Only b has both
	if bHasV && bHasA {
		return 1
	}

	// V before A
	if aHasV && bHasA {
		return -1
	}
	if aHasA && bHasV {
		return 1
	}

	// Both same type, compare lexically
	return strings.Compare(a, b)
}

// rowToClip converts an ALE row to an OTIO Clip
func (d *Decoder) rowToClip(row map[string]string, index int) (*gotio.Clip, error) {
	// Get clip name
	name := row[d.nameColumnKey]
	if name == "" {
		name = fmt.Sprintf("Clip %d", index+1)
	}

	// Parse timecodes
	var sourceRange *opentime.TimeRange
	startTC := row[ColumnStart]
	endTC := row[ColumnEnd]
	durationStr := row[ColumnDuration]

	if startTC != "" && endTC != "" {
		// Parse start and end timecodes
		startTime, err := parseTimecode(startTC, d.fps)
		if err != nil {
			return nil, fmt.Errorf("invalid start timecode '%s': %w", startTC, err)
		}

		endTime, err := parseTimecode(endTC, d.fps)
		if err != nil {
			return nil, fmt.Errorf("invalid end timecode '%s': %w", endTC, err)
		}

		duration := opentime.DurationFromStartEndTime(startTime, endTime)
		sourceRange = &opentime.TimeRange{}
		*sourceRange = opentime.NewTimeRange(startTime, duration)
	} else if durationStr != "" {
		// Parse duration
		duration, err := parseFrameNumber(durationStr, d.fps)
		if err != nil {
			// Try parsing as timecode
			duration, err = parseTimecode(durationStr, d.fps)
			if err != nil {
				return nil, fmt.Errorf("invalid duration '%s': %w", durationStr, err)
			}
		}

		startTime := opentime.NewRationalTime(0, d.fps)
		if startTC != "" {
			startTime, err = parseTimecode(startTC, d.fps)
			if err != nil {
				return nil, fmt.Errorf("invalid start timecode '%s': %w", startTC, err)
			}
		}
		sourceRange = &opentime.TimeRange{}
		*sourceRange = opentime.NewTimeRange(startTime, duration)
	}

	// Create media reference
	var mediaRef gotio.MediaReference
	sourceFile := row[ColumnSourceFile]
	if sourceFile == "" {
		sourceFile = row[ColumnTape]
	}

	if sourceFile != "" {
		mediaRef = gotio.NewExternalReference(
			sourceFile,
			sourceFile,
			sourceRange,
			nil,
		)
	} else {
		mediaRef = gotio.NewMissingReference(
			name,
			sourceRange,
			nil,
		)
	}

	// Create clip metadata - preserve ALL columns dynamically in metadata["ALE"]
	metadata := make(gotio.AnyDictionary)
	aleMetadata := make(map[string]interface{})

	// Parse ASC CDL data first if present
	ascSOP := row["ASC_SOP"]
	ascSat := row["ASC_SAT"]
	if ascSOP != "" || ascSat != "" {
		cdl, err := parseASCCDL(ascSOP, ascSat)
		if err == nil && cdl != nil {
			metadata["cdl"] = cdl
		}
	}

	// Columns to exclude from ALE metadata (these are handled specially)
	// We only exclude the core OTIO fields that map directly to clip properties
	excludeColumns := map[string]bool{
		d.nameColumnKey:  true, // Mapped to clip.Name
		ColumnStart:      true, // Mapped to sourceRange.StartTime
		ColumnEnd:        true, // Mapped to sourceRange.EndTime
		ColumnDuration:   true, // Mapped to sourceRange.Duration
		ColumnSourceFile: true, // Mapped to mediaReference
		ColumnTape:       true, // Fallback for mediaReference
		"ASC_SOP":        true, // Parsed into metadata["cdl"]
		"ASC_SAT":        true, // Parsed into metadata["cdl"]
	}

	// Store all remaining columns in ALE metadata for round-trip preservation
	for key, value := range row {
		if !excludeColumns[key] && value != "" {
			aleMetadata[key] = value
		}
	}

	// Only add ALE metadata if we have any
	if len(aleMetadata) > 0 {
		metadata["ALE"] = aleMetadata
	}

	// Create and return clip
	clip := gotio.NewClip(
		name,
		mediaRef,
		sourceRange,
		metadata,
		nil, // effects
		nil, // markers
		"",  // activeMediaReferenceKey (use default)
		nil, // color
	)

	return clip, nil
}
