package palette

import (
	"image/color"
	"testing"

	"dyamark/seq"
)

func TestDefaultTheme(t *testing.T) {
	th := Default(10)

	// Accent should match Omarchy fallback #FFD60A
	expectedAccent := seq.HexColor("#FFD60A", 255)
	if th.Accent != expectedAccent {
		t.Errorf("expected accent %v, got %v", expectedAccent, th.Accent)
	}
	if th.Seq.ActorBorder.A == 0 {
		t.Error("expected non-zero alpha for actor border")
	}
	if th.Seq.Signal.A == 0 {
		t.Error("expected non-zero alpha for signal color")
	}
	// Frame labels sit on the faint frame fill, so they're in the text
	// colour, not the page's.
	if th.Seq.FrameText != th.Foreground || th.Seq.LabelText != th.Foreground {
		t.Errorf("frame text %v, label text %v, want the foreground %v", th.Seq.FrameText, th.Seq.LabelText, th.Foreground)
	}
}

func TestZeroRoundingIsKept(t *testing.T) {
	th := Default(0)
	if th.Rounding != 0 || th.Seq.Rounding != 0 {
		t.Errorf("rounding 0 should give square corners, got %d / %v", th.Rounding, th.Seq.Rounding)
	}
}

func TestMix(t *testing.T) {
	a := color.NRGBA{R: 0, G: 100, B: 200, A: 77}
	b := color.NRGBA{R: 200, G: 100, B: 0, A: 255}
	if got := Mix(a, b, 0); got != a {
		t.Errorf("Mix(a, b, 0) = %v, want a %v", got, a)
	}
	if got, want := Mix(a, b, 0.5), (color.NRGBA{R: 100, G: 100, B: 100, A: 77}); got != want {
		t.Errorf("Mix(a, b, 0.5) = %v, want %v, keeping a's alpha", got, want)
	}
}
