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

	// Test 1: Ctrl+scroll UP -> Zoom in
	r.Queue(pointer.Event{
		Kind:      pointer.Scroll,
		Source:    pointer.Mouse,
		Position:  f32.Pt(400, 300),
		Scroll:    f32.Pt(0, -100), // scroll up
		Modifiers: key.ModCtrl,
	})
	frame()
	if v.Zoom <= initialZoom {
		t.Errorf("expected zoom to increase on scroll up, got %f <= %f", v.Zoom, initialZoom)
	}

	// Test 2: Ctrl+scroll DOWN -> Zoom out
	zoomAfterZoomIn := v.Zoom
	r.Queue(pointer.Event{
		Kind:      pointer.Scroll,
		Source:    pointer.Mouse,
		Position:  f32.Pt(400, 300),
		Scroll:    f32.Pt(0, 100), // scroll down
		Modifiers: key.ModCtrl,
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

// Scrolling moves the diagram the way the reading view moves its page, on
// either axis, and leaves the zoom. Small steps move it at once.
func TestScrollMovesTheDiagram(t *testing.T) {
	v, r, frame := clockedViewer(t)
	v.Zoom = 10
	v.Pan = f32.Pt(-100, -100)
	r.Queue(wheel(f32.Pt(0, 30), 0))
	frame(10 * time.Millisecond)
	if want := f32.Pt(-100, -130); !near(v.Pan, want) {
		t.Errorf("scrolling down put the pan at %v, want %v", v.Pan, want)
	}
	r.Queue(wheel(f32.Pt(30, 0), 0))
	frame(20 * time.Millisecond)
	if want := f32.Pt(-130, -130); !near(v.Pan, want) {
		t.Errorf("scrolling right put the pan at %v, want %v", v.Pan, want)
	}
	if v.Zoom != 10 {
		t.Errorf("scrolling changed the zoom to %v", v.Zoom)
	}
}

// Ctrl+scrolling zooms at the pointer and doesn't scroll as well.
func TestCtrlScrollZoomsAtThePointer(t *testing.T) {
	v, r, frame := clockedViewer(t)
	v.Zoom, v.Pan = 10, f32.Pt(-100, -100)
	r.Queue(wheel(f32.Pt(0, -wheelNotch), key.ModCtrl))
	frame(10 * time.Millisecond)
	if want := float32(10 * wheelStep); math.Abs(float64(v.Zoom-want)) > 1e-3 {
		t.Errorf("a notch up zoomed to %v, want %v", v.Zoom, want)
	}
	// The diagram's point under the pointer stays there.
	at := f32.Pt(400, 300)
	if want := at.Sub(at.Sub(f32.Pt(-100, -100)).Mul(wheelStep)); !near(v.Pan, want) {
		t.Errorf("the pan went to %v, want %v", v.Pan, want)
	}
}

// A wheel's notch eases to where it goes, and quick ones add up.
func TestWheelNotchEases(t *testing.T) {
	v, r, frame := clockedViewer(t)
	v.Zoom, v.Pan = 10, f32.Pt(-100, -100)
	r.Queue(wheel(f32.Pt(0, 120), 0))
	frame(10 * time.Millisecond)
	frame(60 * time.Millisecond)
	if v.Pan.Y >= -100 || v.Pan.Y <= -220 {
		t.Errorf("part way through a notch the pan is at %v", v.Pan.Y)
	}
	r.Queue(wheel(f32.Pt(0, 120), 0))
	frame(70 * time.Millisecond)
	frame(500 * time.Millisecond)
	if want := f32.Pt(-100, -340); !near(v.Pan, want) {
		t.Errorf("two notches put the pan at %v, want %v", v.Pan, want)
	}
}

// Scrolling stops at the diagram's edges, on each axis of its own.
func TestScrollStopsAtTheEdges(t *testing.T) {
	v, r, frame := clockedViewer(t)
	v.Zoom = 10
	w, h := v.Diagram.ViewBox.W*10, v.Diagram.ViewBox.H*10
	v.Pan = f32.Pt(fitPad-10, fitPad-10)
	r.Queue(wheel(f32.Pt(-50, -50), 0))
	frame(10 * time.Millisecond)
	if want := f32.Pt(fitPad, fitPad); !near(v.Pan, want) {
		t.Errorf("scrolling past the top left put the pan at %v, want %v", v.Pan, want)
	}
	v.Pan = f32.Pt(800-w-fitPad+10, 600-h-fitPad+10)
	r.Queue(wheel(f32.Pt(50, 50), 0))
	frame(20 * time.Millisecond)
	if want := f32.Pt(800-w-fitPad, 600-h-fitPad); !near(v.Pan, want) {
		t.Errorf("scrolling past the bottom right put the pan at %v, want %v", v.Pan, want)
	}
}

// A diagram fitted to the window has nothing to scroll.
func TestScrollLeavesAFittedDiagram(t *testing.T) {
	v, r, frame := clockedViewer(t)
	pan := v.Pan
	r.Queue(wheel(f32.Pt(30, 30), 0))
	frame(10 * time.Millisecond)
	r.Queue(wheel(f32.Pt(0, 120), 0))
	frame(20 * time.Millisecond)
	frame(500 * time.Millisecond)
	if !near(v.Pan, pan) {
		t.Errorf("scrolling moved a fitted diagram from %v to %v", pan, v.Pan)
	}
}

func TestScrollAxis(t *testing.T) {
	const win, long, short = 600, 1000, 200
	tests := []struct {
		name           string
		pan, d, length float32
		want           float32
	}{
		{"moves back as it scrolls", -100, 50, long, -150},
		{"stops at the far edge", win - long - fitPad + 10, 50, long, win - long - fitPad},
		{"stops at the near edge", fitPad - 10, -50, long, fitPad},
		{"dragged past the far edge stays", -2000, 50, long, -2000},
		{"dragged past the far edge comes back", -2000, -50, long, -1950},
		{"dragged past the near edge stays", 900, -50, long, 900},
		{"dragged past the near edge comes back", 900, 50, long, 850},
		{"fits: stays", 100, 50, short, 100},
		{"fits, off the top: comes back", -100, -50, short, -50},
		{"fits, off the bottom: comes back", 900, 50, short, 850},
		{"fits, nearly back: stops inside", 20, -50, short, 70},
	}
	for _, tt := range tests {
		if got := scrollAxis(tt.pan, tt.d, tt.length, win); got != tt.want {
			t.Errorf("%s: scrollAxis(%v, %v, %v, %v) = %v, want %v", tt.name, tt.pan, tt.d, tt.length, float32(win), got, tt.want)
		}
	}
}

// wheel is d px of scrolling at the middle of the window.
func wheel(d f32.Point, mods key.Modifiers) pointer.Event {
	return pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(400, 300), Scroll: d, Modifiers: mods}
}

// The wheel cuts an easing short: it zooms from what's shown.
func TestWheelStopsTheEasing(t *testing.T) {
	v, r, frame := clockedViewer(t)
	r.Queue(key.Event{Name: "1", State: key.Press})
	frame(10 * time.Millisecond)
	frame(60 * time.Millisecond)
	shown := v.Zoom
	r.Queue(wheel(f32.Pt(0, -wheelNotch), key.ModCtrl))
	frame(70 * time.Millisecond)
	frame(500 * time.Millisecond)
	if want := shown * wheelStep; math.Abs(float64(v.Zoom-want)) > 1e-4 {
		t.Errorf("a notch during the easing zoomed to %v, want %v", v.Zoom, want)
	}
}
