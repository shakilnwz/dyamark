package draw_test

import (
	"image"
	"image/color"
	"os"
	"strings"
	"testing"

	"dyamark/fonts"
	"dyamark/seq"
	"dyamark/seq/draw"
	layoutPkg "dyamark/seq/layout"
	"dyamark/seq/parse"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// A cached drawing, kept from one frame and moved in the next, draws
// the same pixels as drawing it there afresh, without drawing it again;
// a new key draws it again.
func TestCacheDrawsTheSame(t *testing.T) {
	const w, h = 900, 700
	win, err := headless.NewWindow(w, h)
	if err != nil {
		t.Skipf("no headless GPU: %v", err)
	}
	defer win.Release()

	data, err := os.ReadFile("../testdata/32-complex-auth.mmd")
	if err != nil {
		t.Fatal(err)
	}
	ast, err := parse.Parse(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	diag, err := layoutPkg.Layout(ast, layoutPkg.Default())
	if err != nil {
		t.Fatal(err)
	}
	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	seqTh := seq.DefaultTheme()

	ops := new(op.Ops)
	frame := func(at image.Point, draw func(gtx layout.Context)) *image.RGBA {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(int(diag.ViewBox.W), int(diag.ViewBox.H))),
		}
		paint.FillShape(ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: image.Pt(w, h)}.Op())
		off := op.Offset(at).Push(ops)
		draw(gtx)
		off.Pop()
		if err := win.Frame(ops); err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		if err := win.Screenshot(img); err != nil {
			t.Fatal(err)
		}
		return img
	}

	at := image.Pt(37, 21)
	direct := frame(at, func(gtx layout.Context) { draw.Draw(gtx, th, seqTh, diag) })

	var c draw.Cache
	drawn := 0
	cached := func(key any) func(gtx layout.Context) {
		return func(gtx layout.Context) {
			c.Layout(gtx, key, func(gtx layout.Context) layout.Dimensions {
				drawn++
				return draw.Draw(gtx, th, seqTh, diag)
			})
		}
	}
	frame(image.Pt(0, 0), cached(1))
	moved := frame(at, cached(1))
	if drawn != 1 {
		t.Errorf("drawn %d times for one key, want once", drawn)
	}
	if n := differ(direct, moved); n > 0 {
		t.Errorf("the kept drawing, moved, differs from a fresh one in %d px", n)
	}
	frame(at, cached(2))
	if drawn != 2 {
		t.Errorf("a new key didn't draw again: drawn %d times", drawn)
	}

	// The drawing isn't blank, so the comparison means something.
	blank := frame(at, func(layout.Context) {})
	if differ(direct, blank) == 0 {
		t.Error("the diagram drew nothing")
	}
}

// differ returns how many pixels of a and b differ.
func differ(a, b *image.RGBA) int {
	n := 0
	for i := 0; i < len(a.Pix); i += 4 {
		if [4]byte(a.Pix[i:i+4]) != [4]byte(b.Pix[i:i+4]) {
			n++
		}
	}
	return n
}
