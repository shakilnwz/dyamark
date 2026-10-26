package app

import (
	"image"
	"image/color"
	"math"

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

// SpikeView renders the Phase 0 hardcoded sequence diagram.
type SpikeView struct {
	Zoom float32
	PanX float32
	PanY float32
}

func NewSpikeView() *SpikeView {
	return &SpikeView{
		Zoom: 1.0,
		PanX: 0,
		PanY: 0,
	}
}

// Layout draws the sequence diagram spike.
func (v *SpikeView) Layout(gtx layout.Context, mTheme *material.Theme, oTheme *Theme) layout.Dimensions {
	ops := gtx.Ops
	seq := oTheme.Seq

	// Diagram dimensions & positions
	// Actors: Alice (Client), API Gateway (Auth/API), Database (Storage)
	aliceX := float32(140)
	gatewayX := float32(400)
	dbX := float32(660)

	actorWidth := float32(130)
	actorHeight := float32(46)
	topY := float32(40)
	bottomY := float32(480)
	radius := int(seq.Rounding)

	// Draw Group / Frame: "opt Authenticated Request"
	frameLeft := aliceX - 40
	frameTop := float32(165)
	frameRight := dbX + 75
	frameBottom := float32(435)
	drawFrame(ops, frameLeft, frameTop, frameRight, frameBottom, "opt [Valid JWT Credentials]", seq, mTheme, gtx)

	// Lifelines (vertical dashed lines)
	drawDashedVLine(ops, aliceX, topY+actorHeight, bottomY, 1.5, seq.Lifeline, 6, 4)
	drawDashedVLine(ops, gatewayX, topY+actorHeight, bottomY, 1.5, seq.Lifeline, 6, 4)
	drawDashedVLine(ops, dbX, topY+actorHeight, bottomY, 1.5, seq.Lifeline, 6, 4)

	// Activations
	// API Gateway active from 120 to 410
	drawActivation(ops, gatewayX, 120, 410, 14, seq)
	// Database active from 185 to 265
	drawActivation(ops, dbX, 185, 265, 14, seq)

	// Messages
	// 1. Alice ->> API Gateway: POST /api/session
	drawMessage(ops, aliceX, gatewayX-7, 120, "POST /api/session", false, true, seq, mTheme, gtx)

	// 2. API Gateway ->> Database: SELECT user, hash FROM users
	drawMessage(ops, gatewayX+7, dbX-7, 185, "SELECT user, hash FROM users", false, true, seq, mTheme, gtx)

	// 3. Database -->> API Gateway: 1 row returned
	drawMessage(ops, dbX-7, gatewayX+7, 265, "1 row returned", true, false, seq, mTheme, gtx)

	// 4. Note over API Gateway: Verify Argon2 hash & issue JWT
	noteWidth := float32(180)
	noteHeight := float32(38)
	drawNote(ops, gatewayX-noteWidth/2, 310, noteWidth, noteHeight, "Verify Argon2 hash\n& issue token", seq, mTheme, gtx)

	// 5. API Gateway -->> Alice: 200 OK (JWT Bearer)
	drawMessage(ops, gatewayX-7, aliceX, 410, "200 OK (JWT Bearer)", true, false, seq, mTheme, gtx)

	// Top Actor Boxes
	drawActor(ops, aliceX, topY, actorWidth, actorHeight, "Alice", radius, seq, mTheme, gtx)
	drawActor(ops, gatewayX, topY, actorWidth, actorHeight, "API Gateway", radius, seq, mTheme, gtx)
	drawActor(ops, dbX, topY, actorWidth, actorHeight, "Database", radius, seq, mTheme, gtx)

	// Bottom Actor Boxes
	drawActor(ops, aliceX, bottomY, actorWidth, actorHeight, "Alice", radius, seq, mTheme, gtx)
	drawActor(ops, gatewayX, bottomY, actorWidth, actorHeight, "API Gateway", radius, seq, mTheme, gtx)
	drawActor(ops, dbX, bottomY, actorWidth, actorHeight, "Database", radius, seq, mTheme, gtx)

	// Title at the top
	drawTitle(ops, "Authentication Sequence Flow", mTheme, oTheme, gtx)

	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func drawTitle(ops *op.Ops, title string, th *material.Theme, oTh *Theme, gtx layout.Context) {
	defer op.Offset(image.Pt(40, 10)).Push(ops).Pop()
	lbl := material.Label(th, unit.Sp(15), title)
	lbl.Color = oTh.Foreground
	lbl.Font.Weight = font.Bold
	lbl.Layout(gtx)
}

func drawActor(ops *op.Ops, centerX, topY, w, h float32, name string, radius int, seq SeqTheme, th *material.Theme, gtx layout.Context) {
	left := int(centerX - w/2)
	top := int(topY)
	rect := image.Rect(left, top, left+int(w), top+int(h))

	// Background fill
	paint.FillShape(ops, seq.ActorBkg, clip.UniformRRect(rect, radius).Op(ops))

	// Border
	paint.FillShape(ops, seq.ActorBorder, clip.Stroke{
		Path:  clip.UniformRRect(rect, radius).Path(ops),
		Width: 1.5,
	}.Op())

	// Centered label
	textOffX := left
	textOffY := top + int(h/2) - 9
	defer op.Offset(image.Pt(textOffX, textOffY)).Push(ops).Pop()

	macroGtx := gtx
	macroGtx.Constraints.Min = image.Pt(int(w), 0)
	macroGtx.Constraints.Max = image.Pt(int(w), int(h))

	lbl := material.Label(th, unit.Sp(13), name)
	lbl.Color = seq.ActorText
	lbl.Alignment = text.Middle
	lbl.Font.Weight = font.SemiBold
	lbl.Layout(macroGtx)
}

func drawActivation(ops *op.Ops, centerX, topY, bottomY, w float32, seq SeqTheme) {
	left := int(centerX - w/2)
	rect := image.Rect(left, int(topY), left+int(w), int(bottomY))

	// Fill
	paint.FillShape(ops, seq.ActivationBkg, clip.Rect(rect).Op())
	// Border
	paint.FillShape(ops, seq.ActivationBorder, clip.Stroke{
		Path:  clip.Rect(rect).Path(),
		Width: 1.2,
	}.Op())
}

func drawNote(ops *op.Ops, left, top, w, h float32, content string, seq SeqTheme, th *material.Theme, gtx layout.Context) {
	rect := image.Rect(int(left), int(top), int(left+w), int(top+h))
	radius := 4

	// Background fill & border
	paint.FillShape(ops, seq.NoteBkg, clip.UniformRRect(rect, radius).Op(ops))
	paint.FillShape(ops, seq.NoteBorder, clip.Stroke{
		Path:  clip.UniformRRect(rect, radius).Path(ops),
		Width: 1.2,
	}.Op())

	defer op.Offset(image.Pt(int(left)+6, int(top)+4)).Push(ops).Pop()
	macroGtx := gtx
	macroGtx.Constraints.Min = image.Pt(int(w)-12, 0)
	macroGtx.Constraints.Max = image.Pt(int(w)-12, int(h))

	lbl := material.Label(th, unit.Sp(11), content)
	lbl.Color = seq.NoteText
	lbl.Alignment = text.Middle
	lbl.Layout(macroGtx)
}

func drawFrame(ops *op.Ops, left, top, right, bottom float32, label string, seq SeqTheme, th *material.Theme, gtx layout.Context) {
	rect := image.Rect(int(left), int(top), int(right), int(bottom))
	radius := int(seq.Rounding)

	// Frame background (semi-transparent)
	paint.FillShape(ops, seq.FrameBkg, clip.UniformRRect(rect, radius).Op(ops))

	// Frame border
	paint.FillShape(ops, seq.FrameBorder, clip.Stroke{
		Path:  clip.UniformRRect(rect, radius).Path(ops),
		Width: 1.0,
	}.Op())

	// Top-left label badge
	badgeW := float32(180)
	badgeH := float32(22)
	badgeRect := image.Rect(int(left), int(top), int(left+badgeW), int(top+badgeH))
	paint.FillShape(ops, seq.FrameBorder, clip.UniformRRect(badgeRect, 3).Op(ops))

	defer op.Offset(image.Pt(int(left)+8, int(top)+3)).Push(ops).Pop()
	lbl := material.Label(th, unit.Sp(10), label)
	lbl.Color = seq.ActorBkg
	lbl.Font.Weight = font.Bold
	lbl.Layout(gtx)
}

func drawMessage(ops *op.Ops, fromX, toX, y float32, label string, dashed bool, filledArrow bool, seq SeqTheme, th *material.Theme, gtx layout.Context) {
	goingRight := toX > fromX

	// Draw arrow line
	if dashed {
		drawDashedHLine(ops, fromX, toX, y, 1.5, seq.Signal, 6, 4)
	} else {
		drawSolidHLine(ops, fromX, toX, y, 1.5, seq.Signal)
	}

	// Draw arrowhead at toX
	if filledArrow {
		drawFilledArrow(ops, toX, y, goingRight, seq.Signal)
	} else {
		drawOpenArrow(ops, toX, y, goingRight, seq.Signal)
	}

	// Centered label above line
	minX := float32(math.Min(float64(fromX), float64(toX)))
	maxX := float32(math.Max(float64(fromX), float64(toX)))
	width := maxX - minX

	textY := int(y) - 18
	defer op.Offset(image.Pt(int(minX), textY)).Push(ops).Pop()

	macroGtx := gtx
	macroGtx.Constraints.Min = image.Pt(int(width), 0)
	macroGtx.Constraints.Max = image.Pt(int(width), 30)

	lbl := material.Label(th, unit.Sp(11), label)
	lbl.Color = seq.SignalText
	lbl.Alignment = text.Middle
	lbl.Layout(macroGtx)
}

func drawSolidHLine(ops *op.Ops, fromX, toX, y, width float32, col color.NRGBA) {
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(f32.Pt(fromX, y))
	p.LineTo(f32.Pt(toX, y))
	paint.FillShape(ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
}

func drawDashedHLine(ops *op.Ops, fromX, toX, y, width float32, col color.NRGBA, dash, gap float32) {
	startX := float32(math.Min(float64(fromX), float64(toX)))
	endX := float32(math.Max(float64(fromX), float64(toX)))

	for cur := startX; cur < endX; cur += dash + gap {
		segEnd := cur + dash
		if segEnd > endX {
			segEnd = endX
		}
		var p clip.Path
		p.Begin(ops)
		p.MoveTo(f32.Pt(cur, y))
		p.LineTo(f32.Pt(segEnd, y))
		paint.FillShape(ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
	}
}

func drawDashedVLine(ops *op.Ops, x, startY, endY, width float32, col color.NRGBA, dash, gap float32) {
	for cur := startY; cur < endY; cur += dash + gap {
		segEnd := cur + dash
		if segEnd > endY {
			segEnd = endY
		}
		var p clip.Path
		p.Begin(ops)
		p.MoveTo(f32.Pt(x, cur))
		p.LineTo(f32.Pt(x, segEnd))
		paint.FillShape(ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
	}
}

func drawFilledArrow(ops *op.Ops, tipX, tipY float32, pointingRight bool, col color.NRGBA) {
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(f32.Pt(tipX, tipY))
	if pointingRight {
		p.LineTo(f32.Pt(tipX-9, tipY-4))
		p.LineTo(f32.Pt(tipX-9, tipY+4))
	} else {
		p.LineTo(f32.Pt(tipX+9, tipY-4))
		p.LineTo(f32.Pt(tipX+9, tipY+4))
	}
	p.Close()
	paint.FillShape(ops, col, clip.Outline{Path: p.End()}.Op())
}

func drawOpenArrow(ops *op.Ops, tipX, tipY float32, pointingRight bool, col color.NRGBA) {
	var p clip.Path
	p.Begin(ops)
	if pointingRight {
		p.MoveTo(f32.Pt(tipX-8, tipY-4))
		p.LineTo(f32.Pt(tipX, tipY))
		p.LineTo(f32.Pt(tipX-8, tipY+4))
	} else {
		p.MoveTo(f32.Pt(tipX+8, tipY-4))
		p.LineTo(f32.Pt(tipX, tipY))
		p.LineTo(f32.Pt(tipX+8, tipY+4))
	}
	paint.FillShape(ops, col, clip.Stroke{Path: p.End(), Width: 1.5}.Op())
}
