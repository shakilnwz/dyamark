package markdown

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"

	"dyamark/fonts"
	"dyamark/seq"
	"dyamark/seq/draw"
	layoutPkg "dyamark/seq/layout"
	"dyamark/seq/parse"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Theme holds the colours and styles for Markdown block rendering.
type Theme struct {
	Background        color.NRGBA
	Foreground        color.NRGBA
	Accent            color.NRGBA
	LighterBackground color.NRGBA
	// Error is the theme's red, for diagrams that don't parse.
	Error    color.NRGBA
	Rounding int
	Seq      seq.Theme
}

// Block is a rendered Markdown block widget.
type Block interface {
	Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions
}

// HeadingBlock represents a markdown heading (# H1 to ###### H6).
type HeadingBlock struct {
	Level int
	Text  string
	Spans []Span
}

func (h *HeadingBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	sizeSp := unit.Sp(16)
	switch h.Level {
	case 1:
		sizeSp = unit.Sp(28)
	case 2:
		sizeSp = unit.Sp(22)
	case 3:
		sizeSp = unit.Sp(18)
	case 4:
		sizeSp = unit.Sp(16)
	case 5:
		sizeSp = unit.Sp(14)
	case 6:
		sizeSp = unit.Sp(13)
	}

	topPad := 12
	if h.Level <= 2 {
		topPad = 20
	}
	defer op.Affine(f32.Affine2D{}.Offset(f32.Pt(0, float32(topPad)))).Push(gtx.Ops).Pop()
	dims := layoutSpans(gtx, th, oTheme, orPlain(h.Spans, h.Text), sizeSp, oTheme.Foreground, font.Font{Weight: font.Bold})
	return layout.Dimensions{Size: image.Pt(dims.Size.X, dims.Size.Y+topPad+6)}
}

// ParagraphBlock represents a text paragraph.
type ParagraphBlock struct {
	Text  string
	Spans []Span
}

func (p *ParagraphBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	dims := layoutSpans(gtx, th, oTheme, orPlain(p.Spans, p.Text), 15, oTheme.Foreground, font.Font{})
	return layout.Dimensions{Size: image.Pt(dims.Size.X, dims.Size.Y+8)}
}

// CodeBlock represents a fenced or indented code block.
type CodeBlock struct {
	Language string
	Code     string
}

func (c *CodeBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	width := gtx.Constraints.Max.X
	pad := float32(12.0)

	lines := strings.Split(strings.TrimRight(c.Code, "\n"), "\n")
	for i, l := range lines {
		lines[i] = expandTabs(l)
	}
	fontSize := unit.Sp(13)
	lineH := gtx.Metric.Sp(unit.Sp(19))
	if lineH < 18 {
		lineH = 18
	}
	headerH := 0
	if c.Language != "" {
		headerH = gtx.Metric.Sp(unit.Sp(18))
	}

	// A line too long for the block wraps, so lines are measured before
	// the background is drawn. Wrapped rows are lineH apart, like lines,
	// so a line takes its height rounded up to whole rows.
	pxPerSp := gtx.Metric.PxPerSp
	if pxPerSp <= 0 {
		pxPerSp = 1
	}
	lineGtx := gtx
	lineGtx.Constraints = layout.Constraints{Max: image.Pt(max(width-int(pad*2), 0), gtx.Constraints.Max.Y)}
	lineCalls := make([]op.CallOp, len(lines))
	lineHs := make([]int, len(lines))
	contentH := 0
	for i, line := range lines {
		lbl := material.Label(th, fontSize, line)
		lbl.Color = oTheme.Foreground
		lbl.Font.Typeface = fonts.Mono
		lbl.LineHeight = unit.Sp(float32(lineH) / pxPerSp)
		lbl.LineHeightScale = 1
		m := op.Record(gtx.Ops)
		dims := lbl.Layout(lineGtx)
		lineCalls[i] = m.Stop()
		lineHs[i] = max((dims.Size.Y+lineH-1)/lineH, 1) * lineH
		contentH += lineHs[i]
	}
	totalH := contentH + headerH + int(pad*2)

	r := clip.Rect(image.Rectangle{Max: image.Pt(width, totalH)})
	paint.FillShape(gtx.Ops, oTheme.LighterBackground, r.Op())
	paint.FillShape(gtx.Ops, withAlpha(oTheme.Foreground, 30), clip.Stroke{Path: r.Path(), Width: 1}.Op())

	y := pad
	if c.Language != "" {
		langLbl := material.Label(th, unit.Sp(11), strings.ToUpper(c.Language))
		langLbl.Color = oTheme.Accent
		langLbl.Font.Weight = font.Bold
		trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(pad, y))).Push(gtx.Ops)
		langLbl.Layout(lineGtx)
		trans.Pop()
		y += float32(headerH)
	}

	for i, call := range lineCalls {
		trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(pad, y))).Push(gtx.Ops)
		call.Add(gtx.Ops)
		trans.Pop()
		y += float32(lineHs[i])
	}

	return layout.Dimensions{Size: image.Pt(width, totalH+10)}
}

// SequenceBlock represents an inline sequence diagram.
type SequenceBlock struct {
	Index        int
	Source       string
	StartLine    int
	AST          *parse.Diagram
	Diagram      *layoutPkg.Diagram
	Error        error
	LastGood     *layoutPkg.Diagram
	Focused      bool
	Click        widget.Clickable
	OnOpenViewer func(block *SequenceBlock)
	// OnEditLine shows the source at a line of the file, 1-based: where a
	// diagram that doesn't parse goes wrong.
	OnEditLine func(line int)
	editClick  widget.Clickable
}

// ErrorLine returns the line of the file where the diagram's error is, or
// its first line when the error doesn't say.
func (s *SequenceBlock) ErrorLine() int {
	var pe *parse.Error
	if errors.As(s.Error, &pe) && pe.Line > 0 {
		return s.StartLine + pe.Line - 1
	}
	return s.StartLine
}

// errorText returns what's wrong, without the line it's on, and that
// line of the diagram's source, trimmed.
func (s *SequenceBlock) errorText() (msg, line string) {
	msg = s.Error.Error()
	var pe *parse.Error
	if errors.As(s.Error, &pe) {
		msg = pe.Msg
		if lines := strings.Split(s.Source, "\n"); pe.Line > 0 && pe.Line <= len(lines) {
			line = strings.TrimSpace(expandTabs(lines[pe.Line-1]))
		}
	}
	return msg, line
}

func (s *SequenceBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	width := gtx.Constraints.Max.X

	// Enter is handled by the View, which knows which diagram is focused.
	if s.Click.Clicked(gtx) {
		if s.OnOpenViewer != nil {
			s.OnOpenViewer(s)
		}
	}

	if s.Error != nil {
		return s.layoutError(gtx, th, oTheme, width)
	}

	if s.Diagram == nil {
		return layout.Dimensions{Size: image.Pt(width, 40)}
	}

	diagW := float32(s.Diagram.ViewBox.W)
	diagH := float32(s.Diagram.ViewBox.H)
	if diagW <= 0 || diagH <= 0 {
		return layout.Dimensions{Size: image.Pt(width, 40)}
	}

	// The card's margin stays inside the column, so its outline shows on
	// all sides.
	const margin = 8
	scale := min(float32(width-2*margin)/diagW, 1)
	renderW := diagW * scale
	renderH := diagH * scale
	offsetX := (float32(width) - renderW) / 2
	offsetY := float32(12.0)
	totalH := int(renderH + offsetY*2 + 16)

	return s.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		hovered := s.Click.Hovered()
		if hovered {
			pointer.CursorPointer.Add(gtx.Ops)
		}

		card := image.Rectangle{
			Min: image.Pt(int(offsetX)-margin, int(offsetY)-margin),
			Max: image.Pt(int(offsetX+renderW)+margin, int(offsetY+renderH)+margin),
		}
		bgCol := withAlpha(oTheme.LighterBackground, 80)
		if hovered || s.Focused {
			bgCol = withAlpha(oTheme.LighterBackground, 140)
		}
		paint.FillShape(gtx.Ops, bgCol, clip.Rect(card).Op())

		// Keyboard focus is a 2 px outline inside the card; hover a 1 px one.
		switch {
		case s.Focused:
			w := max(gtx.Dp(2), 2)
			paint.FillShape(gtx.Ops, oTheme.Accent, clip.Stroke{Path: clip.Rect(card.Inset(w / 2)).Path(), Width: float32(w)}.Op())
		case hovered:
			paint.FillShape(gtx.Ops, withAlpha(oTheme.Accent, 160), clip.Stroke{Path: clip.Rect(card.Inset(1)).Path(), Width: 1}.Op())
		default:
			paint.FillShape(gtx.Ops, withAlpha(oTheme.Foreground, 30), clip.Stroke{Path: clip.Rect(card.Inset(1)).Path(), Width: 1}.Op())
		}

		trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(offsetX, offsetY)).Scale(f32.Pt(0, 0), f32.Pt(scale, scale))).Push(gtx.Ops)

		themeName := ""
		var themeVars map[string]string
		if s.AST != nil {
			themeName = s.AST.Config.Theme
			themeVars = s.AST.Config.ThemeVariables
		}
		diagTheme := seq.ResolveTheme(oTheme.Seq, themeName, themeVars)

		diagGtx := gtx
		diagGtx.Metric = unit.Metric{PxPerDp: 1.0, PxPerSp: 1.0}
		diagGtx.Constraints = layout.Exact(image.Pt(int(diagW), int(diagH)))
		draw.Draw(diagGtx, th, diagTheme, s.Diagram)

		trans.Pop()

		if s.Focused {
			drawCardHint(gtx, th, oTheme, card, "Enter to open")
		}
		return layout.Dimensions{Size: image.Pt(width, totalH)}
	})
}

// drawCardHint shows msg in a small label in the bottom right corner of
// card.
func drawCardHint(gtx layout.Context, th *material.Theme, oTheme *Theme, card image.Rectangle, msg string) {
	pad := gtx.Dp(6)
	lbl := material.Caption(th, msg)
	lbl.Color = seq.TextOn(oTheme.Accent)
	lbl.MaxLines = 1
	cgtx := gtx
	cgtx.Constraints = layout.Constraints{Max: card.Size()}
	rec := op.Record(gtx.Ops)
	dims := lbl.Layout(cgtx)
	text := rec.Stop()
	box := image.Rectangle{Min: card.Max.Sub(dims.Size).Sub(image.Pt(3*pad, 3*pad))}
	box.Max = box.Min.Add(dims.Size).Add(image.Pt(2*pad, pad))
	paint.FillShape(gtx.Ops, oTheme.Accent, clip.UniformRRect(box, gtx.Dp(4)).Op(gtx.Ops))
	off := op.Offset(box.Min.Add(image.Pt(pad, pad/2))).Push(gtx.Ops)
	text.Add(gtx.Ops)
	off.Pop()
}

func (s *SequenceBlock) layoutError(gtx layout.Context, th *material.Theme, oTheme *Theme, width int) layout.Dimensions {
	offsetY := float32(12.0)

	// If there's a last-known good diagram, draw it dimmed underneath
	var lastGoodH float32
	if s.LastGood != nil && s.LastGood.ViewBox.W > 0 && s.LastGood.ViewBox.H > 0 {
		diagW := float32(s.LastGood.ViewBox.W)
		diagH := float32(s.LastGood.ViewBox.H)
		scale := float32(width) / diagW
		if scale > 1.0 {
			scale = 1.0
		}
		renderW := diagW * scale
		lastGoodH = diagH * scale
		offsetX := (float32(width) - renderW) / 2

		trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(offsetX, offsetY)).Scale(f32.Pt(0, 0), f32.Pt(scale, scale))).Push(gtx.Ops)
		dimmedTheme := oTheme.Seq
		dimmedTheme.ActorBkg = withAlpha(dimmedTheme.ActorBkg, 50)
		dimmedTheme.ActorBorder = withAlpha(dimmedTheme.ActorBorder, 50)
		dimmedTheme.ActorText = withAlpha(dimmedTheme.ActorText, 50)
		dimmedTheme.Lifeline = withAlpha(dimmedTheme.Lifeline, 50)
		dimmedTheme.Signal = withAlpha(dimmedTheme.Signal, 50)
		dimmedTheme.SignalText = withAlpha(dimmedTheme.SignalText, 50)
		dimmedTheme.NoteBkg = withAlpha(dimmedTheme.NoteBkg, 50)
		dimmedTheme.NoteBorder = withAlpha(dimmedTheme.NoteBorder, 50)
		dimmedTheme.NoteText = withAlpha(dimmedTheme.NoteText, 50)

		diagGtx := gtx
		diagGtx.Metric = unit.Metric{PxPerDp: 1.0, PxPerSp: 1.0}
		diagGtx.Constraints = layout.Exact(image.Pt(int(diagW), int(diagH)))
		draw.Draw(diagGtx, th, dimmedTheme, s.LastGood)
		trans.Pop()
	}

	if s.editClick.Clicked(gtx) && s.OnEditLine != nil {
		s.OnEditLine(s.ErrorLine())
	}
	errBoxH := s.layoutErrorBox(gtx, th, oTheme, width)
	totalH := errBoxH + 16
	if int(lastGoodH)+24 > totalH {
		totalH = int(lastGoodH) + 24
	}
	return layout.Dimensions{Size: image.Pt(width, totalH)}
}

// layoutErrorBox draws what's wrong with the diagram, over its last good
// drawing, in the theme's red, with a button to edit the line; it returns
// its height.
func (s *SequenceBlock) layoutErrorBox(gtx layout.Context, th *material.Theme, oTheme *Theme, width int) int {
	red := oTheme.Error
	if red.A == 0 {
		red = color.NRGBA{R: 0xf8, G: 0x51, B: 0x49, A: 0xff}
	}
	pad := gtx.Dp(12)
	line := s.ErrorLine()
	msg, src := s.errorText()
	inner := gtx
	inner.Constraints = layout.Constraints{Max: image.Pt(max(width-2*pad, 0), gtx.Constraints.Max.Y)}

	// The contents are recorded first: the box is as tall as they are.
	rec := op.Record(gtx.Ops)
	y := 0
	place := func(call op.CallOp, h, gap int) {
		off := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		off.Pop()
		y += h + gap
	}
	label := func(l material.LabelStyle) (op.CallOp, int) {
		r := op.Record(gtx.Ops)
		h := l.Layout(inner).Size.Y
		return r.Stop(), h
	}
	title := material.Label(th, unit.Sp(15), fmt.Sprintf("Syntax error on line %d", line))
	title.Color = oTheme.Foreground
	title.Font.Weight = font.Bold
	call, h := label(title)
	place(call, h, gtx.Dp(4))
	body := material.Label(th, unit.Sp(14), msg)
	body.Color = oTheme.Foreground
	call, h = label(body)
	place(call, h, gtx.Dp(6))
	if src != "" {
		code := material.Label(th, unit.Sp(13), src)
		code.Color = oTheme.Foreground
		code.Font.Typeface = fonts.Mono
		code.MaxLines = 1
		call, h := label(code)
		place(call, h, gtx.Dp(8))
	}
	if s.OnEditLine != nil {
		btn := material.Button(th, &s.editClick, fmt.Sprintf("Edit line %d", line))
		btn.Background = red
		btn.Color = seq.TextOn(red)
		btn.TextSize = unit.Sp(13)
		btn.Inset = layout.UniformInset(unit.Dp(6))
		bgtx := inner
		bgtx.Constraints.Min = image.Point{}
		r := op.Record(gtx.Ops)
		h := btn.Layout(bgtx).Size.Y
		place(r.Stop(), h, 0)
	}
	contents := rec.Stop()

	box := image.Rectangle{Max: image.Pt(width, y+2*pad)}
	paint.FillShape(gtx.Ops, oTheme.Background, clip.Rect(box).Op())
	paint.FillShape(gtx.Ops, withAlpha(red, 40), clip.Rect(box).Op())
	paint.FillShape(gtx.Ops, red, clip.Stroke{Path: clip.Rect(box.Inset(1)).Path(), Width: float32(max(gtx.Dp(1.5), 1))}.Op())
	if s.Focused {
		inset := max(gtx.Dp(2), 2)
		paint.FillShape(gtx.Ops, oTheme.Accent, clip.Stroke{Path: clip.Rect(box.Inset(inset / 2)).Path(), Width: float32(inset)}.Op())
		drawCardHint(gtx, th, oTheme, box, "Enter to edit")
	}
	off := op.Offset(image.Pt(pad, pad)).Push(gtx.Ops)
	contents.Add(gtx.Ops)
	off.Pop()
	return box.Dy()
}

// MermaidOtherBlock represents non-sequence Mermaid diagrams (e.g. flowchart).
type MermaidOtherBlock struct {
	DiagramType string
	Source      string
	StartLine   int
	Cache       *DiagramCache
	Rendered    image.Image
	RenderError string
	OnRender    func(b *MermaidOtherBlock)

	// hovered is set while the pointer is over the block, so R renders it.
	hovered bool
	// imgOp is Rendered made into an image op, which copies it and uploads
	// it to the GPU, so it's made once per image; imgSrc is that image.
	imgOp  paint.ImageOp
	imgSrc image.Image
}

func (m *MermaidOtherBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	// R is handled by the View, which picks the block to render.
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: m,
			Kinds:  pointer.Press | pointer.Enter | pointer.Leave | pointer.Cancel,
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			if m.OnRender != nil {
				m.OnRender(m)
			}
		case pointer.Enter:
			m.hovered = true
		case pointer.Leave, pointer.Cancel:
			m.hovered = false
		}
	}

	dims := m.layoutContent(gtx, th, oTheme)

	// The list gives its children unbounded height, so the hit area is the
	// block's own size, not the constraints.
	area := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, m)
	area.Pop()
	return dims
}

func (m *MermaidOtherBlock) layoutContent(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	width := gtx.Constraints.Max.X

	if m.Rendered != nil {
		imgSz := m.Rendered.Bounds().Size()
		scale := float32(width) / float32(imgSz.X)
		if scale > 1.0 {
			scale = 1.0
		}
		renderW := int(float32(imgSz.X) * scale)
		renderH := int(float32(imgSz.Y) * scale)
		offsetX := (width - renderW) / 2

		trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(float32(offsetX), 10)).Scale(f32.Pt(0, 0), f32.Pt(scale, scale))).Push(gtx.Ops)
		if m.imgSrc != m.Rendered {
			m.imgOp = paint.NewImageOp(m.Rendered)
			m.imgSrc = m.Rendered
		}
		m.imgOp.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		trans.Pop()

		return layout.Dimensions{Size: image.Pt(width, renderH+24)}
	}

	// The source, and how to render it where mmdc can be run.
	cb := CodeBlock{Language: m.DiagramType, Code: m.Source}
	if m.OnRender != nil {
		cb.Language = fmt.Sprintf("%s — Press 'R' to render with mmdc", m.DiagramType)
	}
	dims := cb.Layout(gtx, th, oTheme)

	if m.RenderError != "" {
		errLbl := material.Label(th, unit.Sp(12), "Render error: "+m.RenderError)
		errLbl.Color = color.NRGBA{R: 240, G: 80, B: 80, A: 255}
		trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(12, float32(dims.Size.Y)))).Push(gtx.Ops)
		errLbl.Layout(gtx)
		trans.Pop()
		return layout.Dimensions{Size: image.Pt(width, dims.Size.Y+24)}
	}

	return dims
}

// expandTabs replaces each tab in line with spaces to the next stop,
// every 4 columns.
func expandTabs(line string) string {
	if !strings.Contains(line, "\t") {
		return line
	}
	var buf strings.Builder
	col := 0
	for _, r := range line {
		if r == '\t' {
			n := 4 - col%4
			buf.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		buf.WriteRune(r)
		col++
	}
	return buf.String()
}

// ListBlock represents ordered or unordered lists.
type ListBlock struct {
	Ordered bool
	// Start is the number of the first item of an ordered list.
	Start int
	Items []ListItem
}

// ListItem is one list entry: its text, with paragraphs separated by a
// blank line, and the lists nested under it. A task item has a box,
// ticked when it's done.
type ListItem struct {
	Text       string
	Spans      []Span
	Nested     []*ListBlock
	Task, Done bool
}

// marker returns the bullet or number in front of item i.
func (l *ListBlock) marker(i int) string {
	if l.Ordered {
		return fmt.Sprintf("%d. ", l.Start+i)
	}
	return "• "
}

func (l *ListBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	width := gtx.Constraints.Max.X
	totalH := l.layoutItems(gtx, th, oTheme)
	return layout.Dimensions{Size: image.Pt(width, totalH+6)}
}

// layoutItems draws the items, with nested lists indented under their
// item, and returns the height they take.
func (l *ListBlock) layoutItems(gtx layout.Context, th *material.Theme, oTheme *Theme) int {
	m, textX := l.layoutMarkers(gtx, th, oTheme)
	totalH := 0
	// Item text starts right of the marker, so it has that much less room.
	textGtx := gtx
	textGtx.Constraints = layout.Constraints{Max: image.Pt(max(gtx.Constraints.Max.X-textX, 0), gtx.Constraints.Max.Y)}

	for i, item := range l.Items {
		col := oTheme.Foreground
		if item.Task {
			drawTaskBox(gtx, oTheme, markerX, totalH, item.Done)
			if item.Done {
				col = withAlpha(col, 150)
			}
		} else {
			// Numbers line up on their dots.
			x := markerX
			if l.Ordered {
				x += m.widest - m.widths[i]
			}
			transB := op.Offset(image.Pt(x, totalH)).Push(gtx.Ops)
			m.calls[i].Add(gtx.Ops)
			transB.Pop()
		}

		transT := op.Affine(f32.Affine2D{}.Offset(f32.Pt(float32(textX), float32(totalH)))).Push(gtx.Ops)
		dims := layoutSpans(textGtx, th, oTheme, orPlain(item.Spans, item.Text), 14, col, font.Font{})
		transT.Pop()

		itemH := dims.Size.Y
		if itemH < 20 {
			itemH = 20
		}
		for _, nested := range item.Nested {
			itemH += 6
			trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(float32(textX), float32(totalH+itemH)))).Push(gtx.Ops)
			itemH += nested.layoutItems(textGtx, th, oTheme) - 6
			trans.Pop()
		}
		totalH += itemH + 6
	}
	return totalH
}

// markerX is where list markers start.
const markerX = 12

// listMarkers holds a list's recorded markers and their widths; a task
// item has none.
type listMarkers struct {
	calls  []op.CallOp
	widths []int
	widest int
}

// layoutMarkers records the items' markers, and returns them with where
// the items' text starts: right of the widest.
func (l *ListBlock) layoutMarkers(gtx layout.Context, th *material.Theme, oTheme *Theme) (listMarkers, int) {
	m := listMarkers{calls: make([]op.CallOp, len(l.Items)), widths: make([]int, len(l.Items))}
	gtx.Constraints.Min = image.Point{}
	for i, item := range l.Items {
		if item.Task {
			// The box and a gap like a bullet's.
			m.widest = max(m.widest, gtx.Sp(13)+gtx.Sp(7))
			continue
		}
		lbl := material.Label(th, unit.Sp(14), l.marker(i))
		lbl.Color = oTheme.Accent
		lbl.Font.Weight = font.Bold
		rec := op.Record(gtx.Ops)
		m.widths[i] = lbl.Layout(gtx).Size.X
		m.calls[i] = rec.Stop()
		m.widest = max(m.widest, m.widths[i])
	}
	return m, max(32, markerX+m.widest)
}

// drawTaskBox draws a task's box at x, centred on the first line of text
// starting at y: outlined, or filled and ticked when done.
func drawTaskBox(gtx layout.Context, oTheme *Theme, x, y int, done bool) {
	size := gtx.Sp(13)
	lineH := gtx.Sp(14) * 6 / 5
	box := image.Rectangle{Min: image.Pt(x, y+(lineH-size)/2)}
	box.Max = box.Min.Add(image.Pt(size, size))
	r := max(size/5, 1)
	if !done {
		inset := box.Inset(1)
		paint.FillShape(gtx.Ops, withAlpha(oTheme.Foreground, 170),
			clip.Stroke{Path: clip.UniformRRect(inset, r).Path(gtx.Ops), Width: float32(max(gtx.Dp(1.5), 1))}.Op())
		return
	}
	paint.FillShape(gtx.Ops, oTheme.Accent, clip.UniformRRect(box, r).Op(gtx.Ops))
	s := float32(size)
	o := f32.Pt(float32(box.Min.X), float32(box.Min.Y))
	var tick clip.Path
	tick.Begin(gtx.Ops)
	tick.MoveTo(o.Add(f32.Pt(s*0.24, s*0.52)))
	tick.LineTo(o.Add(f32.Pt(s*0.43, s*0.71)))
	tick.LineTo(o.Add(f32.Pt(s*0.77, s*0.31)))
	paint.FillShape(gtx.Ops, seq.TextOn(oTheme.Accent), clip.Stroke{Path: tick.End(), Width: s * 0.14}.Op())
}

// QuoteBlock represents a blockquote.
type QuoteBlock struct {
	Text  string
	Spans []Span
}

func (q *QuoteBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	width := gtx.Constraints.Max.X
	pad := float32(14.0)

	textX := int(pad + 8)
	textGtx := gtx
	textGtx.Constraints = layout.Constraints{Max: image.Pt(max(width-textX, 0), gtx.Constraints.Max.Y)}
	trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(float32(textX), 4))).Push(gtx.Ops)
	dims := layoutSpans(textGtx, th, oTheme, orPlain(q.Spans, q.Text), 14, withAlpha(oTheme.Foreground, 210), font.Font{Style: font.Italic})
	trans.Pop()

	totalH := dims.Size.Y + 12

	// Left bar
	var bar clip.Path
	bar.Begin(gtx.Ops)
	bar.MoveTo(f32.Pt(4, 2))
	bar.LineTo(f32.Pt(4, float32(totalH-2)))
	paint.FillShape(gtx.Ops, oTheme.Accent, clip.Stroke{Path: bar.End(), Width: 3.5}.Op())

	return layout.Dimensions{Size: image.Pt(width, totalH+6)}
}

// DividerBlock represents a horizontal rule divider.
type DividerBlock struct{}

func (d *DividerBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	width := gtx.Constraints.Max.X
	var line clip.Path
	line.Begin(gtx.Ops)
	line.MoveTo(f32.Pt(0, 10))
	line.LineTo(f32.Pt(float32(width), 10))
	paint.FillShape(gtx.Ops, withAlpha(oTheme.Foreground, 40), clip.Stroke{Path: line.End(), Width: 1}.Op())
	return layout.Dimensions{Size: image.Pt(width, 24)}
}

// TableBlock represents a Markdown table with headers and data rows.
type TableBlock struct {
	Headers []string
	Rows    [][]string
	// HeaderSpans and RowSpans are the cells' styled text.
	HeaderSpans [][]Span
	RowSpans    [][][]Span
}

// tableRow is a laid-out table row: its recorded cells and its height.
type tableRow struct {
	cells []op.CallOp
	h     float32
}

func (t *TableBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	if len(t.Headers) == 0 && len(t.Rows) == 0 {
		return layout.Dimensions{}
	}

	numCols := len(t.Headers)
	if numCols == 0 && len(t.Rows) > 0 {
		numCols = len(t.Rows[0])
	}
	if numCols == 0 {
		return layout.Dimensions{}
	}

	totalW := gtx.Constraints.Max.X
	colW := float32(totalW) / float32(numCols)
	cellPadX := float32(10.0)
	cellPadY := float32(8.0)
	minRowH := float32(gtx.Metric.Sp(unit.Sp(13))) * 2.2
	if minRowH < 32 {
		minRowH = 32
	}

	// Cell text wraps at its column, so the rows are measured before
	// anything is drawn; a row is as tall as its tallest cell.
	cellGtx := gtx
	cellGtx.Constraints = layout.Constraints{Max: image.Pt(max(int(colW-2*cellPadX), 0), gtx.Constraints.Max.Y)}
	measure := func(texts []string, cells [][]Span, bold bool) tableRow {
		r := tableRow{h: minRowH}
		var base font.Font
		if bold {
			base.Weight = font.Bold
		}
		for c, txt := range texts {
			if c >= numCols {
				break
			}
			var spans []Span
			if c < len(cells) {
				spans = cells[c]
			}
			m := op.Record(gtx.Ops)
			dims := layoutSpans(cellGtx, th, oTheme, orPlain(spans, txt), 13, oTheme.Foreground, base)
			r.cells = append(r.cells, m.Stop())
			r.h = max(r.h, float32(dims.Size.Y)+2*cellPadY)
		}
		return r
	}
	drawCells := func(r tableRow, y float32) {
		for c, call := range r.cells {
			trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(float32(c)*colW+cellPadX, y+cellPadY))).Push(gtx.Ops)
			call.Add(gtx.Ops)
			trans.Pop()
		}
	}

	var header tableRow
	tableH := float32(0)
	if len(t.Headers) > 0 {
		header = measure(t.Headers, t.HeaderSpans, true)
		tableH += header.h
	}
	rows := make([]tableRow, len(t.Rows))
	for i, row := range t.Rows {
		var cells [][]Span
		if i < len(t.RowSpans) {
			cells = t.RowSpans[i]
		}
		rows[i] = measure(row, cells, false)
		tableH += rows[i].h
	}

	tableRect := clip.Rect(image.Rectangle{Max: image.Pt(totalW, int(tableH))})
	paint.FillShape(gtx.Ops, withAlpha(oTheme.LighterBackground, 60), tableRect.Op())
	paint.FillShape(gtx.Ops, withAlpha(oTheme.Foreground, 40), clip.Stroke{Path: tableRect.Path(), Width: 1}.Op())

	curY := float32(0.0)

	if len(t.Headers) > 0 {
		hdrRect := clip.Rect(image.Rectangle{Max: image.Pt(totalW, int(header.h))})
		paint.FillShape(gtx.Ops, withAlpha(oTheme.LighterBackground, 200), hdrRect.Op())
		drawCells(header, curY)

		curY += header.h
		var div clip.Path
		div.Begin(gtx.Ops)
		div.MoveTo(f32.Pt(0, curY))
		div.LineTo(f32.Pt(float32(totalW), curY))
		paint.FillShape(gtx.Ops, withAlpha(oTheme.Foreground, 60), clip.Stroke{Path: div.End(), Width: 1.5}.Op())
	}

	for rIdx, row := range rows {
		if rIdx%2 == 1 {
			zRect := clip.Rect(image.Rectangle{
				Min: image.Pt(0, int(curY)),
				Max: image.Pt(totalW, int(curY+row.h)),
			})
			paint.FillShape(gtx.Ops, withAlpha(oTheme.LighterBackground, 100), zRect.Op())
		}
		drawCells(row, curY)

		curY += row.h
		if rIdx < len(t.Rows)-1 {
			var div clip.Path
			div.Begin(gtx.Ops)
			div.MoveTo(f32.Pt(0, curY))
			div.LineTo(f32.Pt(float32(totalW), curY))
			paint.FillShape(gtx.Ops, withAlpha(oTheme.Foreground, 25), clip.Stroke{Path: div.End(), Width: 1}.Op())
		}
	}

	return layout.Dimensions{Size: image.Pt(totalW, int(tableH+14))}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}
