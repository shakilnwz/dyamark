package markdown

import (
	"fmt"
	"image"
	"math"
	"slices"
	"strings"
	"testing"

	"dyamark/fonts"
	"dyamark/seq"

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

// Enter opens the focused diagram, or the first one on screen when none is
// focused.
func TestEnterOpensDiagram(t *testing.T) {
	doc := Parse([]byte("# Title\n\n"+
		fence+"mermaid\nsequenceDiagram\nA-\n"+fence+"\n\n"+
		fence+"mermaid\nsequenceDiagram\nA->>B: one\n"+fence+"\n\n"+
		fence+"mermaid\nsequenceDiagram\nC->>D: two\n"+fence+"\n"), nil)
	v := NewView(doc)
	var opened *SequenceBlock
	v.OnOpenViewer = func(b *SequenceBlock) { opened = b }
	r, frame := viewFrames(t, v)
	frame()

	enter := key.Event{Name: key.NameReturn, State: key.Press}
	r.Queue(enter)
	frame()
	if opened != doc.Sequences[1] {
		t.Errorf("Enter with nothing focused opened %v, want the first diagram that parses", opened)
	}

	v.FocusNext()
	v.FocusNext()
	v.FocusNext()
	r.Queue(enter)
	frame()
	if opened != doc.Sequences[2] {
		t.Errorf("Enter opened %v, want the focused diagram", opened)
	}
}

// R renders the Mermaid block under the pointer, or else the first one on
// screen that isn't rendered yet.
func TestRenderKey(t *testing.T) {
	doc := Parse([]byte(fence+"mermaid\nflowchart TD\nA-->B\n"+fence+"\n\n"+
		fence+"mermaid\npie\n\"a\": 1\n"+fence+"\n"), nil)
	first := doc.Blocks[0].(*MermaidOtherBlock)
	second := doc.Blocks[1].(*MermaidOtherBlock)
	v := NewView(doc)
	var rendered *MermaidOtherBlock
	v.OnRenderMermaid = func(b *MermaidOtherBlock) { rendered = b }
	r, frame := viewFrames(t, v)
	frame()

	rKey := key.Event{Name: "R", State: key.Press}
	first.Rendered = image.NewNRGBA(image.Rect(0, 0, 1, 1))
	r.Queue(rKey)
	frame()
	if rendered != second {
		t.Errorf("R rendered %v, want the first unrendered block", rendered)
	}

	// Point at the second block, below the first. The view centres an
	// 860 px column and leaves 12 px between blocks.
	first.Rendered = nil
	rendered = nil
	th, mTheme := testThemes(t)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Min: image.Pt(860, 0), Max: image.Pt(860, 1e6)},
	}
	firstH := first.Layout(gtx, th, mTheme).Size.Y
	frame()
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(500, float32(firstH+12+20))})
	frame()
	r.Queue(rKey)
	frame()
	if rendered != second {
		t.Errorf("R rendered %v, want the block under the pointer", rendered)
	}
}

// While the editor takes the typing, letters typed into it don't also
// fire the view's plain shortcuts; Ctrl shortcuts still work.
func TestNoPlainKeys(t *testing.T) {
	doc := Parse([]byte(fence+"mermaid\nflowchart TD\nA-->B\n"+fence+"\n"), nil)
	v := NewView(doc)
	v.NoPlainKeys = true
	rendered := false
	v.OnRenderMermaid = func(*MermaidOtherBlock) { rendered = true }
	r, frame := viewFrames(t, v)
	frame()

	r.Queue(key.Event{Name: "R", State: key.Press})
	frame()
	if rendered {
		t.Error("R typed into the editor rendered a diagram")
	}
	r.Queue(key.Event{Name: "=", Modifiers: key.ModCtrl, State: key.Press})
	frame()
	if v.TextScale <= 1 {
		t.Error("Ctrl+= stopped zooming the text")
	}

	v.NoPlainKeys = false
	frame()
	r.Queue(key.Event{Name: "R", State: key.Press})
	frame()
	if !rendered {
		t.Error("R didn't render once the editor let go")
	}
}

// The reading keys scroll the view: arrows by a line, Page Down and Space
// by a screen, Home and End to either end, which the list doesn't pass.
func TestReadingKeysScroll(t *testing.T) {
	var src strings.Builder
	for i := range 60 {
		fmt.Fprintf(&src, "Paragraph %d.\n\n", i)
	}
	v := NewView(Parse([]byte(src.String()), nil))
	r, frame := viewFrames(t, v)
	frame()
	press := func(name key.Name, mods key.Modifiers) layout.Position {
		t.Helper()
		r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
		frame()
		frame()
		return v.List.Position
	}
	top := func(p layout.Position) int { return p.First*1000 + p.Offset }

	down := press(key.NameDownArrow, 0)
	if top(down) <= 0 {
		t.Fatalf("Down didn't scroll: %+v", down)
	}
	page := press(key.NamePageDown, 0)
	if page.First < 10 {
		t.Errorf("Page Down moved only to block %d of an 800 px screen", page.First)
	}
	space := press(key.NameSpace, 0)
	if top(space) <= top(page) {
		t.Errorf("Space didn't scroll on: %+v after %+v", space, page)
	}
	if back := press(key.NameSpace, key.ModShift); top(back) >= top(space) {
		t.Errorf("Shift+Space didn't scroll back: %+v", back)
	}
	if up := press(key.NameUpArrow, 0); top(up) >= top(space) {
		t.Errorf("Up didn't scroll back: %+v", up)
	}

	end := press(key.NameEnd, 0)
	if end.First+end.Count != len(v.Doc.Blocks) || end.OffsetLast != 0 {
		t.Errorf("End: %+v, want the last block's bottom at the bottom", end)
	}
	if again := press(key.NamePageDown, 0); again.First != end.First || again.Offset != end.Offset {
		t.Errorf("Page Down at the end moved on: %+v", again)
	}
	if home := press(key.NameHome, 0); home.First != 0 || home.Offset != 0 {
		t.Errorf("Home: %+v", home)
	}
}

// ShowBlock scrolls as little as it takes: a block far off goes to the
// top, one cut off at the bottom just comes up into view, and one taller
// than the view keeps its top in view.
func TestShowBlock(t *testing.T) {
	var src strings.Builder
	for i := range 60 {
		fmt.Fprintf(&src, "Paragraph %d.\n\n", i)
	}
	src.WriteString("```\n" + strings.Repeat("code\n", 100) + "```\n")
	v := NewView(Parse([]byte(src.String()), nil))
	_, frame := viewFrames(t, v)
	frame()

	v.ShowBlock(30)
	frame()
	if p := v.List.Position; p.First != 30 || p.Offset != 0 {
		t.Fatalf("far block: first %d offset %d", p.First, p.Offset)
	}

	below := v.List.Position.First + v.List.Position.Count
	v.ShowBlock(below)
	frame()
	p := v.List.Position
	if p.First+p.Count-1 != below || p.OffsetLast < 0 {
		t.Errorf("block below: first %d count %d offset last %d", p.First, p.Count, p.OffsetLast)
	}
	if p.First > 33 {
		t.Errorf("block below scrolled %d blocks", p.First-30)
	}

	v.ShowBlock(29)
	frame()
	if p := v.List.Position; p.First != 29 || p.Offset != 0 {
		t.Errorf("block above: first %d offset %d", p.First, p.Offset)
	}

	v.List.Position = layout.Position{First: 45}
	frame()
	v.ShowBlock(60)
	frame()
	if p := v.List.Position; p.First != 60 || p.Offset != 0 {
		t.Errorf("tall block: first %d offset %d", p.First, p.Offset)
	}
}

// Clicking a link calls OnOpenLink with its URL; clicking plain text
// beside it doesn't.
func TestLinkClick(t *testing.T) {
	v := NewView(Parse([]byte("[here](https://x.org) and plain text after it\n"), nil))
	var opened []string
	v.OnOpenLink = func(url string) { opened = append(opened, url) }
	r, frame := viewFrames(t, v)
	frame()
	// The column starts at x 70 in the 1000 px view.
	click := func(x float32) {
		at := f32.Pt(x, 10)
		r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at})
		r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at})
		frame()
	}
	click(80)
	click(300)
	if len(opened) != 1 || opened[0] != "https://x.org" {
		t.Errorf("opened %q", opened)
	}
}

// A diagram that doesn't parse names the line of the file it goes wrong
// on and quotes it; Enter on it, or its button, leads there.
func TestDiagramErrorLeadsToSource(t *testing.T) {
	src := "# T\n\n```mermaid\nsequenceDiagram\n  Alice->>Bob: hi\n  Alice->>+: oops\n```\n"
	v := NewView(Parse([]byte(src), nil))
	s := v.Doc.Sequences[0]
	if s.Error == nil {
		t.Fatal("diagram parsed")
	}
	if got := s.ErrorLine(); got != 6 {
		t.Errorf("error on line %d, want 6", got)
	}
	if msg, line := s.errorText(); line != "Alice->>+: oops" || strings.Contains(msg, "line") {
		t.Errorf("message %q, line %q", msg, line)
	}

	var edits []int
	v.OnEditLine = func(line int) { edits = append(edits, line) }
	r, frame := viewFrames(t, v)
	frame()
	r.Queue(key.Event{Name: key.NameTab, State: key.Press})
	frame()
	r.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	frame()
	if len(edits) != 1 || edits[0] != 6 {
		t.Fatalf("Enter: edits %v", edits)
	}

	// The button is the last thing in the box, at its bottom left.
	for y := float32(60); y < 300; y += 4 {
		at := f32.Pt(70+12+20, y)
		r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at})
		r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at})
		frame()
		frame()
		if len(edits) > 1 {
			break
		}
	}
	if len(edits) != 2 || edits[1] != 6 {
		t.Errorf("button: edits %v", edits)
	}
}

// A long document has a scrollbar at the right edge: a click low on its
// track scrolls down.
func TestScrollbar(t *testing.T) {
	var src strings.Builder
	for i := range 200 {
		fmt.Fprintf(&src, "Paragraph %d.\n\n", i)
	}
	v := NewView(Parse([]byte(src.String()), nil))
	r, frame := viewFrames(t, v)
	frame()
	frame()
	at := f32.Pt(995, 700)
	r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at})
	r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at})
	frame()
	frame()
	if v.List.Position.First < 50 {
		t.Errorf("click on the track: first block %d", v.List.Position.First)
	}
}

// Ctrl+wheel zooms the text, and doesn't scroll, in the middle of a
// document as at its ends.
func TestCtrlWheelZoomsText(t *testing.T) {
	var src strings.Builder
	for i := range 60 {
		fmt.Fprintf(&src, "Paragraph %d.\n\n", i)
	}
	v := NewView(Parse([]byte(src.String()), nil))
	r, frame := viewFrames(t, v)
	frame()
	v.List.Position = layout.Position{First: 20}
	frame()
	before := v.List.Position
	ctrlWheel := func(dy float32) {
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(500, 400), Scroll: f32.Pt(0, dy), Modifiers: key.ModCtrl})
		frame()
	}
	ctrlWheel(-120)
	if got := v.TextScale; math.Abs(float64(got)-1.1) > 1e-4 {
		t.Errorf("a Ctrl+wheel notch up zoomed the text to %v, want one step, 1.1", got)
	}
	if got := v.List.Position; got.First != before.First || got.Offset != before.Offset {
		t.Errorf("Ctrl+wheel scrolled from %+v to %+v", before, got)
	}
	// A touchpad's small steps add up to a step.
	for range 3 {
		ctrlWheel(-10)
	}
	if got := v.TextScale; math.Abs(float64(got)-1.1) > 1e-4 {
		t.Errorf("30 dp of Ctrl+touchpad zoomed the text to %v, before a step's worth", got)
	}
	ctrlWheel(-10)
	if got := v.TextScale; math.Abs(float64(got)-1.2) > 1e-4 {
		t.Errorf("40 dp of Ctrl+touchpad zoomed the text to %v, want 1.2", got)
	}
}

// smoothView returns a view of 60 short paragraphs, all as tall, with
// SmoothScroll on, laid out once; a router feeding it; a function that
// lays out a frame; and one that returns how far it's scrolled, in px.
func smoothView(t *testing.T) (*View, *input.Router, func(), func() int) {
	t.Helper()
	var src strings.Builder
	for i := range 60 {
		fmt.Fprintf(&src, "Paragraph %d.\n\n", i)
	}
	v := NewView(Parse([]byte(src.String()), nil))
	v.SmoothScroll = true
	r, frame := viewFrames(t, v)
	frame()
	scrolled := func() int {
		p := v.List.Position
		return p.First*(v.heights[p.First]+v.List.Gap) + p.Offset
	}
	return v, r, frame, scrolled
}

// settle lays out frames until the eased scrolling is done, and returns
// how many it took.
func settle(t *testing.T, v *View, frame func()) int {
	t.Helper()
	for n := 1; n <= 100; n++ {
		frame()
		if v.scrollLeft == 0 {
			return n
		}
	}
	t.Fatalf("the eased scrolling didn't end: %v px left", v.scrollLeft)
	return 0
}

func wheel(r *input.Router, dy float32) {
	r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(500, 400), Scroll: f32.Pt(0, dy)})
}

// With SmoothScroll, a wheel's notch is eased over a few frames and ends
// where it would have jumped to.
func TestSmoothWheelScroll(t *testing.T) {
	v, r, frame, scrolled := smoothView(t)
	wheel(r, 100)
	frame()
	if got := scrolled(); got <= 0 || got >= 100 {
		t.Errorf("first frame of a 100 px notch scrolled %d px, want some of it", got)
	}
	if n := settle(t, v, frame); n < 3 {
		t.Errorf("a 100 px notch was done in %d frames", n)
	}
	if got := scrolled(); got != 100 {
		t.Errorf("a 100 px notch scrolled %d px", got)
	}

	// A second notch while the first is under way adds to it.
	wheel(r, 100)
	frame()
	wheel(r, 100)
	settle(t, v, frame)
	if got := scrolled(); got != 300 {
		t.Errorf("three 100 px notches scrolled %d px, want 300", got)
	}
}

// A notch starts gently and picks up speed, rather than making its biggest
// move on its first frame.
func TestSmoothWheelStartsGently(t *testing.T) {
	v, r, frame, scrolled := smoothView(t)
	wheel(r, 120)
	frame()
	first := scrolled()
	frame()
	second := scrolled() - first
	if first <= 0 || second <= first {
		t.Errorf("a notch moved %d px on its first frame and %d px on its second", first, second)
	}
	settle(t, v, frame)
}

// Notches in a row run together at an even speed: once under way, the view
// doesn't stop and start with each one.
func TestSmoothWheelKeepsAnEvenSpeed(t *testing.T) {
	_, r, frame, scrolled := smoothView(t)
	var steps []int
	last := 0
	for i := range 48 {
		if i%4 == 0 {
			wheel(r, 120)
		}
		frame()
		steps = append(steps, scrolled()-last)
		last = scrolled()
	}
	under := steps[24:]
	lo, hi := slices.Min(under), slices.Max(under)
	if lo <= 0 || float64(hi)/float64(lo) > 1.6 {
		t.Errorf("steps went from %d to %d px per frame: %v", lo, hi, under)
	}
}

// A touchpad's small steps move the view at once, even with SmoothScroll.
func TestTouchpadScrollsAtOnce(t *testing.T) {
	_, r, frame, scrolled := smoothView(t)
	for i := 1; i <= 4; i++ {
		wheel(r, 6)
		frame()
		if got := scrolled(); got != 6*i {
			t.Fatalf("after %d steps of 6 px the view is %d px down", i, got)
		}
	}
}

// Turning the wheel back drops the rest of the way, and a jump stops the
// eased scrolling.
func TestSmoothScrollStops(t *testing.T) {
	v, r, frame, scrolled := smoothView(t)
	wheel(r, 400)
	frame()
	frame()
	at := scrolled()
	wheel(r, -100)
	settle(t, v, frame)
	if got, want := scrolled(), at-100; got < want-1 || got > want+1 {
		t.Errorf("turned back 100 px at %d px, ended at %d px, want %d", at, got, want)
	}

	wheel(r, 400)
	frame()
	v.List.Position = layout.Position{First: 2}
	frame()
	if v.scrollLeft != 0 || v.List.Position.First != 2 || v.List.Position.Offset != 0 {
		t.Errorf("after a jump to block 2: %+v, %v px left", v.List.Position, v.scrollLeft)
	}

	// At the top, the rest of the way up goes nowhere.
	v.List.Position = layout.Position{}
	frame()
	wheel(r, -400)
	frame()
	if v.scrollLeft != 0 {
		t.Errorf("scrolling up at the top left %v px to go", v.scrollLeft)
	}
}

// With SmoothScroll, the reading keys are eased too, and Home and End
// still jump.
func TestSmoothKeys(t *testing.T) {
	v, r, frame, scrolled := smoothView(t)
	r.Queue(key.Event{Name: key.NamePageDown, State: key.Press})
	frame()
	if got := scrolled(); got <= 0 || got >= 800-scrollLine {
		t.Errorf("first frame of Page Down scrolled %d px", got)
	}
	settle(t, v, frame)
	if got, want := scrolled(), 800-scrollLine; got != want {
		t.Errorf("Page Down scrolled %d px, want %d", got, want)
	}
	r.Queue(key.Event{Name: key.NameHome, State: key.Press})
	frame()
	if got := scrolled(); got != 0 || v.scrollLeft != 0 {
		t.Errorf("Home: %d px down, %v px left", got, v.scrollLeft)
	}
}

// Without SmoothScroll the wheel moves the view at once, as it did.
func TestWheelScrollsAtOnce(t *testing.T) {
	v, r, frame, scrolled := smoothView(t)
	v.SmoothScroll = false
	wheel(r, 100)
	frame()
	if got := scrolled(); got != 100 {
		t.Errorf("a 100 px notch scrolled %d px", got)
	}
}

// wrappedView returns a view of 40 paragraphs, each several lines long,
// laid out once, a router feeding it and a function laying out a frame.
func wrappedView(t *testing.T) (*View, *input.Router, func()) {
	t.Helper()
	var src strings.Builder
	for i := range 40 {
		fmt.Fprintf(&src, "Paragraph %d. %s\n\n", i, strings.Repeat("Words to read and wrap. ", 30))
	}
	v := NewView(Parse([]byte(src.String()), nil))
	r, frame := viewFrames(t, v)
	frame()
	return v, r, frame
}

// Zooming the text keeps the reader's place: the block at the top stays
// as far scrolled past, as a share of its height, not in px.
func TestTextZoomKeepsThePlace(t *testing.T) {
	v, r, frame := wrappedView(t)
	v.List.Position = layout.Position{First: 10}
	frame()
	h := v.heights[10]
	v.List.Position = layout.Position{First: 10, Offset: h / 2}
	frame()
	for range 5 {
		r.Queue(key.Event{Name: "=", Modifiers: key.ModCtrl, State: key.Press})
		frame()
	}
	if v.TextScale < 1.45 {
		t.Fatalf("the text scale is %v after five steps in", v.TextScale)
	}
	i, frac := v.TopBlock()
	if i != 10 || math.Abs(float64(frac)-0.5) > 0.02 {
		t.Errorf("after zooming in, block %d is at the top, %.2f of it scrolled past; want block 10, half", i, frac)
	}
	if v.heights[10] <= h {
		t.Errorf("block 10 is %d px tall zoomed in, %d before", v.heights[10], h)
	}
}

// ScrollTo puts a block at the top with a share of it scrolled past.
func TestScrollTo(t *testing.T) {
	v, _, frame := wrappedView(t)
	v.ScrollTo(12, 0.25)
	frame()
	i, frac := v.TopBlock()
	if i != 12 || math.Abs(float64(frac)-0.25) > 0.01 {
		t.Errorf("ScrollTo(12, 0.25) put block %d at the top, %.3f scrolled past", i, frac)
	}
}
