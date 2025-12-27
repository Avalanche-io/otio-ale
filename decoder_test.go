// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

package ale

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mrjoshuak/gotio/opentime"
	"github.com/mrjoshuak/gotio/opentimelineio"
)

func TestDecoder_BasicALE(t *testing.T) {
	aleContent := `Heading
FIELD_DELIM	TABS
VIDEO_FORMAT	1920x1080
AUDIO_FORMAT	48kHz
FPS	24.00

Column
Name	Start	End	Duration

Data
Clip001	01:00:00:00	01:00:05:00	120
Clip002	01:00:05:00	01:00:10:00	120
Clip003	01:00:10:00	01:00:15:00	120
`

	decoder := NewDecoder(strings.NewReader(aleContent))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Failed to decode ALE: %v", err)
	}

	if timeline == nil {
		t.Fatal("Timeline is nil")
	}

	// Check timeline has tracks
	tracks := timeline.Tracks()
	if tracks == nil {
		t.Fatal("Timeline has no tracks")
	}

	children := tracks.Children()
	if len(children) == 0 {
		t.Fatal("Timeline has no track children")
	}

	// Get video track
	videoTrack := timeline.VideoTracks()
	if len(videoTrack) == 0 {
		t.Fatal("No video tracks found")
	}

	// Check clips
	clips := videoTrack[0].Children()
	if len(clips) != 3 {
		t.Fatalf("Expected 3 clips, got %d", len(clips))
	}

	// Verify first clip
	clip := clips[0].(*opentimelineio.Clip)
	if clip.Name() != "Clip001" {
		t.Errorf("Expected clip name 'Clip001', got '%s'", clip.Name())
	}

	sourceRange := clip.SourceRange()
	if sourceRange == nil {
		t.Fatal("Clip has no source range")
	}

	// Check start time is 01:00:00:00 at 24fps
	expectedStart := opentime.NewRationalTime(86400, 24) // 1 hour = 3600 seconds * 24 fps
	if !sourceRange.StartTime().Equal(expectedStart) {
		t.Errorf("Expected start time %v, got %v", expectedStart, sourceRange.StartTime())
	}

	// Check duration is 120 frames at 24fps
	expectedDuration := opentime.NewRationalTime(120, 24)
	if !sourceRange.Duration().Equal(expectedDuration) {
		t.Errorf("Expected duration %v, got %v", expectedDuration, sourceRange.Duration())
	}
}

func TestDecoder_WithFPSOption(t *testing.T) {
	aleContent := `Heading
FIELD_DELIM	TABS

Column
Name	Duration

Data
Clip001	100
`

	decoder := NewDecoder(strings.NewReader(aleContent), WithFPS(30.0))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Failed to decode ALE: %v", err)
	}

	clips := timeline.VideoTracks()[0].Children()
	clip := clips[0].(*opentimelineio.Clip)

	sourceRange := clip.SourceRange()
	if sourceRange.Duration().Rate() != 30.0 {
		t.Errorf("Expected rate 30.0, got %f", sourceRange.Duration().Rate())
	}
}

func TestDecoder_WithCustomNameColumn(t *testing.T) {
	aleContent := `Heading
FIELD_DELIM	TABS

Column
ClipName	Duration

Data
MyClip	50
`

	decoder := NewDecoder(strings.NewReader(aleContent), WithNameColumn("ClipName"))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Failed to decode ALE: %v", err)
	}

	clips := timeline.VideoTracks()[0].Children()
	clip := clips[0].(*opentimelineio.Clip)

	if clip.Name() != "MyClip" {
		t.Errorf("Expected clip name 'MyClip', got '%s'", clip.Name())
	}
}

func TestDecoder_EmptyALE(t *testing.T) {
	aleContent := `Heading
FIELD_DELIM	TABS

Column
Name	Duration

Data
`

	decoder := NewDecoder(strings.NewReader(aleContent))
	_, err := decoder.Decode()
	if err == nil {
		t.Fatal("Expected error for empty ALE data, got nil")
	}
}

func TestDecoder_WithMetadata(t *testing.T) {
	aleContent := `Heading
FIELD_DELIM	TABS

Column
Name	Duration	Tape	Scene

Data
Clip001	100	A001	Scene1
`

	decoder := NewDecoder(strings.NewReader(aleContent))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Failed to decode ALE: %v", err)
	}

	clips := timeline.VideoTracks()[0].Children()
	clip := clips[0].(*opentimelineio.Clip)

	metadata := clip.Metadata()
	// Extra columns are now stored in metadata["ALE"]
	if aleData, ok := metadata["ALE"]; ok {
		aleMap := aleData.(map[string]interface{})
		if aleMap["Scene"] != "Scene1" {
			t.Errorf("Expected Scene 'Scene1', got '%v'", aleMap["Scene"])
		}
	} else {
		t.Error("Expected ALE metadata")
	}

	// Tape is handled specially as a fallback for Source File, not in ALE metadata
	// Check that we have a media reference instead
	ref := clip.MediaReference()
	if ref == nil {
		t.Error("Expected media reference from Tape column")
	}
}

func TestDecoder_WithSourceFile(t *testing.T) {
	aleContent := `Heading
FIELD_DELIM	TABS

Column
Name	Duration	Source File

Data
Clip001	100	/path/to/media.mov
`

	decoder := NewDecoder(strings.NewReader(aleContent))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Failed to decode ALE: %v", err)
	}

	clips := timeline.VideoTracks()[0].Children()
	clip := clips[0].(*opentimelineio.Clip)

	mediaRef := clip.MediaReference()
	if mediaRef == nil {
		t.Fatal("Clip has no media reference")
	}

	extRef, ok := mediaRef.(*opentimelineio.ExternalReference)
	if !ok {
		t.Fatal("Media reference is not an ExternalReference")
	}

	if extRef.TargetURL() != "/path/to/media.mov" {
		t.Errorf("Expected target URL '/path/to/media.mov', got '%s'", extRef.TargetURL())
	}
}

func TestParseTimecode(t *testing.T) {
	tests := []struct {
		name     string
		tc       string
		fps      float64
		wantErr  bool
	}{
		{"valid non-drop", "01:00:00:00", 24.0, false},
		{"valid drop", "01:00:00;00", 29.97, false},
		{"valid with frames", "00:00:10:15", 24.0, false},
		{"empty", "", 24.0, true},
		{"frame number", "100", 24.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTimecode(tt.tc, tt.fps)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTimecode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseFPS(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{"valid 24", "24.00", 24.0, false},
		{"valid 29.97", "29.97", 29.97, false},
		{"empty", "", DefaultFPS, false},
		{"invalid", "abc", DefaultFPS, true},
		{"negative", "-10", DefaultFPS, true},
		{"zero", "0", DefaultFPS, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFPS(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFPS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseFPS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDropFrame(t *testing.T) {
	tests := []struct {
		fps  float64
		want bool
	}{
		{24.0, false},
		{25.0, false},
		{29.97, true},
		{30.0, false},
		{59.94, true},
		{60.0, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%.2f", tt.fps), func(t *testing.T) {
			if got := isDropFrame(tt.fps); got != tt.want {
				t.Errorf("isDropFrame(%f) = %v, want %v", tt.fps, got, tt.want)
			}
		})
	}
}
