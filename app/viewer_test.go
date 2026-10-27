package app

import (
	"image"
	"testing"

	"dyamark/fonts"
	"dyamark/seq/parse"

	"gioui.org/f32"
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
