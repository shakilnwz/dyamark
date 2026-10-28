package markdown

import (
	"image"
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
