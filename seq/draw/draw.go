package draw

import (
	"image"
	"image/color"
	"math"

	"dyamark/seq"
	layoutPkg "dyamark/seq/layout"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Draw renders a laid-out sequence diagram into the Gio context.
func Draw(gtx layout.Context, th *material.Theme, seqTh seq.Theme, diag *layoutPkg.Diagram) layout.Dimensions {
	if diag == nil {
		return layout.Dimensions{}
	}

	d := painter{gtx: gtx, th: th, seq: seqTh}

	// Layout coordinates start at the viewBox origin, which is usually
	// negative (the left margin, the title above the actors). Callers treat
	// the diagram as occupying (0,0)-(W,H), so shift it into place.
	defer op.Affine(f32.Affine2D{}.Offset(f32.Pt(-diag.ViewBox.X, -diag.ViewBox.Y))).Push(gtx.Ops).Pop()

	// 1. Background Rects
	for _, bg := range diag.Rects {
		r := rect(bg.Rect.X, bg.Rect.Y, bg.Rect.X+bg.Rect.W, bg.Rect.Y+bg.Rect.H)
		// Mermaid fills a rect with no colour in the actor background; it's
		// drawn behind everything, so it hides nothing.
		fillCol := seqTh.ActorBkg
		if bg.Fill != "" {
			fillCol = seq.ParseColor(bg.Fill, withAlpha(seqTh.FrameBkg, 80))
		}
		d.box(r, 0, fillCol, color.NRGBA{}, 0)
	}

	// 2. Boxes
	for _, b := range diag.Boxes {
		r := rect(b.Rect.X, b.Rect.Y, b.Rect.X+b.Rect.W, b.Rect.Y+b.Rect.H)
		fillCol := seq.ParseColor(b.Fill, withAlpha(seqTh.ActorBkg, 40))
		d.box(r, 4, fillCol, withAlpha(seqTh.Lifeline, 100), 1)
		for _, t := range b.Label {
			d.text(t, seqTh.ActorText)
		}
	}

	// 3. Frames
	for _, f := range diag.Frames {
		r := rect(f.Rect.X, f.Rect.Y, f.Rect.X+f.Rect.W, f.Rect.Y+f.Rect.H)
		// Mermaid leaves frames unfilled; only the keyword tab is filled.
		d.box(r, seqTh.Rounding, color.NRGBA{}, seqTh.FrameBorder, 1)

		if len(f.Tab) >= 3 {
			var p clip.Path
			p.Begin(gtx.Ops)
			p.MoveTo(f32.Pt(f.Tab[0].X, f.Tab[0].Y))
			for _, pt := range f.Tab[1:] {
				p.LineTo(f32.Pt(pt.X, pt.Y))
			}
			p.Close()
			paint.FillShape(gtx.Ops, seqTh.FrameBkg, clip.Outline{Path: p.End()}.Op())
			p.Begin(gtx.Ops)
			p.MoveTo(f32.Pt(f.Tab[0].X, f.Tab[0].Y))
			for _, pt := range f.Tab[1:] {
				p.LineTo(f32.Pt(pt.X, pt.Y))
			}
			p.Close()
			paint.FillShape(gtx.Ops, seqTh.FrameBorder, clip.Stroke{Path: p.End(), Width: 1}.Op())
		}

		for _, divY := range f.Dividers {
			d.dashedLine(f32.Pt(f.Rect.X, divY), f32.Pt(f.Rect.X+f.Rect.W, divY), 1.5, seqTh.FrameBorder)
		}

		for _, t := range f.Texts {
			col := seqTh.FrameText
			if t.Kind == layoutPkg.TextFrameKeyword {
				col = seqTh.LabelText
			}
			d.text(t, col)
		}
	}

	// 4. Lifelines
	for _, l := range diag.Lifelines {
		d.dashedLine(f32.Pt(l.X, l.Y1), f32.Pt(l.X, l.Y2), 1.2, seqTh.Lifeline)
	}

	// 5. Activations
	for _, act := range diag.Activations {
		r := rect(act.Rect.X, act.Rect.Y, act.Rect.X+act.Rect.W, act.Rect.Y+act.Rect.H)
		d.box(r, 0, seqTh.ActivationBkg, seqTh.ActivationBorder, 1.2)
	}

	// 6. Messages
	for _, m := range diag.Messages {
		if m.Curved && len(m.Path) == 4 {
			d.cubicCurve(
				f32.Pt(m.Path[0].X, m.Path[0].Y),
				f32.Pt(m.Path[1].X, m.Path[1].Y),
				f32.Pt(m.Path[2].X, m.Path[2].Y),
				f32.Pt(m.Path[3].X, m.Path[3].Y),
				1.5, seqTh.Signal, m.Dashed,
			)
		} else if len(m.Path) == 4 && !m.Curved {
			for i := 0; i < 3; i++ {
				p1 := f32.Pt(m.Path[i].X, m.Path[i].Y)
				p2 := f32.Pt(m.Path[i+1].X, m.Path[i+1].Y)
				if m.Dashed {
					d.dashedLine(p1, p2, 1.5, seqTh.Signal)
				} else {
					d.line(p1, p2, 1.5, seqTh.Signal)
				}
			}
		} else if len(m.Path) >= 2 {
			p1 := f32.Pt(m.Path[0].X, m.Path[0].Y)
			p2 := f32.Pt(m.Path[len(m.Path)-1].X, m.Path[len(m.Path)-1].Y)
			if m.Dashed {
				d.dashedLine(p1, p2, 1.5, seqTh.Signal)
			} else {
				d.line(p1, p2, 1.5, seqTh.Signal)
			}
		}

		if len(m.Path) >= 2 {
			pLast := f32.Pt(m.Path[len(m.Path)-1].X, m.Path[len(m.Path)-1].Y)
			pPrev := f32.Pt(m.Path[len(m.Path)-2].X, m.Path[len(m.Path)-2].Y)
			if m.Curved && len(m.Path) == 4 {
				pPrev = f32.Pt(m.Path[2].X, m.Path[2].Y)
			}
			dirEnd := pLast.X - pPrev.X
			pointingRight := dirEnd >= 0
			if m.EndMarker != layoutPkg.MarkerNone {
				d.marker(pLast, m.EndMarker, pointingRight, seqTh.Signal)
			}

			if m.StartMarker != layoutPkg.MarkerNone {
				pFirst := f32.Pt(m.Path[0].X, m.Path[0].Y)
				pNext := f32.Pt(m.Path[1].X, m.Path[1].Y)
				dirStart := pFirst.X - pNext.X
				d.marker(pFirst, m.StartMarker, dirStart >= 0, seqTh.Signal)
			}
		}

		for _, t := range m.Text {
			d.text(t, seqTh.SignalText)
		}

		if m.Number != nil {
			num := m.Number
			path := circle(gtx.Ops, f32.Pt(num.Center.X, num.Center.Y), num.Radius)
			paint.FillShape(gtx.Ops, seqTh.Signal, clip.Outline{Path: path}.Op())
			d.text(num.Text, seqTh.ActorBkg)
		}
	}

	// 7. Notes
	for _, n := range diag.Notes {
		r := rect(n.Rect.X, n.Rect.Y, n.Rect.X+n.Rect.W, n.Rect.Y+n.Rect.H)
		d.box(r, 3, seqTh.NoteBkg, seqTh.NoteBorder, 1.2)
		for _, t := range n.Text {
			d.text(t, seqTh.NoteText)
		}
	}

	// 8. Actors
	for _, a := range diag.Actors {
		if a.Figure != nil {
			fig := a.Figure
			headPath := circle(gtx.Ops, f32.Pt(fig.Head.X, fig.Head.Y), fig.Radius)
			paint.FillShape(gtx.Ops, seqTh.ActorBkg, clip.Outline{Path: headPath}.Op())
			paint.FillShape(gtx.Ops, seqTh.ActorBorder, clip.Stroke{Path: headPath, Width: 2}.Op())

			for _, l := range fig.Lines {
				d.line(f32.Pt(l[0].X, l[0].Y), f32.Pt(l[1].X, l[1].Y), 2, seqTh.ActorBorder)
			}
		} else {
			r := rect(a.Rect.X, a.Rect.Y, a.Rect.X+a.Rect.W, a.Rect.Y+a.Rect.H)
			radius := a.Radius
			if seqTh.Rounding > 0 {
				radius = seqTh.Rounding
			}
			d.box(r, radius, seqTh.ActorBkg, seqTh.ActorBorder, 1.5)
		}

		for _, t := range a.Label {
			d.text(t, seqTh.ActorText)
		}
	}

	// 9. Title
	if diag.Title != nil {
		d.text(*diag.Title, seqTh.SignalText)
	}

	w := int(math.Ceil(float64(diag.ViewBox.W)))
	h := int(math.Ceil(float64(diag.ViewBox.H)))
	return layout.Dimensions{Size: image.Pt(w, h)}
}

type painter struct {
	gtx layout.Context
	th  *material.Theme
	seq seq.Theme
}

type frect struct{ Min, Max f32.Point }

func rect(x0, y0, x1, y1 float32) frect {
	return frect{Min: f32.Pt(x0, y0), Max: f32.Pt(x1, y1)}
}

func (d painter) box(r frect, radius float32, fill, border color.NRGBA, width float32) {
	ops := d.gtx.Ops
	if fill.A > 0 {
		paint.FillShape(ops, fill, clip.Outline{Path: roundRect(ops, r, radius)}.Op())
	}
	if border.A > 0 && width > 0 {
		paint.FillShape(ops, border, clip.Stroke{Path: roundRect(ops, r, radius), Width: width}.Op())
	}
}

func (d painter) line(from, to f32.Point, width float32, col color.NRGBA) {
	var p clip.Path
	p.Begin(d.gtx.Ops)
	p.MoveTo(from)
	p.LineTo(to)
	paint.FillShape(d.gtx.Ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
}

func (d painter) dashedLine(from, to f32.Point, width float32, col color.NRGBA) {
	const dash, gap = 6.0, 4.0
	delta := to.Sub(from)
	length := float32(math.Hypot(float64(delta.X), float64(delta.Y)))
	if length == 0 {
		return
	}
	dir := delta.Mul(1 / length)
	var p clip.Path
	p.Begin(d.gtx.Ops)
	for cur := float32(0); cur < length; cur += dash + gap {
		end := cur + dash
		if end > length {
			end = length
		}
		p.MoveTo(from.Add(dir.Mul(cur)))
		p.LineTo(from.Add(dir.Mul(end)))
	}
	paint.FillShape(d.gtx.Ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
}

func (d painter) cubicCurve(p0, p1, p2, p3 f32.Point, width float32, col color.NRGBA, dashed bool) {
	if !dashed {
		var p clip.Path
		p.Begin(d.gtx.Ops)
		p.MoveTo(p0)
		p.CubeTo(p1, p2, p3)
		paint.FillShape(d.gtx.Ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
		return
	}

	const steps = 40
	pts := make([]f32.Point, steps+1)
	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)
		u := 1 - t
		pts[i] = f32.Pt(
			u*u*u*p0.X+3*u*u*t*p1.X+3*u*t*t*p2.X+t*t*t*p3.X,
			u*u*u*p0.Y+3*u*u*t*p1.Y+3*u*t*t*p2.Y+t*t*t*p3.Y,
		)
	}
	d.dashedPolyline(pts, width, col)
}

func (d painter) dashedPolyline(pts []f32.Point, width float32, col color.NRGBA) {
	const dash, gap = 6.0, 4.0
	if len(pts) < 2 {
		return
	}
	var p clip.Path
	p.Begin(d.gtx.Ops)
	p.MoveTo(pts[0])
	curDist := float32(0)
	drawing := true
	for i := 0; i < len(pts)-1; i++ {
		p1, p2 := pts[i], pts[i+1]
		segLen := float32(math.Hypot(float64(p2.X-p1.X), float64(p2.Y-p1.Y)))
		if segLen == 0 {
			continue
		}
		dir := p2.Sub(p1).Mul(1 / segLen)
		pos := float32(0)
		for pos < segLen {
			rem := segLen - pos
			if drawing {
				step := dash - curDist
				if rem < step {
					p.LineTo(p2)
					curDist += rem
					break
				}
				p.LineTo(p1.Add(dir.Mul(pos + step)))
				curDist = 0
				drawing = false
				pos += step
			} else {
				step := gap - curDist
				if rem < step {
					curDist += rem
					break
				}
				p.MoveTo(p1.Add(dir.Mul(pos + step)))
				curDist = 0
				drawing = true
				pos += step
			}
		}
	}
	paint.FillShape(d.gtx.Ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
}

func (d painter) marker(tip f32.Point, marker layoutPkg.Marker, pointingRight bool, col color.NRGBA) {
	back := float32(-1)
	if !pointingRight {
		back = 1
	}
	ops := d.gtx.Ops

	switch marker {
	case layoutPkg.MarkerArrow:
		var p clip.Path
		p.Begin(ops)
		p.MoveTo(tip)
		p.LineTo(f32.Pt(tip.X+back*10, tip.Y-5))
		p.LineTo(f32.Pt(tip.X+back*10, tip.Y+5))
		p.Close()
		paint.FillShape(ops, col, clip.Outline{Path: p.End()}.Op())

	case layoutPkg.MarkerOpen:
		var p clip.Path
		p.Begin(ops)
		p.MoveTo(f32.Pt(tip.X+back*9, tip.Y-5))
		p.LineTo(tip)
		p.LineTo(f32.Pt(tip.X+back*9, tip.Y+5))
		paint.FillShape(ops, col, clip.Stroke{Path: p.End(), Width: 1.5}.Op())

	case layoutPkg.MarkerCross:
		var p clip.Path
		p.Begin(ops)
		p.MoveTo(f32.Pt(tip.X-4, tip.Y-4))
		p.LineTo(f32.Pt(tip.X+4, tip.Y+4))
		p.MoveTo(f32.Pt(tip.X-4, tip.Y+4))
		p.LineTo(f32.Pt(tip.X+4, tip.Y-4))
		paint.FillShape(ops, col, clip.Stroke{Path: p.End(), Width: 1.5}.Op())

	case layoutPkg.MarkerSolidTop:
		var p clip.Path
		p.Begin(ops)
		p.MoveTo(tip)
		p.LineTo(f32.Pt(tip.X+back*10, tip.Y-6))
		p.LineTo(f32.Pt(tip.X+back*10, tip.Y))
		p.Close()
		paint.FillShape(ops, col, clip.Outline{Path: p.End()}.Op())

	case layoutPkg.MarkerSolidBottom:
		var p clip.Path
		p.Begin(ops)
		p.MoveTo(tip)
		p.LineTo(f32.Pt(tip.X+back*10, tip.Y))
		p.LineTo(f32.Pt(tip.X+back*10, tip.Y+6))
		p.Close()
		paint.FillShape(ops, col, clip.Outline{Path: p.End()}.Op())

	case layoutPkg.MarkerStickTop:
		var p clip.Path
		p.Begin(ops)
		p.MoveTo(f32.Pt(tip.X+back*8, tip.Y-6))
		p.LineTo(tip)
		paint.FillShape(ops, col, clip.Stroke{Path: p.End(), Width: 1.5}.Op())

	case layoutPkg.MarkerStickBottom:
		var p clip.Path
		p.Begin(ops)
		p.MoveTo(f32.Pt(tip.X+back*8, tip.Y+6))
		p.LineTo(tip)
		paint.FillShape(ops, col, clip.Stroke{Path: p.End(), Width: 1.5}.Op())
	}
}

func (d painter) text(t layoutPkg.Text, col color.NRGBA) {
	if t.Text == "" {
		return
	}

	weight := font.Normal
	if t.Weight >= 700 {
		weight = font.Bold
	}

	dims := layoutPkg.Default().MeasureText(t.Text, layoutPkg.Font{
		Size:   float64(t.Size),
		Weight: t.Weight,
	})
	w := float32(dims.Width)
	ascent := float32(math.Round(1854.0 * float64(t.Size) / 2048.0))

	left := t.X
	switch t.Anchor {
	case layoutPkg.AnchorMiddle:
		left -= w / 2
	case layoutPkg.AnchorEnd:
		left -= w
	}

	top := t.Y + t.DY
	switch t.Baseline {
	case layoutPkg.BaselineAlphabetic:
		top -= ascent
	case layoutPkg.BaselineCentral, layoutPkg.BaselineMiddle:
		top -= t.Size * 0.5
	}

	pos := f32.Pt(left, top)
	defer op.Affine(f32.Affine2D{}.Offset(pos)).Push(d.gtx.Ops).Pop()

	gtx := d.gtx
	gtx.Constraints = layout.Exact(image.Pt(int(math.Ceil(float64(w)))+4, int(math.Ceil(dims.LineHeight))+4))

	sp := float32(t.Size)
	if d.gtx.Metric.PxPerSp > 0 {
		sp = float32(t.Size) / d.gtx.Metric.PxPerSp
	}
	lbl := material.Label(d.th, unit.Sp(sp), t.Text)
	lbl.Color = col
	lbl.Alignment = text.Start
	lbl.Font.Weight = weight
	lbl.Layout(gtx)
}

func circle(ops *op.Ops, center f32.Point, radius float32) clip.PathSpec {
	return roundRect(ops, rect(center.X-radius, center.Y-radius, center.X+radius, center.Y+radius), radius)
}

func roundRect(ops *op.Ops, r frect, radius float32) clip.PathSpec {
	if radius <= 0 {
		var p clip.Path
		p.Begin(ops)
		p.MoveTo(r.Min)
		p.LineTo(f32.Pt(r.Max.X, r.Min.Y))
		p.LineTo(r.Max)
		p.LineTo(f32.Pt(r.Min.X, r.Max.Y))
		p.Close()
		return p.End()
	}

	maxRadius := (r.Max.X - r.Min.X) / 2
	if (r.Max.Y-r.Min.Y)/2 < maxRadius {
		maxRadius = (r.Max.Y - r.Min.Y) / 2
	}
	if radius > maxRadius {
		radius = maxRadius
	}

	const k = 0.5523
	c := radius * k
	x0, y0, x1, y1 := r.Min.X, r.Min.Y, r.Max.X, r.Max.Y

	var p clip.Path
	p.Begin(ops)
	p.MoveTo(f32.Pt(x0+radius, y0))
	p.LineTo(f32.Pt(x1-radius, y0))
	p.CubeTo(f32.Pt(x1-radius+c, y0), f32.Pt(x1, y0+radius-c), f32.Pt(x1, y0+radius))
	p.LineTo(f32.Pt(x1, y1-radius))
	p.CubeTo(f32.Pt(x1, y1-radius+c), f32.Pt(x1-radius+c, y1), f32.Pt(x1-radius, y1))
	p.LineTo(f32.Pt(x0+radius, y1))
	p.CubeTo(f32.Pt(x0+radius-c, y1), f32.Pt(x0, y1-radius+c), f32.Pt(x0, y1-radius))
	p.LineTo(f32.Pt(x0, y0+radius))
	p.CubeTo(f32.Pt(x0, y0+radius-c), f32.Pt(x0+radius-c, y0), f32.Pt(x0+radius, y0))
	p.Close()
	return p.End()
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}
