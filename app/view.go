package app

import (
	"image"
	"image/color"
	"math"

	"dyamark/seq"

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

// SpikeView renders the Phase 0 hardcoded sequence diagram. seq/draw
// replaces it in phase 3.
type SpikeView struct{}

// Layout draws the sequence diagram spike. All coordinates are in dp: one
// transform scales the whole drawing to pixels, so shapes and labels stay
// in proportion on any output scale.
func (v *SpikeView) Layout(gtx layout.Context, mTheme *material.Theme, oTheme *Theme) layout.Dimensions {
	size := gtx.Constraints.Max
	scale := gtx.Metric.PxPerDp
	defer op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
	// Inside the transform one unit is one dp; text keeps the desktop's
	// sp-to-dp ratio.
	gtx.Metric = unit.Metric{PxPerDp: 1, PxPerSp: gtx.Metric.PxPerSp / scale}
	gtx.Constraints.Max = image.Pt(int(float32(size.X)/scale), int(float32(size.Y)/scale))

	d := diagramPainter{gtx: gtx, th: mTheme, seq: oTheme.Seq}

	// Actors: Alice (Client), API Gateway (Auth/API), Database (Storage)
	aliceX := float32(140)
	gatewayX := float32(400)
	dbX := float32(660)

	actorWidth := float32(130)
	actorHeight := float32(46)
	topY := float32(40)
	bottomY := float32(480)

	d.frame(aliceX-40, 165, dbX+75, 435, "opt [Valid JWT Credentials]")

	for _, x := range []float32{aliceX, gatewayX, dbX} {
		d.dashedLine(f32.Pt(x, topY+actorHeight), f32.Pt(x, bottomY), 1.5, d.seq.Lifeline)
	}

	d.activation(gatewayX, 120, 410, 14)
	d.activation(dbX, 185, 265, 14)

	d.message(aliceX, gatewayX-7, 120, "POST /api/session", false, true)
	d.message(gatewayX+7, dbX-7, 185, "SELECT user, hash FROM users", false, true)
	d.message(dbX-7, gatewayX+7, 265, "1 row returned", true, false)

	noteWidth := float32(180)
	d.note(gatewayX-noteWidth/2, 310, noteWidth, 38, "Verify Argon2 hash\n& issue token")

	d.message(gatewayX-7, aliceX, 410, "200 OK (JWT Bearer)", true, false)

	for _, y := range []float32{topY, bottomY} {
		d.actor(aliceX, y, actorWidth, actorHeight, "Alice")
		d.actor(gatewayX, y, actorWidth, actorHeight, "API Gateway")
		d.actor(dbX, y, actorWidth, actorHeight, "Database")
	}

	d.label(f32.Pt(40, 10), 0, "Authentication Sequence Flow", 15, oTheme.Foreground, font.Bold, text.Start)

	return layout.Dimensions{Size: size}
}

type diagramPainter struct {
	gtx layout.Context
	th  *material.Theme
	seq seq.Theme
}

func (d diagramPainter) actor(centerX, topY, w, h float32, name string) {
	r := rect(centerX-w/2, topY, centerX+w/2, topY+h)
	d.box(r, d.seq.Rounding, d.seq.ActorBkg, d.seq.ActorBorder, 1.5)
	d.label(f32.Pt(r.Min.X, topY+h/2-9), w, name, 13, d.seq.ActorText, font.Normal, text.Middle)
}

func (d diagramPainter) activation(centerX, topY, bottomY, w float32) {
	d.box(rect(centerX-w/2, topY, centerX+w/2, bottomY), 0, d.seq.ActivationBkg, d.seq.ActivationBorder, 1.2)
}

func (d diagramPainter) note(left, top, w, h float32, content string) {
	d.box(rect(left, top, left+w, top+h), 4, d.seq.NoteBkg, d.seq.NoteBorder, 1.2)
	d.label(f32.Pt(left+6, top+4), w-12, content, 11, d.seq.NoteText, font.Normal, text.Middle)
}

func (d diagramPainter) frame(left, top, right, bottom float32, label string) {
	d.box(rect(left, top, right, bottom), d.seq.Rounding, d.seq.FrameBkg, d.seq.FrameBorder, 1)
	paint.FillShape(d.gtx.Ops, d.seq.FrameBorder, clip.Outline{Path: roundRect(d.gtx.Ops, rect(left, top, left+180, top+22), 3)}.Op())
	d.label(f32.Pt(left+8, top+3), 0, label, 10, d.seq.FrameText, font.Bold, text.Start)
}

func (d diagramPainter) message(fromX, toX, y float32, label string, dashed, filledArrow bool) {
	from, to := f32.Pt(fromX, y), f32.Pt(toX, y)
	if dashed {
		d.dashedLine(from, to, 1.5, d.seq.Signal)
	} else {
		d.line(from, to, 1.5, d.seq.Signal)
	}
	d.arrowHead(to, toX > fromX, filledArrow)

	left := min(fromX, toX)
	d.label(f32.Pt(left, y-18), max(fromX, toX)-left, label, 11, d.seq.SignalText, font.Normal, text.Middle)
}

// box fills r and strokes its outline.
func (d diagramPainter) box(r frect, radius float32, fill, border color.NRGBA, width float32) {
	ops := d.gtx.Ops
	paint.FillShape(ops, fill, clip.Outline{Path: roundRect(ops, r, radius)}.Op())
	paint.FillShape(ops, border, clip.Stroke{Path: roundRect(ops, r, radius), Width: width}.Op())
}

// label lays out txt at pos. A width of zero leaves the label unbounded.
func (d diagramPainter) label(pos f32.Point, width float32, txt string, size unit.Sp, col color.NRGBA, weight font.Weight, align text.Alignment) {
	defer op.Affine(f32.Affine2D{}.Offset(pos)).Push(d.gtx.Ops).Pop()
	gtx := d.gtx
	if width > 0 {
		gtx.Constraints = layout.Exact(image.Pt(int(width), gtx.Constraints.Max.Y))
	} else {
		gtx.Constraints.Min = image.Point{}
	}
	lbl := material.Label(d.th, size, txt)
	lbl.Color = col
	lbl.Alignment = align
	lbl.Font.Weight = weight
	lbl.Layout(gtx)
}

func (d diagramPainter) line(from, to f32.Point, width float32, col color.NRGBA) {
	var p clip.Path
	p.Begin(d.gtx.Ops)
	p.MoveTo(from)
	p.LineTo(to)
	paint.FillShape(d.gtx.Ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
}

// dashedLine strokes every dash of the line as one path; Gio strokes have
// no dash style.
func (d diagramPainter) dashedLine(from, to f32.Point, width float32, col color.NRGBA) {
	const dash, gap = 6, 4
	delta := to.Sub(from)
	length := float32(math.Hypot(float64(delta.X), float64(delta.Y)))
	if length == 0 {
		return
	}
	dir := delta.Mul(1 / length)
	var p clip.Path
	p.Begin(d.gtx.Ops)
	for cur := float32(0); cur < length; cur += dash + gap {
		end := min(cur+dash, length)
		p.MoveTo(from.Add(dir.Mul(cur)))
		p.LineTo(from.Add(dir.Mul(end)))
	}
	paint.FillShape(d.gtx.Ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
}

func (d diagramPainter) arrowHead(tip f32.Point, pointingRight, filled bool) {
	back := float32(-1)
	if !pointingRight {
		back = 1
	}
	var p clip.Path
	p.Begin(d.gtx.Ops)
	if filled {
		p.MoveTo(tip)
		p.LineTo(f32.Pt(tip.X+back*9, tip.Y-4))
		p.LineTo(f32.Pt(tip.X+back*9, tip.Y+4))
		p.Close()
		paint.FillShape(d.gtx.Ops, d.seq.Signal, clip.Outline{Path: p.End()}.Op())
		return
	}
	p.MoveTo(f32.Pt(tip.X+back*8, tip.Y-4))
	p.LineTo(tip)
	p.LineTo(f32.Pt(tip.X+back*8, tip.Y+4))
	paint.FillShape(d.gtx.Ops, d.seq.Signal, clip.Stroke{Path: p.End(), Width: 1.5}.Op())
}

// frect is a rectangle in float coordinates.
type frect struct{ Min, Max f32.Point }

func rect(x0, y0, x1, y1 float32) frect {
	return frect{Min: f32.Pt(x0, y0), Max: f32.Pt(x1, y1)}
}

// roundRect is clip.RRect with float coordinates, so shapes keep their
// sub-pixel positions.
func roundRect(ops *op.Ops, r frect, radius float32) clip.PathSpec {
	radius = min(radius, (r.Max.X-r.Min.X)/2, (r.Max.Y-r.Min.Y)/2)
	// Control-point distance for a quarter circle made of one cubic.
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
