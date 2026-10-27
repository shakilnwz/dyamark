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
}
