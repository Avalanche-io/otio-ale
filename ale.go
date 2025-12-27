// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

// Package ale implements an Avid Log Exchange (ALE) adapter for OpenTimelineIO.
// ALE is a tab-separated text file format used for exchanging video logging data.
package ale

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Avalanche-io/gotio/opentime"
)

// Common ALE column names
const (
	ColumnName     = "Name"
	ColumnTracks   = "Tracks"
	ColumnStart    = "Start"
	ColumnEnd      = "End"
	ColumnDuration = "Duration"
	ColumnTape     = "Tape"
	ColumnSourceFile = "Source File"
	ColumnFPS      = "FPS"
)

// Common ALE header keywords
const (
	HeaderHeading      = "Heading"
	HeaderColumn       = "Column"
	HeaderData         = "Data"
	HeaderFieldDelim   = "FIELD_DELIM"
	HeaderVideoFormat  = "VIDEO_FORMAT"
	HeaderAudioFormat  = "AUDIO_FORMAT"
	HeaderFPS          = "FPS"
	HeaderTabs         = "TABS"
)

// Default values
const (
	DefaultFPS = 24.0
	DefaultFieldDelim = "TABS"
)

// ALEFile represents the structure of an ALE file
type ALEFile struct {
	Headers map[string]string
	Columns []string
	Rows    []map[string]string
}

// NewALEFile creates a new empty ALE file structure
func NewALEFile() *ALEFile {
	return &ALEFile{
		Headers: make(map[string]string),
		Columns: make([]string, 0),
		Rows:    make([]map[string]string, 0),
	}
}

// parseTimecode attempts to parse a timecode string and return a RationalTime
func parseTimecode(tc string, fps float64) (opentime.RationalTime, error) {
	if tc == "" {
		return opentime.RationalTime{}, fmt.Errorf("empty timecode")
	}

	// Try parsing as timecode (HH:MM:SS:FF or HH:MM:SS;FF)
	rt, err := opentime.FromTimecode(tc, fps)
	if err == nil {
		return rt, nil
	}

	// Try parsing as frame number
	if frames, err := strconv.ParseFloat(tc, 64); err == nil {
		return opentime.FromFrames(frames, fps), nil
	}

	return opentime.RationalTime{}, fmt.Errorf("invalid timecode format: %s", tc)
}

// formatTimecode converts a RationalTime to a timecode string
func formatTimecode(rt opentime.RationalTime, fps float64, dropFrame bool) (string, error) {
	rescaled := rt.RescaledTo(fps)

	var dfMode opentime.IsDropFrameRate
	if dropFrame {
		dfMode = opentime.ForceYes
	} else {
		dfMode = opentime.ForceNo
	}

	return rescaled.ToTimecode(fps, dfMode)
}

// parseFrameNumber attempts to parse a frame number string
func parseFrameNumber(s string, fps float64) (opentime.RationalTime, error) {
	frames, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return opentime.RationalTime{}, err
	}
	return opentime.FromFrames(frames, fps), nil
}

// formatFrameNumber converts a RationalTime to a frame number string
func formatFrameNumber(rt opentime.RationalTime, fps float64) string {
	rescaled := rt.RescaledTo(fps)
	return strconv.FormatInt(int64(rescaled.Value()), 10)
}

// splitTabs splits a line by tab characters
func splitTabs(line string) []string {
	return strings.Split(line, "\t")
}

// joinTabs joins strings with tab characters
func joinTabs(parts []string) string {
	return strings.Join(parts, "\t")
}

// parseFPS attempts to parse an FPS value from a string
func parseFPS(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultFPS, nil
	}

	fps, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return DefaultFPS, fmt.Errorf("invalid FPS value: %s", s)
	}

	if fps <= 0 {
		return DefaultFPS, fmt.Errorf("FPS must be positive: %f", fps)
	}

	return fps, nil
}

// isDropFrame checks if the FPS value indicates drop frame timecode
func isDropFrame(fps float64) bool {
	// 29.97 and 59.94 typically use drop frame
	return (fps > 29.96 && fps < 29.98) || (fps > 59.93 && fps < 59.95)
}

// CDLData represents ASC CDL color correction data
type CDLData struct {
	ASCSOP *SOPValues `json:"asc_sop,omitempty"`
	ASCSat *float64   `json:"asc_sat,omitempty"`
}

// SOPValues represents Slope, Offset, Power values for CDL
type SOPValues struct {
	Slope  [3]float64 `json:"slope"`
	Offset [3]float64 `json:"offset"`
	Power  [3]float64 `json:"power"`
}

// parseASCCDL parses ASC_SOP and ASC_SAT columns into structured CDL data
func parseASCCDL(ascSOP, ascSat string) (*CDLData, error) {
	cdl := &CDLData{}

	// Parse ASC_SOP if present
	if ascSOP != "" {
		sop, err := parseASCSOP(ascSOP)
		if err == nil && sop != nil {
			cdl.ASCSOP = sop
		}
	}

	// Parse ASC_SAT if present
	if ascSat != "" {
		sat, err := strconv.ParseFloat(strings.TrimSpace(ascSat), 64)
		if err == nil {
			cdl.ASCSat = &sat
		}
	}

	// Only return CDL if we got some data
	if cdl.ASCSOP != nil || cdl.ASCSat != nil {
		return cdl, nil
	}

	return nil, nil
}

// parseASCSOP parses an ASC_SOP string like "(0.8714 0.9334 0.9947)(-0.087 -0.0922 -0.0808)(0.9988 1.0218 1.0101)"
func parseASCSOP(s string) (*SOPValues, error) {
	// Remove all parentheses and split by whitespace
	s = strings.ReplaceAll(s, "(", " ")
	s = strings.ReplaceAll(s, ")", " ")
	fields := strings.Fields(s)

	if len(fields) < 9 {
		return nil, fmt.Errorf("ASC_SOP requires at least 9 values, got %d", len(fields))
	}

	sop := &SOPValues{}

	// Parse slope (first 3 values)
	for i := 0; i < 3; i++ {
		val, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid slope value: %s", fields[i])
		}
		sop.Slope[i] = val
	}

	// Parse offset (next 3 values)
	for i := 0; i < 3; i++ {
		val, err := strconv.ParseFloat(fields[3+i], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid offset value: %s", fields[3+i])
		}
		sop.Offset[i] = val
	}

	// Parse power (next 3 values)
	for i := 0; i < 3; i++ {
		val, err := strconv.ParseFloat(fields[6+i], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid power value: %s", fields[6+i])
		}
		sop.Power[i] = val
	}

	return sop, nil
}

// formatASCSOP formats SOPValues back to ASC_SOP string format
func formatASCSOP(sop *SOPValues) string {
	return fmt.Sprintf("(%.4f %.4f %.4f)(%.4f %.4f %.4f)(%.4f %.4f %.4f)",
		sop.Slope[0], sop.Slope[1], sop.Slope[2],
		sop.Offset[0], sop.Offset[1], sop.Offset[2],
		sop.Power[0], sop.Power[1], sop.Power[2])
}

// videoFormatFromDimensions infers Avid video format from width and height
func videoFormatFromDimensions(width, height int) string {
	// Map height to format
	switch height {
	case 1080:
		// Check for 2K DCI 1080 format
		if width > 1920 {
			return "CUSTOM"
		}
		return "1080"
	case 720:
		return "720"
	case 576:
		return "PAL"
	case 486:
		return "NTSC"
	default:
		return "CUSTOM"
	}
}

// parseImageSize extracts width and height from "Image Size" metadata field
func parseImageSize(imageSize string) (width, height int, ok bool) {
	// Match patterns like "1920 x 1080" or "1920x1080"
	re := strings.NewReplacer(" ", "", "x", " ", "X", " ")
	normalized := re.Replace(imageSize)

	var w, h int
	n, _ := fmt.Sscanf(normalized, "%d %d", &w, &h)
	if n == 2 {
		return w, h, true
	}

	return 0, 0, false
}
