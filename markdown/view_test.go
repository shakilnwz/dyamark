package markdown

import (
	"fmt"
	"image"
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
