package fonts

import (
	"image"
	"testing"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func TestBundledFonts(t *testing.T) {
	if len(RegularTTF) == 0 {
		t.Fatal("RegularTTF is empty")
	}
	if len(BoldTTF) == 0 {
		t.Fatal("BoldTTF is empty")
	}

	faces, err := Collection()
	if err != nil {
		t.Fatalf("Collection() returned error: %v", err)
	}

	if len(faces) != 5 {
		t.Fatalf("expected 5 font faces, got %d", len(faces))
	}
	if got := faces[2].Font.Style; got != font.Italic {
		t.Errorf("third face's style is %v, want italic", got)
	}
	if got := faces[3].Font; got.Style != font.Italic || got.Weight != font.Bold {
		t.Errorf("fourth face is %v, want bold italic", got)
	}
	if got := faces[4].Font.Typeface; got != Mono {
		t.Errorf("monospace face is %q, want %q", got, Mono)
	}
}

// In the monospace font a tab is four spaces wide, not a box one wide.
func TestMonoTab(t *testing.T) {
	faces, err := Collection()
	if err != nil {
		t.Fatal(err)
	}
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(faces), text.NoSystemFonts())
	width := func(s string) int {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(2000, 200)},
		}
		l := material.Label(th, 20, s)
		l.Font.Typeface = Mono
		return l.Layout(gtx).Size.X
	}
	// Rounding can make them a pixel apart.
	if tab, spaces := width("\tx"), width("    x"); tab < spaces-1 || tab > spaces+1 {
		t.Errorf("tab then x is %d px, four spaces then x %d", tab, spaces)
	}
}
