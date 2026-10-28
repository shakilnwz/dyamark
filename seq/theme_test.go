package seq

import (
	"image/color"
	"testing"
)

func TestPresetThemes(t *testing.T) {
	def := DefaultTheme()
	if def.ActorBkg != HexColor("#eaeaea", 255) {
		t.Errorf("default actor bkg unexpected: %v", def.ActorBkg)
	}

	dark := DarkTheme()
	if dark.ActorBkg != HexColor("#2b2b2b", 255) {
		t.Errorf("dark actor bkg unexpected: %v", dark.ActorBkg)
	}

	forest := ForestTheme()
	if forest.ActorBorder != HexColor("#13540c", 255) {
		t.Errorf("forest actor border unexpected: %v", forest.ActorBorder)
	}

	neutral := NeutralTheme()
	if neutral.Signal != HexColor("#666666", 255) {
		t.Errorf("neutral signal unexpected: %v", neutral.Signal)
	}
}

func TestResolveTheme(t *testing.T) {
	base := DarkTheme()

	// Default keeps base if empty
	res := ResolveTheme(base, "", nil)
	if res.ActorBkg != base.ActorBkg {
		t.Errorf("expected base theme kept, got %v", res.ActorBkg)
	}

	// Override with named theme
	forest := ResolveTheme(base, "forest", nil)
	if forest.ActorBkg != ForestTheme().ActorBkg {
		t.Errorf("expected forest theme, got %v", forest.ActorBkg)
	}

	// Override with theme variables
	custom := ResolveTheme(base, "", map[string]string{
		"actorBkg":    "#123456",
		"signalColor": "red",
	})
	if custom.ActorBkg != HexColor("#123456", 255) {
		t.Errorf("expected custom actorBkg #123456, got %v", custom.ActorBkg)
	}
	if custom.Signal != (color.NRGBA{R: 255, G: 0, B: 0, A: 255}) {
		t.Errorf("expected custom signal red, got %v", custom.Signal)
	}
	// The frame's tab keyword and its title have their own colours.
	frames := ResolveTheme(base, "", map[string]string{
		"labelTextColor": "#0000ff",
		"loopTextColor":  "#00ff00",
	})
	if frames.LabelText != HexColor("#0000ff", 255) || frames.FrameText != HexColor("#00ff00", 255) {
		t.Errorf("label text %v, loop text %v", frames.LabelText, frames.FrameText)
	}
}

func TestParseColor(t *testing.T) {
	fallback := color.NRGBA{R: 10, G: 20, B: 30, A: 255}

	if c := ParseColor("none", fallback); c.A != 0 {
		t.Errorf("expected transparent, got %v", c)
	}
	if c := ParseColor("transparent", fallback); c.A != 0 {
		t.Errorf("expected transparent, got %v", c)
	}
	if c := ParseColor("blue", fallback); c != (color.NRGBA{R: 0, G: 0, B: 255, A: 255}) {
		t.Errorf("expected blue, got %v", c)
	}
	if c := ParseColor("#0f0", fallback); c != (color.NRGBA{R: 0, G: 255, B: 0, A: 255}) {
		t.Errorf("expected green #0f0, got %v", c)
	}
	if c := ParseColor("invalid", fallback); c != fallback {
		t.Errorf("expected fallback, got %v", c)
	}

	faint := color.NRGBA{R: 10, G: 20, B: 30, A: 40}
	cases := map[string]color.NRGBA{
		"#123456":                    {R: 0x12, G: 0x34, B: 0x56, A: 255},
		"#12345680":                  {R: 0x12, G: 0x34, B: 0x56, A: 0x80},
		"#0f08":                      {R: 0, G: 255, B: 0, A: 0x88},
		"Aqua":                       {R: 0, G: 255, B: 255, A: 255},
		"orange":                     {R: 255, G: 165, B: 0, A: 255},
		"rgb(255, 0, 0)":             {R: 255, A: 255},
		"rgba(0, 0, 255, 0.5)":       {B: 255, A: 128},
		"rgb(0 128 0 / 50%)":         {G: 128, A: 128},
		"rgb(100%, 0%, 0%)":          {R: 255, A: 255},
		"hsl(120, 100%, 50%)":        {G: 255, A: 255},
		"hsla(240deg, 100%, 50%, 1)": {B: 255, A: 255},
		"rgb(1, 2)":                  faint,
		"#12":                        faint,
	}
	for in, want := range cases {
		if got := ParseColor(in, faint); got != want {
			t.Errorf("ParseColor(%q) = %v, want %v", in, got, want)
		}
	}
}

// Text on a colour is black or white, whichever has more contrast; the
// sequence-number badge in the snxz theme gets black.
func TestTextOn(t *testing.T) {
	for _, tt := range []struct {
		bg   string
		want uint8
	}{{"#7186fd", 0}, {"#ffd60a", 0}, {"#1f242c", 255}, {"#e64553", 0}, {"#1e66f5", 255}} {
		bg := HexColor(tt.bg, 255)
		got := TextOn(bg)
		if got.R != tt.want {
			t.Errorf("on %s: got %v", tt.bg, got)
		}
		if c := Contrast(got, bg); c < 4.5 {
			t.Errorf("on %s: contrast %.2f", tt.bg, c)
		}
	}
	if c := Contrast(HexColor("#000", 255), HexColor("#fff", 255)); c < 20.9 || c > 21.1 {
		t.Errorf("black on white: %.2f", c)
	}
}
