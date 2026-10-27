package app

import (
	"image"
	"testing"

	"dyamark/fonts"
	"dyamark/seq/parse"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const sampleMMD1 = `sequenceDiagram
Alice->>Bob: Hello Bob
Bob-->>Alice: Hi Alice`

const sampleMMD2 = `sequenceDiagram
autonumber
Client->>Server: Request
Server-->>Client: Response`

func TestViewerZoomAndPan(t *testing.T) {
	d1, err := parse.Parse(sampleMMD1)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := parse.Parse(sampleMMD2)
	if err != nil {
		t.Fatal(err)
	}

	v := NewViewer([]*parse.Diagram{d1, d2}, 0)
	if v.Diagram == nil {
		t.Fatal("expected laid-out diagram")
	}

	winW, winH := 800, 600
	v.Fit(winW, winH)
	initialZoom := v.Zoom
	if initialZoom <= 0 {
		t.Fatalf("expected positive zoom, got %f", initialZoom)
	}

	// Zoom in centered at (400, 300)
	center := f32.Pt(400, 300)
	v.ZoomBy(1.5, center)
	if v.Zoom <= initialZoom {
		t.Errorf("expected zoom to increase, got %f <= %f", v.Zoom, initialZoom)
	}

	// Zoom clamp
	v.ZoomBy(100.0, center)
	if v.Zoom > 20.0 {
		t.Errorf("expected zoom clamped to 20, got %f", v.Zoom)
	}
	v.ZoomBy(0.0001, center)
	if v.Zoom < 0.05 {
		t.Errorf("expected zoom clamped to 0.05, got %f", v.Zoom)
	}

	// FitWidth & FitHeight
	v.FitWidth(winW, winH)
	if v.Zoom <= 0 {
		t.Errorf("fitWidth invalid zoom: %f", v.Zoom)
	}
	v.FitHeight(winW, winH)
	if v.Zoom <= 0 {
		t.Errorf("fitHeight invalid zoom: %f", v.Zoom)
	}

	// 100%
	v.Set100(winW, winH)
	if v.Zoom != 1.0 {
		t.Errorf("expected 1.0 zoom, got %f", v.Zoom)
	}
}

func TestViewerNavigationAndToggle(t *testing.T) {
	d1, _ := parse.Parse(sampleMMD1)
	d2, _ := parse.Parse(sampleMMD2)

	v := NewViewer([]*parse.Diagram{d1, d2}, 0)

	// Navigation
	if v.CurIndex != 0 {
		t.Errorf("expected index 0, got %d", v.CurIndex)
	}
	v.NextDiagram()
	if v.CurIndex != 1 {
		t.Errorf("expected index 1, got %d", v.CurIndex)
	}
	v.NextDiagram()
	if v.CurIndex != 0 {
		t.Errorf("expected wrap around to 0, got %d", v.CurIndex)
	}
	v.PrevDiagram()
	if v.CurIndex != 1 {
		t.Errorf("expected wrap around backwards to 1, got %d", v.CurIndex)
	}

	// Toggle sequence numbers
	initialSeqNum := v.Config.ShowSequenceNumbers
	v.ToggleSequenceNumbers()
	if v.Config.ShowSequenceNumbers == initialSeqNum {
		t.Errorf("sequence numbers should have toggled")
	}
}

func TestViewerLayout(t *testing.T) {
	d1, _ := parse.Parse(sampleMMD1)
	v := NewViewer([]*parse.Diagram{d1}, 0)

	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	mTheme := material.NewTheme()
	mTheme.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	oTheme := DefaultTheme(10)

	ops := new(op.Ops)
	gtx := layout.Context{
		Ops: ops,
		Metric: unit.Metric{
			PxPerDp: 1.0,
			PxPerSp: 1.0,
		},
		Constraints: layout.Exact(image.Pt(800, 600)),
	}

	dims := v.Layout(gtx, mTheme, oTheme)
	if dims.Size != image.Pt(800, 600) {
		t.Errorf("expected layout size (800, 600), got %v", dims.Size)
	}
}

func TestViewerScrollZoomAndDragPan(t *testing.T) {
	d1, err := parse.Parse(sampleMMD1)
	if err != nil {
		t.Fatal(err)
	}
	v := NewViewer([]*parse.Diagram{d1}, 0)

	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	mTheme := material.NewTheme()
	mTheme.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	oTheme := DefaultTheme(10)

	var r input.Router
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops: ops,
		Metric: unit.Metric{
			PxPerDp: 1.0,
			PxPerSp: 1.0,
		},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Source:      r.Source(),
	}

	frame := func() {
		gtx.Reset()
		v.Layout(gtx, mTheme, oTheme)
		r.Frame(gtx.Ops)
	}

	// Frame 1: initial render & fit
	frame()
	initialZoom := v.Zoom

	// Test 1: Scroll UP -> Zoom in
	r.Queue(pointer.Event{
		Kind:     pointer.Scroll,
		Source:   pointer.Mouse,
		Position: f32.Pt(400, 300),
		Scroll:   f32.Pt(0, -100), // scroll up
	})
	frame()
	if v.Zoom <= initialZoom {
		t.Errorf("expected zoom to increase on scroll up, got %f <= %f", v.Zoom, initialZoom)
	}

	// Test 2: Scroll DOWN -> Zoom out
	zoomAfterZoomIn := v.Zoom
	r.Queue(pointer.Event{
		Kind:     pointer.Scroll,
		Source:   pointer.Mouse,
		Position: f32.Pt(400, 300),
		Scroll:   f32.Pt(0, 100), // scroll down
	})
	frame()
	if v.Zoom >= zoomAfterZoomIn {
		t.Errorf("expected zoom to decrease on scroll down, got %f >= %f", v.Zoom, zoomAfterZoomIn)
	}

	// Test 3: Drag / Move tool
	panBeforeDrag := v.Pan
	// Press at (400, 300)
	r.Queue(pointer.Event{
		Kind:      pointer.Press,
		Source:    pointer.Mouse,
		Buttons:   pointer.ButtonPrimary,
		Position:  f32.Pt(400, 300),
		PointerID: 1,
	})
	frame()

	// Drag to (450, 370) -> delta (+50, +70)
	r.Queue(pointer.Event{
		Kind:      pointer.Move, // becomes pointer.Drag in Gio router
		Source:    pointer.Mouse,
		Buttons:   pointer.ButtonPrimary,
		Position:  f32.Pt(450, 370),
		PointerID: 1,
	})
	frame()

	expectedPan := panBeforeDrag.Add(f32.Pt(50, 70))
	diffX := v.Pan.X - expectedPan.X
	diffY := v.Pan.Y - expectedPan.Y
	if diffX < -1 || diffX > 1 || diffY < -1 || diffY > 1 {
		t.Errorf("expected pan to move by (+50, +70) to %v, got %v", expectedPan, v.Pan)
	}

	// Release
	r.Queue(pointer.Event{
		Kind:      pointer.Release,
		Source:    pointer.Mouse,
		Position:  f32.Pt(450, 370),
		PointerID: 1,
	})
	frame()
}
