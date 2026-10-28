package draw_test

import (
	"image"
	"image/color"
	"testing"

	"dyamark/fonts"
	"dyamark/seq"
	"dyamark/seq/draw"
	layoutPkg "dyamark/seq/layout"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

var white = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

// render draws diag on white with the GPU and returns the pixels. It skips
// the test where there is no GPU to draw with.
func render(t *testing.T, seqTh seq.Theme, diag *layoutPkg.Diagram) *image.RGBA {
	t.Helper()
	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())

	w, h := int(diag.ViewBox.W), int(diag.ViewBox.H)
	win, err := headless.NewWindow(w, h)
	if err != nil {
		t.Skipf("no headless GPU: %v", err)
	}
	defer win.Release()

	ops := new(op.Ops)
	paint.Fill(ops, white)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, h)),
	}
	draw.Draw(gtx, th, seqTh, diag)
	if err := win.Frame(ops); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := win.Screenshot(img); err != nil {
		t.Fatal(err)
	}
	return img
}

// blank reports whether every pixel in r is white.
func blank(img *image.RGBA, r image.Rectangle) (image.Point, bool) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if c := img.RGBAAt(x, y); c.R != 255 || c.G != 255 || c.B != 255 {
				return image.Pt(x, y), false
			}
		}
	}
	return image.Point{}, true
}

func TestRoundedCornerStaysInside(t *testing.T) {
	th := seq.DefaultTheme()
	th.Rounding = 20
	img := render(t, th, &layoutPkg.Diagram{
		ViewBox: layoutPkg.Rect{W: 200, H: 150},
		Actors:  []layoutPkg.Actor{{Rect: layoutPkg.Rect{X: 60, Y: 40, W: 100, H: 60}}},
	})
	// Left of and above the box's top-left corner.
	if p, ok := blank(img, image.Rect(30, 20, 58, 60)); !ok {
		t.Errorf("rounded corner draws outside the box at %v", p)
	}
	if p, ok := blank(img, image.Rect(30, 20, 100, 38)); !ok {
		t.Errorf("rounded corner draws outside the box at %v", p)
	}
}

func TestDashedCurveStartsAtCurve(t *testing.T) {
	img := render(t, seq.DefaultTheme(), &layoutPkg.Diagram{
		ViewBox: layoutPkg.Rect{W: 300, H: 300},
		Messages: []layoutPkg.Message{{
			Dashed: true,
			Curved: true,
			Path:   []layoutPkg.Point{{X: 200, Y: 200}, {X: 260, Y: 200}, {X: 260, Y: 240}, {X: 200, Y: 240}},
		}},
	})
	// Nothing between the origin and the curve.
	if p, ok := blank(img, image.Rect(0, 0, 190, 190)); !ok {
		t.Errorf("dashed curve draws a stray line, at %v", p)
	}
}

func TestDrawStartsAtViewBoxOrigin(t *testing.T) {
	th := seq.DefaultTheme()
	img := render(t, th, &layoutPkg.Diagram{
		ViewBox: layoutPkg.Rect{X: -120, Y: -50, W: 300, H: 200},
		Notes:   []layoutPkg.Note{{Rect: layoutPkg.Rect{X: -100, Y: -40, W: 80, H: 30}}},
	})
	// The note spans (20,10)-(100,40) once shifted by the viewBox origin.
	if c := img.RGBAAt(60, 25); c.R != th.NoteBkg.R || c.G != th.NoteBkg.G || c.B != th.NoteBkg.B {
		t.Errorf("note not at the viewBox-relative position: pixel is %v", c)
	}
}

func TestFrameIsNotFilled(t *testing.T) {
	img := render(t, seq.DefaultTheme(), &layoutPkg.Diagram{
		ViewBox: layoutPkg.Rect{W: 300, H: 200},
		Frames:  []layoutPkg.Frame{{Rect: layoutPkg.Rect{X: 20, Y: 20, W: 200, H: 100}}},
	})
	if p, ok := blank(img, image.Rect(30, 30, 210, 110)); !ok {
		t.Errorf("frame body is filled, at %v", p)
	}
}
