package viewer

import (
	"image"
	"math"
	"testing"
	"time"

	"dyamark/fonts"
	"dyamark/palette"
	"dyamark/seq/parse"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
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

	v := New([]*parse.Diagram{d1, d2}, 0)
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

	v := New([]*parse.Diagram{d1, d2}, 0)

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
	v := New([]*parse.Diagram{d1}, 0)

	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	mTheme := material.NewTheme()
	mTheme.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	oTheme := palette.Default(10)

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
	v := New([]*parse.Diagram{d1}, 0)

	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	mTheme := material.NewTheme()
	mTheme.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	oTheme := palette.Default(10)

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

// `+` and `_` are typed with Shift, and Gio reports it; they still zoom.
func TestViewerShiftedZoomKeys(t *testing.T) {
	d1, err := parse.Parse(sampleMMD1)
	if err != nil {
		t.Fatal(err)
	}
	v := New([]*parse.Diagram{d1}, 0)

	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	mTheme := material.NewTheme()
	mTheme.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	oTheme := palette.Default(10)

	var r input.Router
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Source:      r.Source(),
	}
	frame := func() {
		gtx.Reset()
		v.Layout(gtx, mTheme, oTheme)
		r.Frame(gtx.Ops)
	}
	frame()

	zoom := v.Zoom
	r.Queue(key.Event{Name: "+", Modifiers: key.ModShift, State: key.Press})
	frame()
	if v.Zoom <= zoom {
		t.Errorf("Shift++ didn't zoom in: %f <= %f", v.Zoom, zoom)
	}

	zoom = v.Zoom
	r.Queue(key.Event{Name: "_", Modifiers: key.ModShift, State: key.Press})
	frame()
	if v.Zoom >= zoom {
		t.Errorf("Shift+_ didn't zoom out: %f >= %f", v.Zoom, zoom)
	}
}

// The wheel zooms in proportion to how far it scrolls, up to a notch's
// step: ten small touchpad steps zoom as far as one notch, and a big
// notch no further than before.
func TestWheelZoomFollowsTheScroll(t *testing.T) {
	step := wheelZoom(-wheelNotch / 10)
	if step <= 1 || step > 1.02 {
		t.Errorf("a tenth of a notch up zooms by %v", step)
	}
	total := float32(1)
	for range 10 {
		total *= step
	}
	if math.Abs(float64(total-wheelStep)) > 1e-4 {
		t.Errorf("ten tenths of a notch zoom by %v, want %v", total, wheelStep)
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
	if got := wheelZoom(-120); !near(float64(got), wheelStep) {
		t.Errorf("a 120 dp notch up zooms by %v, want %v", got, wheelStep)
	}
	if got := wheelZoom(120); !near(float64(got), 1/wheelStep) {
		t.Errorf("a 120 dp notch down zooms by %v, want %v", got, 1/wheelStep)
	}
	if got := wheelZoom(0); got != 1 {
		t.Errorf("no scrolling zooms by %v", got)
	}
}

// clockedViewer returns a viewer of sampleMMD1 laid out once, 800×600 at
// time 0, a router feeding it, and a function laying out a frame at a
// time from then.
func clockedViewer(t *testing.T) (*Viewer, *input.Router, func(at time.Duration)) {
	t.Helper()
	d, err := parse.Parse(sampleMMD1)
	if err != nil {
		t.Fatal(err)
	}
	v := New([]*parse.Diagram{d}, 0)
	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	oTheme := palette.Default(10)
	r := new(input.Router)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Source:      r.Source(),
	}
	frame := func(at time.Duration) {
		gtx.Reset()
		gtx.Now = start.Add(at)
		v.Layout(gtx, th, oTheme)
		r.Frame(gtx.Ops)
	}
	frame(0)
	return v, r, frame
}

func near(a, b f32.Point) bool {
	return math.Abs(float64(a.X-b.X)) < 0.01 && math.Abs(float64(a.Y-b.Y)) < 0.01
}

// The zoom keys ease to their zoom, keeping the point under the pointer
// still all the way, and quick presses add up.
func TestZoomKeysEase(t *testing.T) {
	v, r, frame := clockedViewer(t)
	at := f32.Pt(300, 200)
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: at})
	frame(10 * time.Millisecond)
	zoom := v.Zoom
	under := v.toDiagram(at)

	r.Queue(key.Event{Name: "=", State: key.Press})
	frame(20 * time.Millisecond)
	frame(80 * time.Millisecond)
	if v.Zoom <= zoom || v.Zoom >= zoom*1.2 {
		t.Errorf("60 ms into easing to %v the zoom is %v, from %v", zoom*1.2, v.Zoom, zoom)
	}
	if got := v.toDiagram(at); !near(got, under) {
		t.Errorf("easing moved the point under the pointer from %v to %v", under, got)
	}

	// A second press while the first eases goes on from where it's going.
	r.Queue(key.Event{Name: "=", State: key.Press})
	frame(100 * time.Millisecond)
	frame(400 * time.Millisecond)
	if want := zoom * 1.2 * 1.2; math.Abs(float64(v.Zoom-want)) > 1e-4 {
		t.Errorf("two quick presses zoomed to %v, want %v", v.Zoom, want)
	}
	if got := v.toDiagram(at); !near(got, under) {
		t.Errorf("after easing the point under the pointer is %v, was %v", got, under)
	}
}

// Fit eases back to the fitted view and ends exactly on it.
func TestFitEases(t *testing.T) {
	v, r, frame := clockedViewer(t)
	fitZoom, fitPan := v.Zoom, v.Pan
	v.ZoomBy(3, f32.Pt(100, 100))
	frame(10 * time.Millisecond)

	r.Queue(key.Event{Name: "0", State: key.Press})
	frame(20 * time.Millisecond)
	frame(100 * time.Millisecond)
	if v.Zoom <= fitZoom || v.Zoom >= 3*fitZoom {
		t.Errorf("easing to fit, the zoom is %v, between %v and %v", v.Zoom, 3*fitZoom, fitZoom)
	}
	frame(300 * time.Millisecond)
	if v.Zoom != fitZoom || !near(v.Pan, fitPan) {
		t.Errorf("fit ended at zoom %v pan %v, want %v %v", v.Zoom, v.Pan, fitZoom, fitPan)
	}
}

// StepZoom, the IDE's zoom actions, zooms at the middle of the viewer.
func TestStepZoomAtTheMiddle(t *testing.T) {
	v, _, frame := clockedViewer(t)
	middle := f32.Pt(400, 300)
	under := v.toDiagram(middle)
	zoom := v.Zoom
	v.StepZoom(1.2)
	frame(10 * time.Millisecond)
	frame(300 * time.Millisecond)
	if math.Abs(float64(v.Zoom-zoom*1.2)) > 1e-4 {
		t.Errorf("StepZoom(1.2) zoomed from %v to %v", zoom, v.Zoom)
	}
	if got := v.toDiagram(middle); !near(got, under) {
		t.Errorf("StepZoom moved the middle's point from %v to %v", under, got)
	}
}

// The wheel cuts an easing short: it zooms from what's shown.
func TestWheelStopsTheEasing(t *testing.T) {
	v, r, frame := clockedViewer(t)
	r.Queue(key.Event{Name: "1", State: key.Press})
	frame(10 * time.Millisecond)
	frame(60 * time.Millisecond)
	shown := v.Zoom
	r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(400, 300), Scroll: f32.Pt(0, -wheelNotch)})
	frame(70 * time.Millisecond)
	frame(500 * time.Millisecond)
	if want := shown * wheelStep; math.Abs(float64(v.Zoom-want)) > 1e-4 {
		t.Errorf("a notch during the easing zoomed to %v, want %v", v.Zoom, want)
	}
}
