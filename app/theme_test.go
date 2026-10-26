package app

import (
	"image/color"
	"testing"
)

func TestDefaultTheme(t *testing.T) {
	th := DefaultTheme()
	if th == nil {
		t.Fatal("expected non-nil default theme")
	}

	// Accent should match Omarchy fallback #FFD60A
	expectedAccent := hexColor("#FFD60A", 255)
	if th.Accent != expectedAccent {
		t.Errorf("expected accent %v, got %v", expectedAccent, th.Accent)
	}

	if th.Rounding <= 0 {
		t.Errorf("expected positive rounding, got %d", th.Rounding)
	}

	if th.Seq.ActorBorder.A == 0 {
		t.Error("expected non-zero alpha for actor border")
	}
	if th.Seq.Signal.A == 0 {
		t.Error("expected non-zero alpha for signal color")
	}
}

func TestHexColor(t *testing.T) {
	cases := []struct {
		input    string
		alpha    uint8
		expected color.NRGBA
	}{
		{"#fff", 255, color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
		{"#000000", 255, color.NRGBA{R: 0, G: 0, B: 0, A: 255}},
		{"#7186fd", 200, color.NRGBA{R: 0x71, G: 0x86, B: 0xfd, A: 200}},
		{"#12345678", 255, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0x78}},
		{"invalid", 255, color.NRGBA{R: 200, G: 200, B: 200, A: 255}},
	}

	for _, tc := range cases {
		got := hexColor(tc.input, tc.alpha)
		if got != tc.expected {
			t.Errorf("hexColor(%q, %d) = %v, expected %v", tc.input, tc.alpha, got, tc.expected)
		}
	}
}

func TestLoadTheme(t *testing.T) {
	th := LoadTheme()
	if th == nil {
		t.Fatal("expected non-nil loaded theme")
	}
	if th.Background.A == 0 {
		t.Error("expected valid background alpha")
	}
	if th.Seq.Rounding <= 0 {
		t.Errorf("expected positive seq rounding, got %f", th.Seq.Rounding)
	}
}
