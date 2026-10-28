package app

import (
	"image"
	"testing"
	"time"

	"dyamark/fonts"
	layoutPkg "dyamark/seq/layout"
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

// gtxNow is the time of the frames viewerFrames lays out.
var gtxNow time.Time

// viewerFrames returns a router feeding v and a function that lays out one
// frame of it at size, at gtxNow.
func viewerFrames(t *testing.T, v *Viewer, size image.Point) (*input.Router, func()) {
	t.Helper()
	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	r := new(input.Router)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size),
		Source:      r.Source(),
	}
	return r, func() {
		gtx.Reset()
		gtx.Now = gtxNow
		v.Layout(gtx, th, DefaultTheme(10))
		r.Frame(gtx.Ops)
	}
}

// onScreen returns the middle of the actor's box on screen.
func onScreen(v *Viewer, a layoutPkg.Actor) f32.Point {
	vb := v.Diagram.ViewBox
	return f32.Pt(v.Pan.X+(a.Rect.X+a.Rect.W/2-vb.X)*v.Zoom, v.Pan.Y+(a.Rect.Y+a.Rect.H/2-vb.Y)*v.Zoom)
}

// click presses and releases the primary button at from, moving to to in
// between.
func click(r *input.Router, frame func(), from, to f32.Point) {
	r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: from})
	frame()
	if to != from {
		r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: to})
		frame()
	}
	r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: to})
	frame()
}

func viewerOf(t *testing.T, src string) *Viewer {
	t.Helper()
	d, err := parse.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return NewViewer([]*parse.Diagram{d}, 0)
}

// Clicking a participant focuses it; clicking it again, or Esc, shows all
// again, and only a second Esc closes the viewer. A drag pans and leaves
// the focus alone.
func TestViewerFocusParticipant(t *testing.T) {
	v := viewerOf(t, sampleMMD1)
	closed := false
	v.OnClose = func() { closed = true }
	r, frame := viewerFrames(t, v, image.Pt(800, 600))
	frame()

	bob := v.Diagram.Actors[1]
	if bob.ID != "Bob" {
		t.Fatalf("second actor is %q", bob.ID)
	}
	at := onScreen(v, bob)
	click(r, frame, at, at)
	if v.Focus != "Bob" {
		t.Fatalf("focus %q after clicking Bob", v.Focus)
	}

	pan := v.Pan
	click(r, frame, at, at.Add(f32.Pt(60, 0)))
	if v.Focus != "Bob" || v.Pan == pan {
		t.Errorf("a drag changed the focus to %q, or didn't pan", v.Focus)
	}

	click(r, frame, onScreen(v, bob), onScreen(v, bob))
	if v.Focus != "" {
		t.Errorf("clicking Bob again left focus %q", v.Focus)
	}

	click(r, frame, onScreen(v, bob), onScreen(v, bob))
	r.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()
	if v.Focus != "" || closed {
		t.Fatalf("Esc: focus %q, closed %v", v.Focus, closed)
	}
	r.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()
	if !closed {
		t.Error("second Esc didn't close the viewer")
	}
}

// The focus lasts through a save that keeps the participant, and goes
// when it's gone or another diagram is shown.
func TestViewerFocusThroughReload(t *testing.T) {
	d1, _ := parse.Parse(sampleMMD1)
	d2, _ := parse.Parse(sampleMMD1 + "\nAlice->>Bob: again")
	d3, _ := parse.Parse("sequenceDiagram\nAlice->>Carol: hi")
	v := NewViewer([]*parse.Diagram{d1}, 0)
	v.Focus = "Bob"
	v.ReloadDiagrams([]*parse.Diagram{d2}, 0)
	if v.Focus != "Bob" {
		t.Error("a save dropped the focus")
	}
	v.ReloadDiagrams([]*parse.Diagram{d3}, 0)
	if v.Focus != "" {
		t.Error("focus kept on a participant that's gone")
	}
	v.Focus = "Alice"
	v.SetDiagrams([]*parse.Diagram{d1, d3}, 1)
	if v.Focus != "" {
		t.Error("focus kept on another diagram")
	}
}

// Scrolled down a tall diagram, the participants' names are pinned at the
// top in a strip as tall at any zoom, and clicking a pinned name focuses
// its participant. A participant created part way down isn't in the row.
func TestStickyRow(t *testing.T) {
	src := "sequenceDiagram\nparticipant A\nparticipant B\n"
	for range 30 {
		src += "A->>B: ping\n"
	}
	src += "create participant C\nB->>C: new\n"
	v := viewerOf(t, src)
	r, frame := viewerFrames(t, v, image.Pt(1200, 400))
	frame()
	v.Set100(1200, 400)
	v.Pan.Y = 20
	if _, _, ok := v.stickyRow(); ok {
		t.Fatal("pinned while the row is on screen")
	}

	var heights []float32
	for _, zoom := range []float32{0.5, 1, 4} {
		v.Zoom = zoom
		// A's lifeline at x 50, B's on screen to its right.
		a := v.Diagram.Actors[0]
		v.Pan = f32.Pt(50-(a.Rect.X+a.Rect.W/2-v.Diagram.ViewBox.X)*zoom, -600*zoom)
		frame()
		row, h, ok := v.stickyRow()
		if !ok || len(row) != 2 || len(v.chips) != 2 {
			t.Fatalf("zoom %v: ok %v, %d actors, %d names", zoom, ok, len(row), len(v.chips))
		}
		heights = append(heights, h)
		for i, c := range v.chips {
			mid := (c.rect.Min.X + c.rect.Max.X) / 2
			if x := int(v.lifelineX(row[i])); mid < x-1 || mid > x+1 || c.rect.Max.Y > int(h) {
				t.Errorf("zoom %v: %s's name at %v, its lifeline at %d, strip %v high", zoom, c.id, c.rect, x, h)
			}
		}
	}
	if heights[0] != stickyHeight || heights[1] != heights[0] || heights[2] != heights[0] {
		t.Errorf("strip heights %v at 50, 100 and 400 %%, want %d", heights, stickyHeight)
	}

	b := v.chips[1].rect
	at := f32.Pt(float32(b.Min.X+b.Max.X)/2, float32(b.Min.Y+b.Max.Y)/2)
	click(r, frame, at, at)
	if v.Focus != "B" {
		t.Errorf("clicking pinned B focused %q", v.Focus)
	}
	// Beside a name, the strip still covers the diagram.
	beside := f32.Pt(float32(b.Max.X+4), at.Y)
	click(r, frame, beside, beside)
	if v.Focus != "" {
		t.Errorf("clicking beside pinned B focused %q", v.Focus)
	}
}

// Zoomed out, a long name is cut short to the room between lifelines, so
// the names don't overlap, and the pointer on it shows it in full.
func TestStickyRowLongNames(t *testing.T) {
	src := "sequenceDiagram\nparticipant A as Authentication Service Gateway\nparticipant B as Billing and Invoicing Backend\nparticipant C\n"
	for range 40 {
		src += "A->>B: ping\nB->>C: pong\n"
	}
	v := viewerOf(t, src)
	r, frame := viewerFrames(t, v, image.Pt(800, 400))
	frame()
	v.Zoom = 0.3
	v.Pan = f32.Pt(20, -300)
	frame()
	if len(v.chips) != 3 {
		t.Fatalf("%d names pinned", len(v.chips))
	}
	for i := 1; i < len(v.chips); i++ {
		if v.chips[i-1].rect.Max.X > v.chips[i].rect.Min.X {
			t.Errorf("%s's name %v runs into %s's %v", v.chips[i-1].id, v.chips[i-1].rect, v.chips[i].id, v.chips[i].rect)
		}
	}
	a := v.chips[0]
	if !a.cut || a.label != "Authentication Service Gateway" || v.chips[2].cut {
		t.Errorf("cut: A %v (%q), C %v", a.cut, a.label, v.chips[2].cut)
	}
	// Drawn with the pointer on A's name, the full name shows under the
	// strip; that it's drawn without a panic is what's checked here.
	mid := f32.Pt(float32(a.rect.Min.X+a.rect.Max.X)/2, float32(a.rect.Min.Y+a.rect.Max.Y)/2)
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: mid})
	frame()
	if v.pointerPos != mid {
		t.Errorf("pointer at %v, want %v", v.pointerPos, mid)
	}
}

// The info line names the diagram, its place among the file's, the zoom
// and the key list; it shows when a diagram opens, fades, and comes back
// on a key.
func TestViewerInfo(t *testing.T) {
	d1, _ := parse.Parse("sequenceDiagram\ntitle Checkout flow\nA->>B: hi")
	d2, _ := parse.Parse("sequenceDiagram\nA->>B: hi")
	v := NewViewer([]*parse.Diagram{d1, d2}, 0)
	v.Zoom = 1.5
	if got := v.infoText(); got != "Checkout flow · 1 of 2 · 150 % · ? for keys" {
		t.Errorf("info %q", got)
	}
	v.NextDiagram()
	v.Zoom = 1
	if got := v.infoText(); got != "Diagram · 2 of 2 · 100 % · ? for keys" {
		t.Errorf("info %q", got)
	}

	r, frame := viewerFrames(t, v, image.Pt(800, 600))
	start := time.Now()
	at := func(d time.Duration) {
		gtxNow = start.Add(d)
		frame()
	}
	at(0)
	if a := v.infoAlpha(start); a != 1 {
		t.Errorf("opened: alpha %f", a)
	}
	if a := v.infoAlpha(start.Add(infoShow + infoFade/2)); a <= 0 || a >= 1 {
		t.Errorf("fading: alpha %f", a)
	}
	if a := v.infoAlpha(start.Add(5 * time.Second)); a != 0 {
		t.Errorf("later: alpha %f", a)
	}
	r.Queue(key.Event{Name: "N", State: key.Press})
	at(5 * time.Second)
	if a := v.infoAlpha(start.Add(5 * time.Second)); a != 1 {
		t.Errorf("after a key: alpha %f", a)
	}
}
