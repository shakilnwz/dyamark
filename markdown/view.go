package markdown

import (
	"image"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget/material"
)

// View renders a scrollable markdown document in a readable column.
type View struct {
	Doc          *ParsedDocument
	List         layout.List
	FocusedIndex int

	OnOpenViewer    func(block *SequenceBlock)
	OnRenderMermaid func(block *MermaidOtherBlock)
}

// NewView creates a new markdown document view.
func NewView(doc *ParsedDocument) *View {
	v := &View{
		Doc:          doc,
		FocusedIndex: -1,
	}
	v.List.Axis = layout.Vertical
	v.List.Gap = 12
	return v
}

// SetDocument updates the document while preserving scroll position.
func (v *View) SetDocument(doc *ParsedDocument) {
	v.Doc = doc
	if v.FocusedIndex >= len(doc.Sequences) {
		v.FocusedIndex = len(doc.Sequences) - 1
	}
}

// FocusNext advances focus to the next sequence diagram.
func (v *View) FocusNext() {
	if v.Doc == nil || len(v.Doc.Sequences) == 0 {
		return
	}
	v.FocusedIndex = (v.FocusedIndex + 1) % len(v.Doc.Sequences)
}

// FocusPrev moves focus to the previous sequence diagram.
func (v *View) FocusPrev() {
	if v.Doc == nil || len(v.Doc.Sequences) == 0 {
		return
	}
	v.FocusedIndex = (v.FocusedIndex - 1 + len(v.Doc.Sequences)) % len(v.Doc.Sequences)
}

// Layout renders the document into a readable column with scrolling.
func (v *View) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X <= 0 || size.Y <= 0 || v.Doc == nil {
		return layout.Dimensions{}
	}

	for {
		ev, ok := gtx.Event(
			key.Filter{Name: key.NameTab},
			key.Filter{Name: key.NameTab, Required: key.ModShift},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
		)
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			if ke.Name == key.NameTab {
				if ke.Modifiers.Contain(key.ModShift) {
					v.FocusPrev()
				} else {
					v.FocusNext()
				}
			} else if (ke.Name == key.NameReturn || ke.Name == key.NameEnter) && v.FocusedIndex >= 0 && v.FocusedIndex < len(v.Doc.Sequences) {
				if v.OnOpenViewer != nil {
					v.OnOpenViewer(v.Doc.Sequences[v.FocusedIndex])
				}
			}
		}
	}

	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	event.Op(gtx.Ops, v)
	area.Pop()

	// Compute readable column width (capped at 860px)
	const maxColWidth = 860
	const sidePadding = 32
	availW := size.X - 2*sidePadding
	colW := availW
	if colW > maxColWidth {
		colW = maxColWidth
	}
	if colW < 200 {
		colW = 200
	}
	offsetX := (size.X - colW) / 2

	// Render scrollable column
	trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(float32(offsetX), 0))).Push(gtx.Ops)
	defer trans.Pop()

	colGtx := gtx
	colGtx.Constraints = layout.Exact(image.Pt(colW, size.Y))

	// Wire callbacks into blocks
	for i, seq := range v.Doc.Sequences {
		seq.Focused = (i == v.FocusedIndex)
		if seq.OnOpenViewer == nil {
			seq.OnOpenViewer = v.OnOpenViewer
		}
	}

	for _, b := range v.Doc.Blocks {
		if mob, ok := b.(*MermaidOtherBlock); ok {
			if mob.OnRender == nil {
				mob.OnRender = v.OnRenderMermaid
			}
		}
	}

	return v.List.Layout(colGtx, len(v.Doc.Blocks), func(gtx layout.Context, i int) layout.Dimensions {
		return v.Doc.Blocks[i].Layout(gtx, th, oTheme)
	})
}
