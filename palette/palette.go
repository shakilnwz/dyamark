// Package palette holds dyamark's colours: the page's palette, and the
// sequence diagram theme derived from it. It reads no files and starts no
// processes, so the desktop app and the web build share it; the app fills
// it from Omarchy's colors.toml.
package palette

import (
	"image/color"

	"dyamark/seq"
)

// Theme holds the page's palette, the desktop's corner rounding, and the
// diagram theme derived from them.
type Theme struct {
	Background        color.NRGBA
	Foreground        color.NRGBA
	Accent            color.NRGBA
	LighterBackground color.NRGBA
	Cursor            color.NRGBA
	SelectionBg       color.NRGBA
	SelectionFg       color.NRGBA
	Colors            [16]color.NRGBA
	Rounding          int

	Seq seq.Theme
}

// Default returns the fallback dark palette, with the #FFD60A accent.
func Default(rounding int) *Theme {
	t := &Theme{
		Background:        seq.HexColor("#0b0f19", 255),
		Foreground:        seq.HexColor("#e6edf3", 255),
		Accent:            seq.HexColor("#FFD60A", 255),
		LighterBackground: seq.HexColor("#1f242c", 255),
		Cursor:            seq.HexColor("#e6edf3", 255),
		SelectionBg:       seq.HexColor("#2f3746", 255),
		SelectionFg:       seq.HexColor("#ffffff", 255),
		Rounding:          rounding,
	}
	t.DeriveSeq()
	return t
}

// DeriveSeq sets t.Seq from the page colours, the way Mermaid's base theme
// derives actor, line, note and frame colours from its themeVariables.
func (t *Theme) DeriveSeq() {
	// Notes use the theme's yellow, falling back to amber.
	noteAccent := seq.HexColor("#ffd166", 255)
	if t.Colors[3].A != 0 {
		noteAccent = t.Colors[3]
	}
	if t.Colors[11].A != 0 {
		noteAccent = t.Colors[11]
	}

	t.Seq = seq.Theme{
		ActorBkg:         t.LighterBackground,
		ActorBorder:      t.Accent,
		ActorText:        t.Foreground,
		Lifeline:         WithAlpha(t.Foreground, 110),
		Signal:           t.Accent,
		SignalText:       t.Foreground,
		ActivationBkg:    WithAlpha(t.Accent, 60),
		ActivationBorder: t.Accent,
		// Mixed from the page colour, so notes read on light themes too.
		NoteBkg:     Mix(t.Background, noteAccent, 0.18),
		NoteBorder:  noteAccent,
		NoteText:    t.Foreground,
		FrameBkg:    WithAlpha(t.LighterBackground, 90),
		FrameBorder: WithAlpha(t.Accent, 180),
		// The tab is the faint frame fill, so both read in the text colour.
		FrameText: t.Foreground,
		LabelText: t.Foreground,
		Rounding:  float32(t.Rounding),
	}
}

// Error is the theme's red, or a plain red when it has none.
func (t *Theme) Error() color.NRGBA {
	if t.Colors[1].A != 0 {
		return t.Colors[1]
	}
	return color.NRGBA{R: 0xf8, G: 0x51, B: 0x49, A: 0xff}
}

// WithAlpha returns c with alpha a.
func WithAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

// Mix blends b into a by t (0 gives a, 1 gives b), keeping a's alpha.
func Mix(a, b color.NRGBA, t float32) color.NRGBA {
	lerp := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return color.NRGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: a.A}
}
