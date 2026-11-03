package markdown

import (
	"image"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
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
	// SmoothScroll eases the wheel's notches and the reading keys over a
	// few frames instead of jumping; a touchpad's small steps still move
	// the view at once.
	SmoothScroll bool

	OnOpenViewer    func(block *SequenceBlock)
	OnRenderMermaid func(block *MermaidOtherBlock)
	// OnOpenLink is called with a clicked link's URL.
	OnOpenLink func(url string)
	// OnEditLine shows the source at a line of the file, 1-based.
	OnEditLine func(line int)
	// Mark is the block marked beside it, a find's match, or -1.
	Mark int

	scrollbar widget.Scrollbar

	// show is 1 + the block to bring into view at the next layout, or 0.
	show int
	// heights holds the height of each block laid out last, for laidDoc.
	heights map[int]int
	laidDoc *ParsedDocument

	// scrollLeft is how far, in px, the eased scrolling has still to go,
	// and scrollAt the frame it last moved. scrollFrac is the part of a
	// px scrolled but not shown yet.
	scrollLeft float32
	scrollAt   time.Time
	scrollFrac float32
	// laidPos is where the list was after the last layout. When it has
	// moved since, by a jump, the eased scrolling stops.
	laidPos layout.Position
	// zoomWheel adds up a Ctrl+wheel's scrolling, in dp, towards a text
	// zoom step.
	zoomWheel float32

	// anchor, when set, is the place to put at the top of the view at
	// the next layout.
	anchor *anchorAt
	// laidScale is the text scale the blocks were last laid out at.
	laidScale float32
}

// anchorAt is a place in the document: a block, and how far down it, as
// a fraction of its height.
type anchorAt struct {
	block int
	frac  float32
}

const (
	// easeNotch is the least wheel scrolling, in dp, that's eased: half
	// a notch in the IDE's browser, which gets 120 px for one. A
	// touchpad's smaller steps move the view at once.
	easeNotch = 60
	// zoomNotch is the Ctrl+wheel scrolling, in dp, that zooms the text a
	// step: a notch is 120 px in the IDE's browser, and 50 dp or more in
	// the app. A touchpad's smaller steps add up to one.
	zoomNotch = 40
	// easeTime is the eased scrolling's time constant: it covers 90% of
	// the way in about 2.3 of them.
	easeTime = 40 * time.Millisecond
	// frameTime is the time an eased frame is taken to be when there's
	// no clock: its first frame, and in tests.
	frameTime = 16 * time.Millisecond
)

// blockSpan returns where block i's top is in the view and its height, as
// last laid out; ok is false if it wasn't.
func (v *View) blockSpan(i int) (top, h int, ok bool) {
	h, ok = v.heights[i]
	if !ok {
		return 0, 0, false
	}
	p := v.List.Position
	top = -p.Offset
	for k := p.First; k < i; k++ {
		top += v.heights[k] + v.List.Gap
	}
	for k := p.First - 1; k >= i; k-- {
		top -= v.heights[k] + v.List.Gap
	}
	return top, h, true
}

// ScrollTo puts block i at the top of the view at the next layout, with
// frac of its height, from 0 to 1, scrolled past.
func (v *View) ScrollTo(i int, frac float32) {
	v.anchor = &anchorAt{i, min(max(frac, 0), 1)}
}

// TopBlock returns the block at the top of the view, as last laid out,
// and the fraction of its height scrolled past.
func (v *View) TopBlock() (i int, frac float32) {
	p := v.List.Position
	if h := v.heights[p.First]; h > 0 {
		frac = min(max(float32(p.Offset)/float32(h), 0), 1)
	}
	return p.First, frac
}

// ShowBlock scrolls block i into view at the next layout, as little as
// it takes.
func (v *View) ShowBlock(i int) {
	v.show = i + 1
}

// reveal scrolls block i into the view, height tall, as little as it
// takes, using where the blocks were laid out last. A block taller than
// the view keeps its top in view. It reports false when that layout was
// of another document.
func (v *View) reveal(i, height int) bool {
	if v.laidDoc != v.Doc {
		return false
	}
	p := &v.List.Position
	top, h, ok := v.blockSpan(i)
	if !ok {
		// Not near the view: put its top at the top.
		p.First, p.Offset = i, 0
		return true
	}
	bottom := top + h
	switch {
	case top < 0 && bottom < height:
		p.Offset += top
	case top > 0 && bottom > height:
		p.Offset += min(bottom-height, top)
	}
	return true
}

// NewView creates a new markdown document view.
func NewView(doc *ParsedDocument) *View {
	v := &View{
		Doc:          doc,
		FocusedIndex: -1,
		TextScale:    1.0,
		Mark:         -1,
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

// openDiagram opens the focused diagram in the viewer, or its source at the
// error when it doesn't parse; with none focused, the first one on screen
// that parsed.
func (v *View) openDiagram() {
	if v.FocusedIndex >= 0 && v.FocusedIndex < len(v.Doc.Sequences) {
		s := v.Doc.Sequences[v.FocusedIndex]
		switch {
		case s.Error != nil && v.OnEditLine != nil:
			// A broken diagram is fixed in the source.
			v.OnEditLine(s.ErrorLine())
		case s.Error == nil && v.OnOpenViewer != nil:
			v.OnOpenViewer(s)
		}
		return
	}
	if v.OnOpenViewer == nil {
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

// scrollLine is how far the arrow keys scroll, in px.
const scrollLine = 48

// scrollKey scrolls for the reading keys and reports whether e was one:
// the arrows by a line, Page Up/Down and Space by a screen less a line,
// Home and End to either end.
func (v *View) scrollKey(e key.Event, height int) bool {
	page := float32(max(height-scrollLine, scrollLine))
	switch e.Name {
	case key.NameUpArrow:
		v.scroll(-scrollLine, 0)
	case key.NameDownArrow:
		v.scroll(scrollLine, 0)
	case key.NamePageUp:
		v.scroll(-page, 0)
	case key.NamePageDown:
		v.scroll(page, 0)
	case key.NameSpace:
		if e.Modifiers.Contain(key.ModShift) {
			v.scroll(-page, 0)
		} else {
			v.scroll(page, 0)
		}
	case key.NameHome:
		v.stopScroll()
		v.List.Position = layout.Position{}
	case key.NameEnd:
		v.stopScroll()
		// Past the end; the list stops at the last block's bottom.
		v.List.Position = layout.Position{First: len(v.Doc.Blocks), BeforeEnd: true}
	default:
		return false
	}
	return true
}

// scroll scrolls the view by d px: eased when SmoothScroll is on and d
// is notch px or more, a wheel's notch or a key's line rather than a
// touchpad's small step; otherwise at once.
func (v *View) scroll(d, notch float32) {
	if !v.SmoothScroll || abs(d) < notch {
		v.moveBy(d)
		return
	}
	if d*v.scrollLeft < 0 {
		// Turned back: the rest of the way the other way is dropped.
		v.scrollLeft = 0
	}
	if v.scrollLeft == 0 {
		v.scrollAt = time.Time{}
	}
	v.scrollLeft += d
}

// moveBy moves the list by d px, keeping the part of a px it can't.
func (v *View) moveBy(d float32) {
	d += v.scrollFrac
	n := int(d)
	v.scrollFrac = d - float32(n)
	v.List.Position.Offset += n
}

// stopScroll drops what's left of the eased scrolling.
func (v *View) stopScroll() {
	v.scrollLeft, v.scrollFrac = 0, 0
}

// ease moves the eased scrolling on by the time since its last frame,
// and asks for another frame while there's more of it.
func (v *View) ease(gtx layout.Context) {
	if v.scrollLeft == 0 {
		return
	}
	dt := frameTime
	if !v.scrollAt.IsZero() && !gtx.Now.IsZero() {
		dt = min(max(gtx.Now.Sub(v.scrollAt), 0), 3*frameTime)
	}
	v.scrollAt = gtx.Now
	step := v.scrollLeft * float32(1-math.Exp(-dt.Seconds()/easeTime.Seconds()))
	if abs(v.scrollLeft-step) < 1 {
		// The last of it, to the whole px.
		v.List.Position.Offset += int(math.Round(float64(v.scrollLeft + v.scrollFrac)))
		v.stopScroll()
		return
	}
	v.scrollLeft -= step
	v.moveBy(step)
	gtx.Execute(op.InvalidateCmd{})
}

// wheelZoom zooms the text a step for a Ctrl+wheel's notch, d dp of
// scrolling; a touchpad's smaller steps add up to one.
func (v *View) wheelZoom(d float32) {
	if d*v.zoomWheel < 0 {
		v.zoomWheel = 0
	}
	v.zoomWheel += d
	switch {
	case v.zoomWheel <= -zoomNotch:
		v.ZoomTextIn()
		v.zoomWheel = 0
	case v.zoomWheel >= zoomNotch:
		v.ZoomTextOut()
		v.zoomWheel = 0
	}
}

func abs(f float32) float32 {
	return float32(math.Abs(float64(f)))
}

// measure returns block i's height in the column gtx is for, without
// drawing it.
func (v *View) measure(gtx layout.Context, th *material.Theme, oTheme *Theme, i int) int {
	w := gtx.Constraints.Max.X
	gtx.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, 1e6)}
	m := op.Record(gtx.Ops)
	dims := v.Doc.Blocks[i].Layout(gtx, th, oTheme)
	m.Stop()
	return dims.Size.Y
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
	// A jump since the last layout, the caller's or the scrollbar's,
	// stops the eased scrolling.
	if p := v.List.Position; p.First != v.laidPos.First || p.Offset != v.laidPos.Offset {
		v.stopScroll()
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
			key.Filter{Name: key.NameUpArrow},
			key.Filter{Name: key.NameDownArrow},
			key.Filter{Name: key.NamePageUp},
			key.Filter{Name: key.NamePageDown},
			key.Filter{Name: key.NameSpace, Optional: key.ModShift},
			key.Filter{Name: key.NameHome},
			key.Filter{Name: key.NameEnd},
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
			if e.Kind != pointer.Scroll {
				break
			}
			if e.Modifiers.Contain(key.ModCtrl) {
				v.wheelZoom(e.Scroll.Y / gtx.Metric.PxPerDp)
			} else {
				v.scroll(e.Scroll.Y, easeNotch*gtx.Metric.PxPerDp)
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
			} else if v.scrollKey(e, size.Y) {
				gtx.Execute(op.InvalidateCmd{})
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

	if v.TextScale <= 0 {
		v.TextScale = 1.0
	}
	colGtx := gtx
	colGtx.Metric.PxPerSp = gtx.Metric.PxPerSp * v.TextScale
	colGtx.Constraints = layout.Exact(image.Pt(colW, size.Y))

	for _, l := range v.Doc.Links {
		for {
			e, ok := l.click.Update(gtx.Source)
			if !ok {
				break
			}
			if e.Kind == gesture.KindClick && v.OnOpenLink != nil {
				v.OnOpenLink(l.URL)
			}
		}
	}

	if v.show > 0 {
		if v.reveal(v.show-1, size.Y) {
			v.show = 0
			v.stopScroll()
		} else {
			// Lay out the new document first.
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	// Wire callbacks into blocks
	for i, seq := range v.Doc.Sequences {
		seq.Focused = (i == v.FocusedIndex)
		if seq.OnOpenViewer == nil {
			seq.OnOpenViewer = v.OnOpenViewer
		}
		if seq.OnEditLine == nil {
			seq.OnEditLine = v.OnEditLine
		}
	}

	for _, b := range v.Doc.Blocks {
		if mob, ok := b.(*MermaidOtherBlock); ok {
			if mob.OnRender == nil {
				mob.OnRender = v.OnRenderMermaid
			}
		}
	}

	// A new text scale keeps the block at the top where it was, as a
	// share of its height rather than in px, so the reader's place stays.
	if v.laidScale != 0 && v.TextScale != v.laidScale && v.anchor == nil {
		i, frac := v.TopBlock()
		v.anchor = &anchorAt{i, frac}
	}
	if v.heights == nil {
		v.heights = make(map[int]int)
	}
	clear(v.heights)
	v.laidDoc = v.Doc
	n := len(v.Doc.Blocks)
	if a := v.anchor; a != nil {
		v.anchor = nil
		if a.block >= 0 && a.block < n {
			h := v.measure(colGtx, th, oTheme, a.block)
			v.List.Position = layout.Position{First: a.block, Offset: int(math.Round(float64(a.frac) * float64(h)))}
			v.stopScroll()
		}
	}
	v.ease(gtx)
	dims := v.List.Layout(colGtx, n, func(gtx layout.Context, i int) layout.Dimensions {
		dims := v.Doc.Blocks[i].Layout(gtx, th, oTheme)
		v.heights[i] = dims.Size.Y
		return dims
	})
	v.laidPos = v.List.Position
	v.laidScale = v.TextScale
	// At either end, the rest of the way goes nowhere.
	if p := v.List.Position; v.scrollLeft < 0 && p.First == 0 && p.Offset == 0 || v.scrollLeft > 0 && !p.BeforeEnd {
		v.stopScroll()
	}
	// The list clips its blocks to the column, so the mark beside one is
	// drawn after it.
	if top, h, ok := v.blockSpan(v.Mark); ok {
		x := -gtx.Dp(14)
		bar := image.Rect(x, max(top, 0), x+gtx.Dp(4), min(top+h, size.Y))
		paint.FillShape(gtx.Ops, oTheme.Accent, clip.UniformRRect(bar, gtx.Dp(2)).Op(gtx.Ops))
	}
	trans.Pop()
	v.layoutScrollbar(gtx, th, oTheme, size)

	// The wheel is taken here, over the list, rather than by the list:
	// so that Ctrl+wheel zooms instead of scrolling, and the scrolling can
	// be eased. Everything else passes through to the blocks.
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	event.Op(gtx.Ops, v)
	area.Pop()
	pass.Pop()
	return dims
}

// layoutScrollbar draws the scrollbar at the right edge of the view, size
// big, and scrolls the list by what's dragged on it.
func (v *View) layoutScrollbar(gtx layout.Context, th *material.Theme, oTheme *Theme, size image.Point) {
	n := len(v.Doc.Blocks)
	if n == 0 || v.List.Position.Length <= 0 {
		return
	}
	start, end := viewportOf(v.List.Position, n, size.Y)
	sb := material.Scrollbar(th, &v.scrollbar)
	sb.Indicator.Color = withAlpha(oTheme.Foreground, 70)
	sb.Indicator.HoverColor = withAlpha(oTheme.Foreground, 140)
	bar := gtx
	bar.Constraints = layout.Exact(size)
	layout.E.Layout(bar, func(gtx layout.Context) layout.Dimensions {
		return sb.Layout(gtx, layout.Vertical, start, end)
	})
	if d := v.scrollbar.ScrollDistance(); d != 0 {
		v.List.ScrollBy(d * float32(n))
	}
}

// viewportOf returns where the view, height tall, is in the list, as
// fractions of its estimated length. It's material.List's estimate.
func viewportOf(p layout.Position, n, height int) (start, end float32) {
	clamp := func(v float32) float32 { return max(0, min(v, 1)) }
	length := float32(p.Length)
	each := length / float32(n)
	start = clamp((float32(p.First)*each + float32(p.Offset)) / length)
	end = clamp((float32(p.First+p.Count)*each + float32(p.OffsetLast)) / length)
	// Spread the difference from the visible share of the estimated
	// length over both ends, by how close each is.
	shown := end - start
	if shown < 1 {
		err := float32(height)/length - shown
		s, e := start/(1-shown), (1-end)/(1-shown)
		start, end = start-s*err, end+e*err
	}
	return start, end
}
