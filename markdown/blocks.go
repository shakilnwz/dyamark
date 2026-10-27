package markdown

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"dyamark/seq"
	"dyamark/seq/draw"
	layoutPkg "dyamark/seq/layout"
	"dyamark/seq/parse"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
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

	lbl := material.Label(th, sizeSp, h.Text)
	lbl.Color = oTheme.Foreground
	lbl.Font.Weight = font.Bold
	topPad := 12
	if h.Level <= 2 {
		topPad = 20
	}
	defer op.Affine(f32.Affine2D{}.Offset(f32.Pt(0, float32(topPad)))).Push(gtx.Ops).Pop()
	dims := lbl.Layout(gtx)
	return layout.Dimensions{Size: image.Pt(dims.Size.X, dims.Size.Y+topPad+6)}
}

// ParagraphBlock represents a text paragraph.
type ParagraphBlock struct {
	Text string
}

func (p *ParagraphBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	lbl := material.Label(th, unit.Sp(15), p.Text)
	lbl.Color = oTheme.Foreground
	dims := lbl.Layout(gtx)
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
	lineH := 20
	contentH := len(lines) * lineH
	headerH := 0
	if c.Language != "" {
		headerH = 20
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
		langLbl.Layout(gtx)
		trans.Pop()
		y += float32(headerH)
	}

	for _, line := range lines {
		lbl := material.Label(th, unit.Sp(13), line)
		lbl.Color = oTheme.Foreground
		lbl.Font.Typeface = font.Typeface("monospace")
		trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(pad, y))).Push(gtx.Ops)
		lbl.Layout(gtx)
		trans.Pop()
		y += float32(lineH)
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
	OnOpenViewer func(block *SequenceBlock)
}

func (s *SequenceBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	width := gtx.Constraints.Max.X

	for {
		ev, ok := gtx.Event(
			pointer.Filter{
				Target: s,
				Kinds:  pointer.Press,
			},
			key.Filter{Focus: s},
			key.Filter{Name: key.NameEnter},
			key.Filter{Name: key.NameReturn},
		)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case pointer.Event:
			if e.Kind == pointer.Press && e.Buttons == pointer.ButtonPrimary {
				if s.OnOpenViewer != nil {
					s.OnOpenViewer(s)
				}
			}
		case key.Event:
			if e.State == key.Press {
				if s.OnOpenViewer != nil {
					s.OnOpenViewer(s)
				}
			}
		}
	}

	area := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops)
	event.Op(gtx.Ops, s)
	area.Pop()

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

	// Card background & subtle border
	cardRect := clip.Rect(image.Rectangle{
		Min: image.Pt(int(offsetX)-8, int(offsetY)-8),
		Max: image.Pt(int(offsetX+renderW)+8, int(offsetY+renderH)+8),
	})
	paint.FillShape(gtx.Ops, withAlpha(oTheme.LighterBackground, 80), cardRect.Op())
	borderColor := withAlpha(oTheme.Foreground, 30)
	if s.Focused {
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

	return layout.Dimensions{Size: image.Pt(width, int(renderH+offsetY*2+16))}
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
}

func (m *MermaidOtherBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	width := gtx.Constraints.Max.X

	for {
		ev, ok := gtx.Event(
			pointer.Filter{
				Target: m,
				Kinds:  pointer.Press,
			},
			key.Filter{Focus: m},
			key.Filter{Name: "R"},
			key.Filter{Name: "r"},
		)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case pointer.Event:
			if e.Kind == pointer.Press && m.OnRender != nil {
				m.OnRender(m)
			}
		case key.Event:
			if e.State == key.Press && m.OnRender != nil {
				m.OnRender(m)
			}
		}
	}

	area := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops)
	event.Op(gtx.Ops, m)
	area.Pop()

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
		paint.NewImageOp(m.Rendered).Add(gtx.Ops)
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
	Items   []string
}

func (l *ListBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	width := gtx.Constraints.Max.X
	totalH := 0

	for i, item := range l.Items {
		bullet := "• "
		if l.Ordered {
			bullet = fmt.Sprintf("%d. ", i+1)
		}

		bLbl := material.Label(th, unit.Sp(14), bullet)
		bLbl.Color = oTheme.Accent
		bLbl.Font.Weight = font.Bold
		transB := op.Affine(f32.Affine2D{}.Offset(f32.Pt(12, float32(totalH)))).Push(gtx.Ops)
		bLbl.Layout(gtx)
		transB.Pop()

		tLbl := material.Label(th, unit.Sp(14), item)
		tLbl.Color = oTheme.Foreground
		transT := op.Affine(f32.Affine2D{}.Offset(f32.Pt(32, float32(totalH)))).Push(gtx.Ops)
		dims := tLbl.Layout(gtx)
		transT.Pop()

		itemH := dims.Size.Y
		if itemH < 20 {
			itemH = 20
		}
		totalH += itemH + 6
	}

	return layout.Dimensions{Size: image.Pt(width, totalH+6)}
}

// QuoteBlock represents a blockquote.
type QuoteBlock struct {
	Text string
}

func (q *QuoteBlock) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	width := gtx.Constraints.Max.X
	pad := float32(14.0)

	lbl := material.Label(th, unit.Sp(14), q.Text)
	lbl.Color = withAlpha(oTheme.Foreground, 210)
	lbl.Font.Style = font.Italic

	trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(pad+8, 4))).Push(gtx.Ops)
	dims := lbl.Layout(gtx)
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
	rowH := float32(34.0)

	numRows := len(t.Rows)
	if len(t.Headers) > 0 {
		numRows++
	}
	tableH := float32(numRows) * rowH

	tableRect := clip.Rect(image.Rectangle{Max: image.Pt(totalW, int(tableH))})
	paint.FillShape(gtx.Ops, withAlpha(oTheme.LighterBackground, 60), tableRect.Op())
	paint.FillShape(gtx.Ops, withAlpha(oTheme.Foreground, 40), clip.Stroke{Path: tableRect.Path(), Width: 1}.Op())

	curY := float32(0.0)

	if len(t.Headers) > 0 {
		hdrRect := clip.Rect(image.Rectangle{Max: image.Pt(totalW, int(rowH))})
		paint.FillShape(gtx.Ops, withAlpha(oTheme.LighterBackground, 200), hdrRect.Op())

		for c, hText := range t.Headers {
			lbl := material.Label(th, unit.Sp(13), hText)
			lbl.Color = oTheme.Foreground
			lbl.Font.Weight = font.Bold
			trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(float32(c)*colW+cellPadX, curY+cellPadY))).Push(gtx.Ops)
			lbl.Layout(gtx)
			trans.Pop()
		}

		curY += rowH
		var div clip.Path
		div.Begin(gtx.Ops)
		div.MoveTo(f32.Pt(0, curY))
		div.LineTo(f32.Pt(float32(totalW), curY))
		paint.FillShape(gtx.Ops, withAlpha(oTheme.Foreground, 60), clip.Stroke{Path: div.End(), Width: 1.5}.Op())
	}

	for rIdx, row := range t.Rows {
		if rIdx%2 == 1 {
			zRect := clip.Rect(image.Rectangle{
				Min: image.Pt(0, int(curY)),
				Max: image.Pt(totalW, int(curY+rowH)),
			})
			paint.FillShape(gtx.Ops, withAlpha(oTheme.LighterBackground, 100), zRect.Op())
		}

		for c, cellText := range row {
			if c >= numCols {
				break
			}
			lbl := material.Label(th, unit.Sp(13), cellText)
			lbl.Color = oTheme.Foreground
			trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(float32(c)*colW+cellPadX, curY+cellPadY))).Push(gtx.Ops)
			lbl.Layout(gtx)
			trans.Pop()
		}

		curY += rowH
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
