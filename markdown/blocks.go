package markdown

import (
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
	Rounding          int
	Seq               seq.Theme
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

	scale := float32(width) / diagW
	if scale > 1.0 {
		scale = 1.0
	}
	renderW := diagW * scale
	renderH := diagH * scale
	offsetX := (float32(width) - renderW) / 2
	offsetY := float32(12.0)
	totalH := int(renderH + offsetY*2 + 16)

	return s.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if s.Click.Hovered() {
			pointer.CursorPointer.Add(gtx.Ops)
		}

		// Card background & subtle border
		cardRect := clip.Rect(image.Rectangle{
			Min: image.Pt(int(offsetX)-8, int(offsetY)-8),
			Max: image.Pt(int(offsetX+renderW)+8, int(offsetY+renderH)+8),
		})
		bgCol := withAlpha(oTheme.LighterBackground, 80)
		if s.Click.Hovered() {
			bgCol = withAlpha(oTheme.LighterBackground, 140)
		}
		paint.FillShape(gtx.Ops, bgCol, cardRect.Op())

		borderColor := withAlpha(oTheme.Foreground, 30)
		if s.Focused || s.Click.Hovered() {
			borderColor = oTheme.Accent
		}
		paint.FillShape(gtx.Ops, borderColor, clip.Stroke{Path: cardRect.Path(), Width: 1}.Op())

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

		return layout.Dimensions{Size: image.Pt(width, totalH)}
	})
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

	// Mermaid-style error banner overlay
	errBoxH := 80
	boxRect := clip.Rect(image.Rectangle{Max: image.Pt(width, errBoxH)})
	errBg := color.NRGBA{R: 80, G: 20, B: 25, A: 220}
	errBorder := color.NRGBA{R: 240, G: 70, B: 70, A: 255}
	paint.FillShape(gtx.Ops, errBg, boxRect.Op())
	paint.FillShape(gtx.Ops, errBorder, clip.Stroke{Path: boxRect.Path(), Width: 1.5}.Op())

	t1 := material.Label(th, unit.Sp(13), fmt.Sprintf("Syntax error in diagram (markdown line %d)", s.StartLine))
	t1.Color = color.NRGBA{R: 255, G: 200, B: 200, A: 255}
	t1.Font.Weight = font.Bold
	trans1 := op.Affine(f32.Affine2D{}.Offset(f32.Pt(12, 10))).Push(gtx.Ops)
	t1.Layout(gtx)
	trans1.Pop()

	t2 := material.Label(th, unit.Sp(12), s.Error.Error())
	t2.Color = color.NRGBA{R: 240, G: 240, B: 240, A: 255}
	trans2 := op.Affine(f32.Affine2D{}.Offset(f32.Pt(12, 34))).Push(gtx.Ops)
	t2.Layout(gtx)
	trans2.Pop()

	totalH := errBoxH + 16
	if int(lastGoodH)+24 > totalH {
		totalH = int(lastGoodH) + 24
	}
	return layout.Dimensions{Size: image.Pt(width, totalH)}
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

	// Code block with hint to render with mmdc
	cb := CodeBlock{
		Language: fmt.Sprintf("%s — Press 'R' to render with mmdc", m.DiagramType),
		Code:     m.Source,
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

// ListBlock represents ordered or unordered lists.
type ListBlock struct {
	Ordered bool
	// Start is the number of the first item of an ordered list.
	Start int
	Items []ListItem
}

// ListItem is one list entry: its text, with paragraphs separated by a
// blank line, and the lists nested under it.
type ListItem struct {
	Text   string
	Spans  []Span
	Nested []*ListBlock
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
	const textX = 32
	totalH := 0
	// Item text starts right of the bullet, so it has that much less room.
	textGtx := gtx
	textGtx.Constraints = layout.Constraints{Max: image.Pt(max(gtx.Constraints.Max.X-textX, 0), gtx.Constraints.Max.Y)}

	for i, item := range l.Items {
		bLbl := material.Label(th, unit.Sp(14), l.marker(i))
		bLbl.Color = oTheme.Accent
		bLbl.Font.Weight = font.Bold
		transB := op.Affine(f32.Affine2D{}.Offset(f32.Pt(12, float32(totalH)))).Push(gtx.Ops)
		bLbl.Layout(gtx)
		transB.Pop()

		transT := op.Affine(f32.Affine2D{}.Offset(f32.Pt(textX, float32(totalH)))).Push(gtx.Ops)
		dims := layoutSpans(textGtx, th, oTheme, orPlain(item.Spans, item.Text), 14, oTheme.Foreground, font.Font{})
		transT.Pop()

		itemH := dims.Size.Y
		if itemH < 20 {
			itemH = 20
		}
		for _, nested := range item.Nested {
			itemH += 6
			trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(textX, float32(totalH+itemH)))).Push(gtx.Ops)
			itemH += nested.layoutItems(textGtx, th, oTheme) - 6
			trans.Pop()
		}
		totalH += itemH + 6
	}
	return totalH
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
