// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

package ale

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/mrjoshuak/gotio/opentimelineio"
)

func TestRoundTrip_WithAllColumns(t *testing.T) {
	// Read sample.ale which has many columns
	data, err := os.ReadFile("testdata/sample.ale")
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	// Decode
	decoder := NewDecoder(strings.NewReader(string(data)))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Failed to decode ALE: %v", err)
	}

	clips := timeline.FindClips(nil, false)
	if len(clips) == 0 {
		t.Fatal("No clips found")
	}

	// Verify that extra columns with values are preserved in metadata["ALE"]
	firstClip := clips[0]
	metadata := firstClip.Metadata()
	if aleData, ok := metadata["ALE"]; ok {
		aleMap := aleData.(map[string]interface{})
		// Check for some custom columns from sample.ale that have values
		// Looking at the file, these columns have non-empty values:
		// "Project", "Format", "Image Size", "Raster Dimension"
		if _, ok := aleMap["Project"]; !ok {
			t.Error("Missing Project in ALE metadata")
		}
		if _, ok := aleMap["Format"]; !ok {
			t.Error("Missing Format in ALE metadata")
		}
		if _, ok := aleMap["Image Size"]; !ok {
			t.Error("Missing Image Size in ALE metadata")
		}
	} else {
		t.Error("Missing ALE metadata")
	}

	// Encode back
	var buf bytes.Buffer
	encoder := NewEncoder(&buf, WithEncoderFPS(24.0))
	err = encoder.Encode(timeline)
	if err != nil {
		t.Fatalf("Failed to encode timeline: %v", err)
	}

	output := buf.String()

	// Verify output contains custom columns that had values
	if !strings.Contains(output, "Project") {
		t.Error("Output missing Project column")
	}
	if !strings.Contains(output, "Format") {
		t.Error("Output missing Format column")
	}
	if !strings.Contains(output, "Image Size") {
		t.Error("Output missing Image Size column")
	}

	// Decode again to verify round-trip
	decoder2 := NewDecoder(strings.NewReader(output))
	timeline2, err := decoder2.Decode()
	if err != nil {
		t.Fatalf("Failed to decode round-trip ALE: %v", err)
	}

	clips2 := timeline2.FindClips(nil, false)
	if len(clips2) != len(clips) {
		t.Errorf("Round-trip clip count mismatch: got %d, want %d", len(clips2), len(clips))
	}
}

func TestRoundTrip_WithCDL(t *testing.T) {
	// Read sample_cdl.ale
	data, err := os.ReadFile("testdata/sample_cdl.ale")
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	// Decode
	decoder := NewDecoder(strings.NewReader(string(data)))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Failed to decode ALE: %v", err)
	}

	// Encode
	var buf bytes.Buffer
	encoder := NewEncoder(&buf, WithEncoderFPS(23.976))
	err = encoder.Encode(timeline)
	if err != nil {
		t.Fatalf("Failed to encode timeline: %v", err)
	}

	output := buf.String()

	// Verify output contains CDL columns
	if !strings.Contains(output, "ASC_SOP") {
		t.Error("Output missing ASC_SOP column")
	}
	if !strings.Contains(output, "ASC_SAT") {
		t.Error("Output missing ASC_SAT column")
	}

	// Verify ASC_SAT value is preserved
	if !strings.Contains(output, "0.9") {
		t.Error("Output missing ASC_SAT value 0.9")
	}

	// Decode again
	decoder2 := NewDecoder(strings.NewReader(output))
	timeline2, err := decoder2.Decode()
	if err != nil {
		t.Fatalf("Failed to decode round-trip ALE: %v", err)
	}

	clips2 := timeline2.FindClips(nil, false)
	if len(clips2) == 0 {
		t.Fatal("No clips in round-trip timeline")
	}

	// Verify CDL is still there
	metadata := clips2[0].Metadata()
	if cdlData, ok := metadata["cdl"]; !ok {
		t.Error("Round-trip clip missing cdl metadata")
	} else {
		if cdl, ok := cdlData.(*CDLData); ok {
			if cdl.ASCSOP == nil {
				t.Error("Round-trip CDL missing ASCSOP")
			}
			if cdl.ASCSat == nil {
				t.Error("Round-trip CDL missing ASCSat")
			}
		}
	}
}

func TestRoundTrip_WithTracks(t *testing.T) {
	// Read sample2.ale which has VA1 tracks
	data, err := os.ReadFile("testdata/sample2.ale")
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	// Decode
	decoder := NewDecoder(strings.NewReader(string(data)))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Failed to decode ALE: %v", err)
	}

	// Check that we created tracks based on Tracks column
	tracks := timeline.Tracks().Children()
	if len(tracks) == 0 {
		t.Fatal("No tracks found")
	}

	// Encode back
	var buf bytes.Buffer
	encoder := NewEncoder(&buf, WithEncoderFPS(23.98))
	err = encoder.Encode(timeline)
	if err != nil {
		t.Fatalf("Failed to encode timeline: %v", err)
	}

	output := buf.String()

	// Should have Tracks column
	if !strings.Contains(output, "Tracks") {
		t.Error("Output missing Tracks column")
	}
}

func TestDynamicColumnPreservation(t *testing.T) {
	// Create a timeline with clips that have custom ALE metadata
	timeline := opentimelineio.NewTimeline("Test", nil, nil)
	track := opentimelineio.NewTrack("V", nil, opentimelineio.TrackKindVideo, nil, nil)

	metadata := make(opentimelineio.AnyDictionary)
	aleMetadata := make(map[string]interface{})
	aleMetadata["CustomColumn1"] = "Value1"
	aleMetadata["CustomColumn2"] = "Value2"
	aleMetadata["Scene"] = "Scene001"
	metadata["ALE"] = aleMetadata

	clip := opentimelineio.NewClip(
		"TestClip",
		opentimelineio.NewMissingReference("", nil, nil),
		nil,
		metadata,
		nil,
		nil,
		"",
		nil,
	)

	track.AppendChild(clip)
	timeline.Tracks().AppendChild(track)

	// Encode
	var buf bytes.Buffer
	encoder := NewEncoder(&buf, WithEncoderFPS(24.0))
	err := encoder.Encode(timeline)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	output := buf.String()

	// Verify custom columns are in output
	if !strings.Contains(output, "CustomColumn1") {
		t.Error("Output missing CustomColumn1")
	}
	if !strings.Contains(output, "CustomColumn2") {
		t.Error("Output missing CustomColumn2")
	}
	if !strings.Contains(output, "Scene") {
		t.Error("Output missing Scene")
	}
	if !strings.Contains(output, "Value1") {
		t.Error("Output missing Value1")
	}
	if !strings.Contains(output, "Value2") {
		t.Error("Output missing Value2")
	}

	// Decode and verify
	decoder := NewDecoder(strings.NewReader(output))
	timeline2, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	clips := timeline2.FindClips(nil, false)
	if len(clips) != 1 {
		t.Fatalf("Expected 1 clip, got %d", len(clips))
	}

	metadata2 := clips[0].Metadata()
	if aleData, ok := metadata2["ALE"]; ok {
		aleMap := aleData.(map[string]interface{})
		if aleMap["CustomColumn1"] != "Value1" {
			t.Errorf("CustomColumn1 = %v, want Value1", aleMap["CustomColumn1"])
		}
		if aleMap["CustomColumn2"] != "Value2" {
			t.Errorf("CustomColumn2 = %v, want Value2", aleMap["CustomColumn2"])
		}
	} else {
		t.Error("Missing ALE metadata after round-trip")
	}
}
