// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

package ale

import (
	"os"
	"strings"
	"testing"
)

func TestParseASCSOP(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid SOP",
			input:   "(0.8714 0.9334 0.9947)(-0.087 -0.0922 -0.0808)(0.9988 1.0218 1.0101)",
			wantErr: false,
		},
		{
			name:    "valid SOP with spaces",
			input:   "( 0.8714  0.9334  0.9947 ) ( -0.087  -0.0922  -0.0808 ) ( 0.9988  1.0218  1.0101 )",
			wantErr: false,
		},
		{
			name:    "invalid - too few values",
			input:   "(0.8714 0.9334)(0.9988 1.0218)",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sop, err := parseASCSOP(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseASCSOP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && sop == nil {
				t.Error("parseASCSOP() returned nil SOP without error")
			}
		})
	}
}

func TestParseASCCDL(t *testing.T) {
	ascSOP := "(0.8714 0.9334 0.9947)(-0.087 -0.0922 -0.0808)(0.9988 1.0218 1.0101)"
	ascSat := "0.9"

	cdl, err := parseASCCDL(ascSOP, ascSat)
	if err != nil {
		t.Fatalf("parseASCCDL() error = %v", err)
	}

	if cdl == nil {
		t.Fatal("parseASCCDL() returned nil")
	}

	if cdl.ASCSOP == nil {
		t.Error("CDL missing ASCSOP")
	}

	if cdl.ASCSat == nil {
		t.Error("CDL missing ASCSat")
	} else if *cdl.ASCSat != 0.9 {
		t.Errorf("ASCSat = %v, want 0.9", *cdl.ASCSat)
	}
}

func TestFormatASCSOP(t *testing.T) {
	sop := &SOPValues{
		Slope:  [3]float64{0.8714, 0.9334, 0.9947},
		Offset: [3]float64{-0.087, -0.0922, -0.0808},
		Power:  [3]float64{0.9988, 1.0218, 1.0101},
	}

	result := formatASCSOP(sop)

	// Should contain all values in parentheses
	if !strings.Contains(result, "0.8714") {
		t.Error("formatASCSOP() missing slope value")
	}
	if !strings.Contains(result, "-0.087") {
		t.Error("formatASCSOP() missing offset value")
	}
	if !strings.Contains(result, "0.9988") {
		t.Error("formatASCSOP() missing power value")
	}
}

func TestDecoder_WithASCCDL(t *testing.T) {
	// Read sample_cdl.ale
	data, err := os.ReadFile("testdata/sample_cdl.ale")
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	decoder := NewDecoder(strings.NewReader(string(data)))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Failed to decode ALE: %v", err)
	}

	clips := timeline.FindClips(nil, false)
	if len(clips) == 0 {
		t.Fatal("No clips found in timeline")
	}

	// Check first clip for CDL metadata
	clip := clips[0]
	metadata := clip.Metadata()

	if cdlData, ok := metadata["cdl"]; ok {
		if cdl, ok := cdlData.(*CDLData); ok {
			if cdl.ASCSOP == nil {
				t.Error("Clip missing ASCSOP in cdl metadata")
			}
			if cdl.ASCSat == nil {
				t.Error("Clip missing ASCSat in cdl metadata")
			} else if *cdl.ASCSat != 0.9 {
				t.Errorf("ASCSat = %v, want 0.9", *cdl.ASCSat)
			}
		} else {
			t.Error("cdl metadata is not *CDLData type")
		}
	} else {
		t.Error("Clip missing cdl metadata")
	}
}

func TestVideoFormatFromDimensions(t *testing.T) {
	tests := []struct {
		width  int
		height int
		want   string
	}{
		{1920, 1080, "1080"},
		{1280, 720, "720"},
		{720, 576, "PAL"},
		{720, 486, "NTSC"},
		{2048, 1080, "CUSTOM"}, // 2K DCI
		{3840, 2160, "CUSTOM"}, // 4K
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := videoFormatFromDimensions(tt.width, tt.height)
			if got != tt.want {
				t.Errorf("videoFormatFromDimensions(%d, %d) = %v, want %v", tt.width, tt.height, got, tt.want)
			}
		})
	}
}

func TestParseImageSize(t *testing.T) {
	tests := []struct {
		input      string
		wantWidth  int
		wantHeight int
		wantOk     bool
	}{
		{"1920 x 1080", 1920, 1080, true},
		{"1920x1080", 1920, 1080, true},
		{"1920X1080", 1920, 1080, true},
		{"invalid", 0, 0, false},
		{"", 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			w, h, ok := parseImageSize(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseImageSize(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
			if ok && (w != tt.wantWidth || h != tt.wantHeight) {
				t.Errorf("parseImageSize(%q) = (%d, %d), want (%d, %d)", tt.input, w, h, tt.wantWidth, tt.wantHeight)
			}
		})
	}
}
