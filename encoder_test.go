// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

package ale

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Avalanche-io/gotio/opentime"
	"github.com/Avalanche-io/gotio/opentimelineio"
)

func TestEncoder_BasicTimeline(t *testing.T) {
	// Create a simple timeline with clips
	timeline := opentimelineio.NewTimeline("Test Timeline", nil, nil)

	videoTrack := opentimelineio.NewTrack(
		"Video",
		nil,
		opentimelineio.TrackKindVideo,
		nil,
		nil,
	)

	// Create clips
	clip1 := opentimelineio.NewClip(
		"Clip001",
		opentimelineio.NewMissingReference("", nil, nil),
		&opentime.TimeRange{},
		nil,
		nil,
		nil,
		"",
		nil,
	)
	*clip1.SourceRange() = opentime.NewTimeRange(
		opentime.NewRationalTime(0, 24),
		opentime.NewRationalTime(120, 24),
	)

	clip2 := opentimelineio.NewClip(
		"Clip002",
		opentimelineio.NewMissingReference("", nil, nil),
		&opentime.TimeRange{},
		nil,
		nil,
		nil,
		"",
		nil,
	)
	*clip2.SourceRange() = opentime.NewTimeRange(
		opentime.NewRationalTime(120, 24),
		opentime.NewRationalTime(120, 24),
	)

	videoTrack.AppendChild(clip1)
	videoTrack.AppendChild(clip2)
	timeline.Tracks().AppendChild(videoTrack)

	// Encode to ALE
	var buf bytes.Buffer
	encoder := NewEncoder(&buf, WithEncoderFPS(24.0))
	err := encoder.Encode(timeline)
	if err != nil {
		t.Fatalf("Failed to encode timeline: %v", err)
	}

	output := buf.String()

	// Check for expected sections
	if !strings.Contains(output, "Heading") {
		t.Error("Output missing Heading section")
	}
	if !strings.Contains(output, "Column") {
		t.Error("Output missing Column section")
	}
	if !strings.Contains(output, "Data") {
		t.Error("Output missing Data section")
	}

	// Check for clip names
	if !strings.Contains(output, "Clip001") {
		t.Error("Output missing Clip001")
	}
	if !strings.Contains(output, "Clip002") {
		t.Error("Output missing Clip002")
	}

	// Check for FPS header
	if !strings.Contains(output, "FPS\t24.00") {
		t.Error("Output missing FPS header")
	}
}

func TestEncoder_WithExternalReference(t *testing.T) {
	timeline := opentimelineio.NewTimeline("Test Timeline", nil, nil)

	videoTrack := opentimelineio.NewTrack(
		"Video",
		nil,
		opentimelineio.TrackKindVideo,
		nil,
		nil,
	)

	mediaRef := opentimelineio.NewExternalReference(
		"media.mov",
		"/path/to/media.mov",
		&opentime.TimeRange{},
		nil,
	)
	*mediaRef.AvailableRange() = opentime.NewTimeRange(
		opentime.NewRationalTime(0, 24),
		opentime.NewRationalTime(100, 24),
	)

	clip := opentimelineio.NewClip(
		"Clip001",
		mediaRef,
		mediaRef.AvailableRange(),
		nil,
		nil,
		nil,
		"",
		nil,
	)

	videoTrack.AppendChild(clip)
	timeline.Tracks().AppendChild(videoTrack)

	var buf bytes.Buffer
	encoder := NewEncoder(&buf, WithEncoderFPS(24.0))
	err := encoder.Encode(timeline)
	if err != nil {
		t.Fatalf("Failed to encode timeline: %v", err)
	}

	output := buf.String()

	// Check for source file column
	if !strings.Contains(output, "Source File") {
		t.Error("Output missing Source File column")
	}
	if !strings.Contains(output, "/path/to/media.mov") {
		t.Error("Output missing media file path")
	}
}

func TestEncoder_CustomColumns(t *testing.T) {
	timeline := opentimelineio.NewTimeline("Test Timeline", nil, nil)

	videoTrack := opentimelineio.NewTrack(
		"Video",
		nil,
		opentimelineio.TrackKindVideo,
		nil,
		nil,
	)

	metadata := make(opentimelineio.AnyDictionary)
	// Custom columns should be in metadata["ALE"]
	aleMetadata := make(map[string]interface{})
	aleMetadata["Scene"] = "Scene001"
	aleMetadata["Take"] = "Take1"
	metadata["ALE"] = aleMetadata

	clip := opentimelineio.NewClip(
		"Clip001",
		opentimelineio.NewMissingReference("", nil, nil),
		&opentime.TimeRange{},
		metadata,
		nil,
		nil,
		"",
		nil,
	)
	*clip.SourceRange() = opentime.NewTimeRange(
		opentime.NewRationalTime(0, 24),
		opentime.NewRationalTime(100, 24),
	)

	videoTrack.AppendChild(clip)
	timeline.Tracks().AppendChild(videoTrack)

	var buf bytes.Buffer
	columns := []string{"Name", "Duration", "Scene", "Take"}
	encoder := NewEncoder(&buf, WithEncoderFPS(24.0), WithColumns(columns))
	err := encoder.Encode(timeline)
	if err != nil {
		t.Fatalf("Failed to encode timeline: %v", err)
	}

	output := buf.String()

	// Check for custom columns
	if !strings.Contains(output, "Scene") {
		t.Error("Output missing Scene column")
	}
	if !strings.Contains(output, "Take") {
		t.Error("Output missing Take column")
	}
	if !strings.Contains(output, "Scene001") {
		t.Error("Output missing Scene001 value")
	}
	if !strings.Contains(output, "Take1") {
		t.Error("Output missing Take1 value")
	}
}

func TestEncoder_NilTimeline(t *testing.T) {
	var buf bytes.Buffer
	encoder := NewEncoder(&buf)
	err := encoder.Encode(nil)
	if err == nil {
		t.Fatal("Expected error for nil timeline, got nil")
	}
}

func TestEncoder_EmptyTimeline(t *testing.T) {
	timeline := opentimelineio.NewTimeline("Empty Timeline", nil, nil)

	var buf bytes.Buffer
	encoder := NewEncoder(&buf)
	err := encoder.Encode(timeline)
	if err != nil {
		t.Fatalf("Failed to encode empty timeline: %v", err)
	}

	output := buf.String()

	// Should still have structure even if no clips
	if !strings.Contains(output, "Heading") {
		t.Error("Output missing Heading section")
	}
	if !strings.Contains(output, "Column") {
		t.Error("Output missing Column section")
	}
	if !strings.Contains(output, "Data") {
		t.Error("Output missing Data section")
	}
}

func TestRoundTrip(t *testing.T) {
	// Create a timeline
	timeline := opentimelineio.NewTimeline("Test Timeline", nil, nil)

	videoTrack := opentimelineio.NewTrack(
		"Video",
		nil,
		opentimelineio.TrackKindVideo,
		nil,
		nil,
	)

	clip := opentimelineio.NewClip(
		"Clip001",
		opentimelineio.NewMissingReference("", nil, nil),
		&opentime.TimeRange{},
		nil,
		nil,
		nil,
		"",
		nil,
	)
	*clip.SourceRange() = opentime.NewTimeRange(
		opentime.NewRationalTime(100, 24),
		opentime.NewRationalTime(50, 24),
	)

	videoTrack.AppendChild(clip)
	timeline.Tracks().AppendChild(videoTrack)

	// Encode to ALE
	var buf bytes.Buffer
	encoder := NewEncoder(&buf, WithEncoderFPS(24.0))
	err := encoder.Encode(timeline)
	if err != nil {
		t.Fatalf("Failed to encode timeline: %v", err)
	}

	// Decode from ALE
	decoder := NewDecoder(strings.NewReader(buf.String()), WithFPS(24.0))
	decodedTimeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Failed to decode ALE: %v", err)
	}

	// Verify decoded timeline
	clips := decodedTimeline.VideoTracks()[0].Children()
	if len(clips) != 1 {
		t.Fatalf("Expected 1 clip, got %d", len(clips))
	}

	decodedClip := clips[0].(*opentimelineio.Clip)
	if decodedClip.Name() != "Clip001" {
		t.Errorf("Expected clip name 'Clip001', got '%s'", decodedClip.Name())
	}

	sourceRange := decodedClip.SourceRange()
	originalRange := clip.SourceRange()

	// Compare start times and durations
	if !sourceRange.StartTime().AlmostEqual(originalRange.StartTime(), 0.001) {
		t.Errorf("Start time mismatch: original %v, decoded %v",
			originalRange.StartTime(), sourceRange.StartTime())
	}

	if !sourceRange.Duration().AlmostEqual(originalRange.Duration(), 0.001) {
		t.Errorf("Duration mismatch: original %v, decoded %v",
			originalRange.Duration(), sourceRange.Duration())
	}
}

func TestFormatTimecode(t *testing.T) {
	tests := []struct {
		name      string
		time      opentime.RationalTime
		fps       float64
		dropFrame bool
		wantErr   bool
	}{
		{
			name:      "zero time",
			time:      opentime.NewRationalTime(0, 24),
			fps:       24.0,
			dropFrame: false,
			wantErr:   false,
		},
		{
			name:      "one second",
			time:      opentime.NewRationalTime(24, 24),
			fps:       24.0,
			dropFrame: false,
			wantErr:   false,
		},
		{
			name:      "drop frame",
			time:      opentime.NewRationalTime(30, 29.97),
			fps:       29.97,
			dropFrame: true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatTimecode(tt.time, tt.fps, tt.dropFrame)
			if (err != nil) != tt.wantErr {
				t.Errorf("formatTimecode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == "" {
				t.Error("formatTimecode() returned empty string")
			}
		})
	}
}
