package markdown

import (
	"image"
	"testing"

	"dyamark/fonts"
	"dyamark/seq"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// testThemes returns a material theme with the bundled fonts and a
// Markdown theme.
func testThemes(t *testing.T) (*material.Theme, *Theme) {
	t.Helper()
	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	return th, &Theme{
		Background:        seq.HexColor("#0b0f19", 255),
		Foreground:        seq.HexColor("#e6edf3", 255),
		Accent:            seq.HexColor("#ffd60a", 255),
		LighterBackground: seq.HexColor("#1f242c", 255),
		Seq:               seq.DefaultTheme(),
	}
}

// viewFrames returns a router feeding v and a function that lays out one
// 1000×800 frame of it.
func viewFrames(t *testing.T, v *View) (*input.Router, func()) {
	th, mTheme := testThemes(t)
	r := new(input.Router)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1000, 800)),
		Source:      r.Source(),
	}
	return r, func() {
		gtx.Reset()
		v.Layout(gtx, th, mTheme)
		r.Frame(gtx.Ops)
	}
}

// Ctrl+`+` and Ctrl+`_` are typed with Shift, and Gio reports it; they
// still zoom the text.
func TestViewShiftedZoomKeys(t *testing.T) {
	v := NewView(Parse([]byte("# Title\n"), nil))
	r, frame := viewFrames(t, v)
	frame()

	r.Queue(key.Event{Name: "+", Modifiers: key.ModCtrl | key.ModShift, State: key.Press})
	frame()
	if v.TextScale <= 1 {
		t.Errorf("Ctrl+Shift++ didn't zoom in: %f", v.TextScale)
	}

	v.ResetTextZoom()
	r.Queue(key.Event{Name: "_", Modifiers: key.ModCtrl | key.ModShift, State: key.Press})
	frame()
	if v.TextScale >= 1 {
		t.Errorf("Ctrl+Shift+_ didn't zoom out: %f", v.TextScale)
	}
}
