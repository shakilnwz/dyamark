package markdown

import (
	"image"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
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
	TextScale    float32 // 1.0 default
	// NoPlainKeys turns off the shortcuts typed without Ctrl (Tab, Enter
	// and R) while another widget, the editor, takes the typing: a typed
	// letter arrives as both text and a key press.
	NoPlainKeys bool

	OnOpenViewer    func(block *SequenceBlock)
	OnRenderMermaid func(block *MermaidOtherBlock)
}

// NewView creates a new markdown document view.
func NewView(doc *ParsedDocument) *View {
	v := &View{
		Doc:          doc,
		FocusedIndex: -1,
		TextScale:    1.0,
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

// ZoomTextIn increases the text scale by 10%.
func (v *View) ZoomTextIn() {
	if v.TextScale <= 0 {
		v.TextScale = 1.0
	}
	v.TextScale += 0.1
	if v.TextScale > 2.5 {
		v.TextScale = 2.5
	}
}

// ZoomTextOut decreases the text scale by 10%.
func (v *View) ZoomTextOut() {
	if v.TextScale <= 0 {
		v.TextScale = 1.0
	}
	v.TextScale -= 0.1
	if v.TextScale < 0.6 {
		v.TextScale = 0.6
	}
}

// ResetTextZoom resets the text scale to 100%.
func (v *View) ResetTextZoom() {
	v.TextScale = 1.0
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

// openDiagram opens the focused diagram in the viewer, or the first one on
// screen that parsed when none is focused.
func (v *View) openDiagram() {
	if v.OnOpenViewer == nil {
		return
	}
	if v.FocusedIndex >= 0 && v.FocusedIndex < len(v.Doc.Sequences) {
		v.OnOpenViewer(v.Doc.Sequences[v.FocusedIndex])
		return
	}
	for _, b := range v.visibleBlocks() {
		if s, ok := b.(*SequenceBlock); ok && s.AST != nil {
			v.OnOpenViewer(s)
			return
		}
	}
}

// renderMermaid renders the Mermaid block under the pointer with mmdc, or
// else the first one on screen that isn't rendered yet.
func (v *View) renderMermaid() {
	if v.OnRenderMermaid == nil {
		return
	}
	var target *MermaidOtherBlock
	for _, b := range v.visibleBlocks() {
		m, ok := b.(*MermaidOtherBlock)
		if !ok {
			continue
		}
		if m.hovered {
			target = m
			break
		}
		if target == nil && m.Rendered == nil {
			target = m
		}
	}
	if target != nil {
		v.OnRenderMermaid(target)
	}
}

// visibleBlocks returns the blocks on screen in the last frame.
func (v *View) visibleBlocks() []Block {
	first := v.List.Position.First
	last := min(first+v.List.Position.Count, len(v.Doc.Blocks))
	if first < 0 || first >= last {
		return nil
	}
	return v.Doc.Blocks[first:last]
}

// Layout renders the document into a readable column with scrolling.
func (v *View) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X <= 0 || size.Y <= 0 || v.Doc == nil {
		return layout.Dimensions{}
	}

	filters := []event.Filter{
		pointer.Filter{
			Target:  v,
			Kinds:   pointer.Scroll,
			ScrollX: pointer.ScrollRange{Min: -1000000, Max: 1000000},
			ScrollY: pointer.ScrollRange{Min: -1000000, Max: 1000000},
		},
		key.Filter{Required: key.ModCtrl, Name: "="},
		// `+` and `_` are usually typed with Shift, which Gio reports.
		key.Filter{Required: key.ModCtrl, Optional: key.ModShift, Name: "+"},
		key.Filter{Required: key.ModCtrl, Name: "-"},
		key.Filter{Required: key.ModCtrl, Optional: key.ModShift, Name: "_"},
		key.Filter{Required: key.ModCtrl, Name: "0"},
	}
	if !v.NoPlainKeys {
		filters = append(filters,
			key.Filter{Name: key.NameTab},
			key.Filter{Name: key.NameTab, Required: key.ModShift},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
			key.Filter{Name: "R"},
		)
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case pointer.Event:
			if e.Kind == pointer.Scroll && e.Modifiers.Contain(key.ModCtrl) {
				if e.Scroll.Y < 0 {
					v.ZoomTextIn()
				} else if e.Scroll.Y > 0 {
					v.ZoomTextOut()
				}
			}
		case key.Event:
			if e.State != key.Press {
				break
			}
			if e.Modifiers.Contain(key.ModCtrl) {
				switch e.Name {
				case "=", "+":
					v.ZoomTextIn()
				case "-", "_":
					v.ZoomTextOut()
				case "0":
					v.ResetTextZoom()
				}
			} else {
				if e.Name == key.NameTab {
					if e.Modifiers.Contain(key.ModShift) {
						v.FocusPrev()
					} else {
						v.FocusNext()
					}
				} else if e.Name == key.NameReturn || e.Name == key.NameEnter {
					v.openDiagram()
				} else if e.Name == "R" {
					v.renderMermaid()
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

	if v.TextScale <= 0 {
		v.TextScale = 1.0
	}
	colGtx := gtx
	colGtx.Metric.PxPerSp = gtx.Metric.PxPerSp * v.TextScale
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
