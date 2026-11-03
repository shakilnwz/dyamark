// Package viewer is the fullscreen sequence diagram viewer: zoom at the
// pointer, pan, fit, the pinned participant row and focus-a-participant.
// It draws with Gio and needs nothing from the desktop, so the app and the
// web build share it.
package viewer

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"dyamark/palette"
	"dyamark/seq"
	"dyamark/seq/draw"
	layoutPkg "dyamark/seq/layout"
	"dyamark/seq/parse"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Viewer is a fullscreen, interactive diagram viewer widget supporting
// pan, zoom at pointer, fit to window, diagram navigation, and sticky actors.
type Viewer struct {
	AST      *parse.Diagram
	Diagram  *layoutPkg.Diagram
	Config   layoutPkg.Config
	Diagrams []*parse.Diagram
	CurIndex int

	Zoom   float32
	Pan    f32.Point
	fitted bool
	// numbersFlipped is set when N has flipped the diagram's own
	// sequence-number setting.
	numbersFlipped bool
	lastSize       image.Point
	pointerPos     f32.Point

	drag      gesture.Drag
	dragStart f32.Point
	panStart  f32.Point
	// moved is set once a press has moved far enough to be a drag rather
	// than a click.
	moved bool

	// Focus is the ID of the participant clicked on, whose messages,
	// notes and activations stay bright while the rest is dimmed.
	Focus string

	// chips holds the pinned strip's names, as last drawn, and metric
	// the screen's, for the strip's height.
	chips  []chip
	metric unit.Metric

	// infoSince is when the info line was last shown; wakeInfo shows it
	// again at the next frame.
	infoSince time.Time
	wakeInfo  bool

	// drawing keeps the diagram's drawing between frames, so panning
	// doesn't draw it again.
	drawing draw.Cache

	// anim eases the zoom and pan to where a key put them, rather than
	// jumping there. stepZoom is a zoom StepZoom asked for, for the next
	// layout, or 0.
	anim     zoomAnim
	stepZoom float32

	// Callbacks
	OnClose func()
	OnEdit  func(index int)
}

// New creates a diagram viewer showing diagrams[initialIndex].
func New(diagrams []*parse.Diagram, initialIndex int) *Viewer {
	v := &Viewer{
		Diagrams: diagrams,
		CurIndex: initialIndex,
		Zoom:     1.0,
	}
	if len(diagrams) > 0 && initialIndex >= 0 && initialIndex < len(diagrams) {
		v.SetAST(diagrams[initialIndex])
	}
	return v
}

// SetAST loads an AST into the viewer and computes its layout.
func (v *Viewer) SetAST(ast *parse.Diagram) {
	v.numbersFlipped = false
	v.Focus = ""
	v.wakeInfo = true
	v.layoutAST(ast)
	if ast != nil {
		v.fitted = false
	}
}

// layoutAST lays out ast with its own config, flipped by N if it was
// pressed.
func (v *Viewer) layoutAST(ast *parse.Diagram) {
	v.AST = ast
	if ast == nil {
		v.Diagram = nil
		return
	}
	v.Config = layoutPkg.ResolveConfig(ast.Config)
	if v.numbersFlipped {
		v.Config.ShowSequenceNumbers = !v.Config.ShowSequenceNumbers
	}
	diag, err := layoutPkg.LayoutWithConfig(ast, v.Config, layoutPkg.Default())
	if err == nil {
		v.Diagram = diag
	}
}

// SetDiagrams updates the collection of diagrams and picks the current index.
func (v *Viewer) SetDiagrams(diagrams []*parse.Diagram, index int) {
	v.Diagrams = diagrams
	if index < 0 {
		index = 0
	}
	if index >= len(diagrams) {
		index = len(diagrams) - 1
	}
	v.CurIndex = index
	if len(diagrams) > 0 && index >= 0 {
		v.SetAST(diagrams[index])
	}
}

// ReloadDiagrams swaps in a new version of the same diagrams, after the
// file is saved. index is where the diagram being shown is now, which
// moves when one before it stops or starts parsing; it keeps its zoom,
// pan and N toggle.
func (v *Viewer) ReloadDiagrams(diagrams []*parse.Diagram, index int) {
	if index < 0 || index >= len(diagrams) {
		v.SetDiagrams(diagrams, index)
		return
	}
	v.Diagrams = diagrams
	v.CurIndex = index
	v.layoutAST(diagrams[index])
	// The focused participant may have been renamed or removed.
	if v.Focus != "" && v.actorLabel(v.Focus) == "" {
		v.Focus = ""
	}
}

// ToggleSequenceNumbers toggles sequence numbers visibility.
func (v *Viewer) ToggleSequenceNumbers() {
	if v.AST == nil {
		return
	}
	v.numbersFlipped = !v.numbersFlipped
	v.Config.ShowSequenceNumbers = !v.Config.ShowSequenceNumbers
	diag, err := layoutPkg.LayoutWithConfig(v.AST, v.Config, layoutPkg.Default())
	if err == nil {
		v.Diagram = diag
	}
}

// NextDiagram advances to the next diagram in the collection.
func (v *Viewer) NextDiagram() {
	if len(v.Diagrams) <= 1 {
		return
	}
	v.CurIndex = (v.CurIndex + 1) % len(v.Diagrams)
	v.SetAST(v.Diagrams[v.CurIndex])
}

// PrevDiagram moves to the previous diagram in the collection.
func (v *Viewer) PrevDiagram() {
	if len(v.Diagrams) <= 1 {
		return
	}
	v.CurIndex = (v.CurIndex - 1 + len(v.Diagrams)) % len(v.Diagrams)
	v.SetAST(v.Diagrams[v.CurIndex])
}

// ZoomBy scales the zoom by factor around point P in screen coordinates.
func (v *Viewer) ZoomBy(factor float32, p f32.Point) {
	if v.Zoom <= 0 {
		v.Zoom = 1.0
	}
	newZoom := v.Zoom * factor
	if newZoom < 0.05 {
		newZoom = 0.05
	} else if newZoom > 20.0 {
		newZoom = 20.0
	}

	ratio := newZoom / v.Zoom
	v.Pan = p.Sub(p.Sub(v.Pan).Mul(ratio))
	v.Zoom = newZoom
}

// Fit scales and centers the diagram inside the given viewport dimensions.
func (v *Viewer) Fit(winW, winH int) {
	if v.Diagram == nil || v.Diagram.ViewBox.W <= 0 || v.Diagram.ViewBox.H <= 0 {
		return
	}
	const pad = float32(32.0)
	diagW := float32(v.Diagram.ViewBox.W)
	diagH := float32(v.Diagram.ViewBox.H)

	availW := float32(winW) - 2*pad
	availH := float32(winH) - 2*pad
	if availW <= 0 || availH <= 0 {
		return
	}

	scaleX := availW / diagW
	scaleY := availH / diagH
	v.Zoom = scaleX
	if scaleY < scaleX {
		v.Zoom = scaleY
	}

	v.Pan.X = (float32(winW) - diagW*v.Zoom) / 2
	v.Pan.Y = (float32(winH) - diagH*v.Zoom) / 2
}

// FitWidth fits the diagram width to the window width.
func (v *Viewer) FitWidth(winW, winH int) {
	if v.Diagram == nil || v.Diagram.ViewBox.W <= 0 {
		return
	}
	const pad = float32(32.0)
	diagW := float32(v.Diagram.ViewBox.W)
	availW := float32(winW) - 2*pad
	if availW <= 0 {
		return
	}
	v.Zoom = availW / diagW
	v.Pan.X = pad
	v.Pan.Y = pad
}

// FitHeight fits the diagram height to the window height.
func (v *Viewer) FitHeight(winW, winH int) {
	if v.Diagram == nil || v.Diagram.ViewBox.H <= 0 {
		return
	}
	const pad = float32(32.0)
	diagH := float32(v.Diagram.ViewBox.H)
	availH := float32(winH) - 2*pad
	if availH <= 0 {
		return
	}
	v.Zoom = availH / diagH
	v.Pan.X = (float32(winW) - float32(v.Diagram.ViewBox.W)*v.Zoom) / 2
	v.Pan.Y = pad
}

// Set100 resets zoom to 100% and centers the diagram.
func (v *Viewer) Set100(winW, winH int) {
	if v.Diagram == nil {
		return
	}
	v.Zoom = 1.0
	diagW := float32(v.Diagram.ViewBox.W)
	diagH := float32(v.Diagram.ViewBox.H)
	v.Pan.X = (float32(winW) - diagW) / 2
	v.Pan.Y = (float32(winH) - diagH) / 2
}

// Layout handles events and renders the viewer.
func (v *Viewer) Layout(gtx layout.Context, th *material.Theme, oTheme *palette.Theme) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{}
	}
	v.metric = gtx.Metric

	if !v.fitted || size != v.lastSize {
		v.anim.on = false
		v.Fit(size.X, size.Y)
		v.fitted = true
		v.lastSize = size
	}

	v.handleEvents(gtx, size)
	v.stepAnim(gtx)

	// Background fill
	paint.FillShape(gtx.Ops, oTheme.Background, clip.Rect(image.Rectangle{Max: size}).Op())

	// A closed hand while dragging, a pointing one over a participant
	// that can be clicked, an open one elsewhere.
	switch {
	case v.drag.Dragging():
		pointer.CursorGrabbing.Add(gtx.Ops)
	case v.actorAt(v.pointerPos, size) != "":
		pointer.CursorPointer.Add(gtx.Ops)
	default:
		pointer.CursorGrab.Add(gtx.Ops)
	}

	// Set up hit-testing area for pointer events (drag gesture and viewer events)
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	v.drag.Add(gtx.Ops)
	event.Op(gtx.Ops, v)
	area.Pop()

	if v.Diagram == nil {
		return layout.Dimensions{Size: size}
	}

	// Resolve diagram theme
	themeName := ""
	var themeVars map[string]string
	if v.AST != nil {
		themeName = v.AST.Config.Theme
		themeVars = v.AST.Config.ThemeVariables
	}
	diagTheme := seq.ResolveTheme(oTheme.Seq, themeName, themeVars)

	// Draw diagram transformed by (Pan, Zoom)
	v.drawDiagram(gtx, th, diagTheme, oTheme.Background, size)

	// Draw sticky participant header if scrolled down
	v.drawStickyActors(gtx, th, diagTheme, oTheme, size)

	if v.Focus != "" {
		v.drawFocusHint(gtx, th, oTheme, size)
	}
	v.drawInfo(gtx, th, oTheme, size)

	return layout.Dimensions{Size: size}
}

func (v *Viewer) handleEvents(gtx layout.Context, size image.Point) {
	center := f32.Pt(float32(size.X)/2, float32(size.Y)/2)

	// Process drag gesture (move tool)
	for {
		ev, ok := v.drag.Update(gtx.Metric, gtx.Source, gesture.Both)
		if !ok {
			break
		}
		switch ev.Kind {
		case pointer.Press:
			v.anim.on = false
			v.dragStart = ev.Position
			v.panStart = v.Pan
			v.moved = false
		case pointer.Drag:
			delta := ev.Position.Sub(v.dragStart)
			if delta.X*delta.X+delta.Y*delta.Y > clickSlop*clickSlop {
				v.moved = true
			}
			v.Pan = v.panStart.Add(delta)
			gtx.Execute(op.InvalidateCmd{})
		case pointer.Release:
			if !v.moved {
				// A press that didn't move is a click: on a participant
				// it focuses it, again or elsewhere it lets go.
				v.Pan = v.panStart
				id := v.actorAt(ev.Position, size)
				if id == v.Focus {
					id = ""
				}
				v.Focus = id
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}

	for {
		ev, ok := gtx.Event(
			pointer.Filter{
				Target:  v,
				Kinds:   pointer.Scroll | pointer.Move,
				ScrollX: pointer.ScrollRange{Min: -1000000, Max: 1000000},
				ScrollY: pointer.ScrollRange{Min: -1000000, Max: 1000000},
			},
			key.Filter{Focus: v},
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: "0"},
			key.Filter{Name: "1"},
			key.Filter{Name: "W"},
			key.Filter{Name: "H"},
			key.Filter{Name: "N"},
			key.Filter{Name: "E"},
			key.Filter{Name: "["},
			key.Filter{Name: "]"},
			// `+` and `_` are usually typed with Shift, which Gio reports.
			key.Filter{Name: "+", Optional: key.ModShift},
			key.Filter{Name: "="},
			key.Filter{Name: "-"},
			key.Filter{Name: "_", Optional: key.ModShift},
		)
		if !ok {
			break
		}

		switch e := ev.(type) {
		case pointer.Event:
			v.pointerPos = e.Position
			switch e.Kind {
			case pointer.Move:
				v.pointerPos = e.Position
			case pointer.Scroll:
				v.wakeInfo = true
				v.anim.on = false
				if e.Scroll.Y != 0 {
					v.ZoomBy(wheelZoom(e.Scroll.Y/gtx.Metric.PxPerDp), e.Position)
					gtx.Execute(op.InvalidateCmd{})
				}
			}

		case key.Event:
			if e.State != key.Press {
				break
			}
			v.wakeInfo = true
			p := v.pointerPos
			if p == (f32.Point{}) {
				p = center
			}

			switch e.Name {
			case key.NameEscape:
				switch {
				case v.Focus != "":
					v.Focus = ""
				case v.OnClose != nil:
					v.OnClose()
				}
			case "0":
				v.ease(gtx, func() { v.Fit(size.X, size.Y) })
			case "1":
				v.ease(gtx, func() { v.Set100(size.X, size.Y) })
			case "W":
				v.ease(gtx, func() { v.FitWidth(size.X, size.Y) })
			case "H":
				v.ease(gtx, func() { v.FitHeight(size.X, size.Y) })
			case "+", "=":
				v.ease(gtx, func() { v.ZoomBy(1.2, p) })
			case "-", "_":
				v.ease(gtx, func() { v.ZoomBy(1.0/1.2, p) })
			case "N":
				v.ToggleSequenceNumbers()
			case "[":
				v.PrevDiagram()
			case "]":
				v.NextDiagram()
			case "E":
				if v.OnEdit != nil {
					v.OnEdit(v.CurIndex)
				}
			}
		}
	}

	if f := v.stepZoom; f != 0 {
		v.stepZoom = 0
		v.wakeInfo = true
		v.ease(gtx, func() { v.ZoomBy(f, center) })
	}
}

// StepZoom zooms by factor around the middle of the viewer, eased, at its
// next layout: for zoom actions from outside it, such as the IDE's.
func (v *Viewer) StepZoom(factor float32) {
	if v.stepZoom == 0 {
		v.stepZoom = 1
	}
	v.stepZoom *= factor
}

// zoomTime is how long the zoom keys take to ease to their zoom.
const zoomTime = 180 * time.Millisecond

// zoomAnim eases the zoom and pan from one view of the diagram to
// another.
type zoomAnim struct {
	on               bool
	start            time.Time
	fromZoom, toZoom float32
	fromPan, toPan   f32.Point
}

// ease runs f, which sets the zoom and pan, and eases them there from
// where they are rather than jumping. One under way goes on from where
// it was going, so quick presses add up. Without a clock, as in tests,
// it jumps.
func (v *Viewer) ease(gtx layout.Context, f func()) {
	zoom, pan := v.Zoom, v.Pan
	if v.anim.on {
		v.Zoom, v.Pan = v.anim.toZoom, v.anim.toPan
	}
	f()
	if gtx.Now.IsZero() {
		v.anim.on = false
		return
	}
	v.anim = zoomAnim{on: true, start: gtx.Now, fromZoom: zoom, toZoom: v.Zoom, fromPan: pan, toPan: v.Pan}
	v.Zoom, v.Pan = zoom, pan
	gtx.Execute(op.InvalidateCmd{})
}

// stepAnim sets the zoom and pan for this frame of the easing, if one is
// under way, and asks for the next.
func (v *Viewer) stepAnim(gtx layout.Context) {
	a := &v.anim
	if !a.on {
		return
	}
	t := float32(gtx.Now.Sub(a.start)) / float32(zoomTime)
	if t >= 1 || gtx.Now.IsZero() {
		v.Zoom, v.Pan = a.toZoom, a.toPan
		a.on = false
		return
	}
	t = max(t, 0)
	// Ease out: quick at first, settling at the end.
	e := 1 - (1-t)*(1-t)*(1-t)
	v.Zoom, v.Pan = a.at(e)
	gtx.Execute(op.InvalidateCmd{})
}

// at returns the zoom and pan a fraction e of the way. The zoom goes by
// equal ratios, and the pan keeps still the point both ends show in the
// same place, as zooming at a point does, so the view doesn't swing.
func (a zoomAnim) at(e float32) (float32, f32.Point) {
	z0, z1 := a.fromZoom, a.toZoom
	p0, p1 := a.fromPan, a.toPan
	z := z0 * float32(math.Pow(float64(z1/z0), float64(e)))
	if math.Abs(float64(z1-z0)) < 1e-3*float64(z0) {
		// The same zoom: only the pan moves.
		return z, p0.Add(p1.Sub(p0).Mul(e))
	}
	// The fixed point q, on each axis: (q-p0)/z0 = (q-p1)/z1.
	q := p0.Mul(z1).Sub(p1.Mul(z0)).Div(z1 - z0)
	return z, q.Sub(q.Sub(p0).Mul(z / z0))
}

func (v *Viewer) drawDiagram(gtx layout.Context, th *material.Theme, seqTh seq.Theme, bg color.NRGBA, size image.Point) {
	// Clip to viewer bounds
	clipStack := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	defer clipStack.Pop()

	// Apply Zoom scaling first, then Pan offset in true screen coordinates
	trans := op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(v.Zoom, v.Zoom)).Offset(v.Pan)).Push(gtx.Ops)
	defer trans.Pop()

	diagGtx := gtx
	diagGtx.Metric = unit.Metric{PxPerDp: 1.0, PxPerSp: 1.0}
	diagGtx.Constraints = layout.Exact(image.Pt(int(v.Diagram.ViewBox.W), int(v.Diagram.ViewBox.H)))

	key := drawingKey{v.Diagram, th, th.Shaper, seqTh, bg, v.Focus}
	v.drawing.Layout(diagGtx, key, func(gtx layout.Context) layout.Dimensions {
		dims := draw.Draw(gtx, th, seqTh, v.Diagram)
		if v.Focus != "" {
			v.dimExcept(gtx, th, seqTh, v.Diagram, bg)
		}
		return dims
	})
}

// drawingKey is what the viewer's drawing of a diagram depends on.
type drawingKey struct {
	diag   *layoutPkg.Diagram
	th     *material.Theme
	shaper *text.Shaper
	seq    seq.Theme
	bg     color.NRGBA
	focus  string
}

// clickSlop is how far, in px, a press may move and still be a click.
const clickSlop = 4

const (
	// wheelStep is the zoom for a wheel's notch.
	wheelStep = 1.15
	// wheelNotch is the scrolling, in dp, that zooms by wheelStep. A
	// notch scrolls at least as far, 50 dp or more in the app and 120 px
	// in the IDE's browser, so each still zooms by wheelStep.
	wheelNotch = 40
)

// wheelZoom returns the zoom for d dp of wheel scrolling: in proportion
// to it, so that a touchpad's many small steps zoom a little each, and
// at most wheelStep, a notch's, either way.
func wheelZoom(d float32) float32 {
	f := math.Pow(wheelStep, -float64(d)/wheelNotch)
	return float32(min(max(f, 1/wheelStep), wheelStep))
}

// dimExcept washes diag over with bg and draws the parts involving the
// focused participant again on top, at full strength.
func (v *Viewer) dimExcept(gtx layout.Context, th *material.Theme, seqTh seq.Theme, diag *layoutPkg.Diagram, bg color.NRGBA) {
	vb := diag.ViewBox
	paint.FillShape(gtx.Ops, palette.WithAlpha(bg, 200), clip.Rect(image.Rect(0, 0, int(vb.W)+1, int(vb.H)+1)).Op())
	draw.Draw(gtx, th, seqTh, diag.Involving(v.Focus))
}

// stickyRow returns the participant row once the diagram is scrolled up
// past it, when it's pinned to the top of the viewer as a strip of names:
// the actors in it and the strip's height, in screen px. ok is false while
// the row is still on screen by itself.
func (v *Viewer) stickyRow() (actors []layoutPkg.Actor, headerH float32, ok bool) {
	if v.Diagram == nil {
		return nil, 0, false
	}
	// The row is the heads at the top; a participant created later has
	// its head further down, and the feet are at the bottom.
	rowY := float32(math.Inf(1))
	for _, a := range v.Diagram.Actors {
		if !a.Footer {
			rowY = min(rowY, a.Rect.Y)
		}
	}
	for _, a := range v.Diagram.Actors {
		if !a.Footer && a.Rect.Y <= rowY+1 {
			actors = append(actors, a)
		}
	}
	if len(actors) == 0 {
		return nil, 0, false
	}
	if v.Pan.Y+(rowY-v.Diagram.ViewBox.Y)*v.Zoom >= 0 {
		return nil, 0, false
	}
	// The strip is as tall whatever the zoom, so zoomed in it doesn't
	// take the window.
	return actors, float32(v.metric.Dp(stickyHeight)), true
}

const (
	// stickyHeight is the pinned strip's height, and chipHeight that of
	// a name in it, in dp.
	stickyHeight = 30
	chipHeight   = 22
	// chipMaxWidth is the widest a name gets, in dp.
	chipMaxWidth = 240
)

// chip is a participant's name in the pinned strip, as last drawn.
type chip struct {
	id string
	// label is the name in full, and cut set when it's shortened to fit.
	label string
	cut   bool
	rect  image.Rectangle
}

// lifelineX returns where actor a's lifeline is on screen.
func (v *Viewer) lifelineX(a layoutPkg.Actor) float32 {
	return v.Pan.X + (a.Rect.X+a.Rect.W/2-v.Diagram.ViewBox.X)*v.Zoom
}

// toDiagram turns a point on screen into diagram coordinates.
func (v *Viewer) toDiagram(p f32.Point) f32.Point {
	vb := v.Diagram.ViewBox
	return f32.Pt((p.X-v.Pan.X)/v.Zoom+vb.X, (p.Y-v.Pan.Y)/v.Zoom+vb.Y)
}

// actorAt returns the ID of the participant box at p on screen, pinned or
// not, or "".
func (v *Viewer) actorAt(p f32.Point, size image.Point) string {
	if v.Diagram == nil || v.Zoom <= 0 {
		return ""
	}
	in := func(a layoutPkg.Actor, q f32.Point) bool {
		r := a.Rect
		return q.X >= r.X && q.X <= r.X+r.W && q.Y >= r.Y && q.Y <= r.Y+r.H
	}
	if _, headerH, ok := v.stickyRow(); ok && p.Y < headerH {
		// The pinned strip covers the diagram under it.
		for _, c := range v.chips {
			if image.Pt(int(p.X), int(p.Y)).In(c.rect) {
				return c.id
			}
		}
		return ""
	}
	q := v.toDiagram(p)
	for _, a := range v.Diagram.Actors {
		if in(a, q) {
			return a.ID
		}
	}
	return ""
}

// actorLabel returns the participant id's label as drawn, or "" if the
// diagram has no such participant.
func (v *Viewer) actorLabel(id string) string {
	if v.Diagram == nil {
		return ""
	}
	for _, a := range v.Diagram.Actors {
		if a.ID != id {
			continue
		}
		var lines []string
		for _, t := range a.Label {
			lines = append(lines, t.Text)
		}
		if label := strings.Join(lines, " "); label != "" {
			return label
		}
		return id
	}
	return ""
}

// drawStickyActors pins the participants' names to the top of the viewer,
// each over its lifeline, once a tall diagram is scrolled up past them.
// A name too long for the room between lifelines is cut short, and shown
// in full while the pointer is on it.
func (v *Viewer) drawStickyActors(gtx layout.Context, th *material.Theme, seqTh seq.Theme, oTheme *palette.Theme, size image.Point) {
	v.chips = v.chips[:0]
	row, headerH, ok := v.stickyRow()
	if !ok {
		return
	}
	h := int(headerH)
	paint.FillShape(gtx.Ops, oTheme.Background, clip.Rect(image.Rect(0, 0, size.X, h)).Op())
	paint.FillShape(gtx.Ops, palette.WithAlpha(oTheme.Foreground, 40), clip.Rect(image.Rect(0, h-1, size.X, h)).Op())

	// label records a name in one line at most maxW wide.
	label := func(s string, c color.NRGBA, maxW int) (op.CallOp, image.Point) {
		gtx := gtx
		gtx.Constraints = layout.Constraints{Max: image.Pt(max(maxW, 0), h)}
		rec := op.Record(gtx.Ops)
		l := material.Caption(th, s)
		l.Color = c
		l.MaxLines = 1
		dims := l.Layout(gtx)
		return rec.Stop(), dims.Size
	}
	padX := gtx.Dp(8)
	gap := gtx.Dp(6)
	ch := gtx.Dp(chipHeight)
	r := gtx.Dp(unit.Dp(min(seqTh.Rounding, 6)))
	var tip *chip
	for i, a := range row {
		cx := v.lifelineX(a)
		// A name has the room up to halfway to its neighbours' lifelines.
		room := float32(gtx.Dp(chipMaxWidth))
		if i > 0 {
			room = min(room, cx-v.lifelineX(row[i-1]))
		}
		if i+1 < len(row) {
			room = min(room, v.lifelineX(row[i+1])-cx)
		}
		maxW := max(int(room)-gap, 2*padX+gtx.Dp(12))
		c := chip{id: a.ID, label: v.actorLabel(a.ID)}

		text, sz := label(c.label, seqTh.ActorText, gtx.Dp(chipMaxWidth))
		if sz.X > maxW-2*padX {
			c.cut = true
			text, sz = label(c.label, seqTh.ActorText, maxW-2*padX)
		}
		w := sz.X + 2*padX
		c.rect = image.Rect(int(cx)-w/2, (h-ch)/2, int(cx)-w/2+w, (h-ch)/2+ch)
		if c.rect.Max.X < 0 || c.rect.Min.X > size.X {
			continue
		}

		// Behind a focused participant the others are faint.
		alpha := uint8(255)
		if v.Focus != "" && v.Focus != a.ID {
			alpha = 90
		}
		fade := func(col color.NRGBA) color.NRGBA {
			col.A = uint8(uint16(col.A) * uint16(alpha) / 255)
			return col
		}
		rr := clip.UniformRRect(c.rect, r)
		paint.FillShape(gtx.Ops, fade(seqTh.ActorBkg), rr.Op(gtx.Ops))
		paint.FillShape(gtx.Ops, fade(seqTh.ActorBorder), clip.Stroke{Path: rr.Path(gtx.Ops), Width: 1}.Op())
		off := op.Offset(image.Pt(c.rect.Min.X+padX, c.rect.Min.Y+(ch-sz.Y)/2)).Push(gtx.Ops)
		opacity := paint.PushOpacity(gtx.Ops, float32(alpha)/255)
		text.Add(gtx.Ops)
		opacity.Pop()
		off.Pop()

		v.chips = append(v.chips, c)
		if c.cut && image.Pt(int(v.pointerPos.X), int(v.pointerPos.Y)).In(c.rect) {
			tip = &v.chips[len(v.chips)-1]
		}
	}
	if tip == nil {
		return
	}
	// The full name, just under the strip.
	text, sz := label(tip.label, oTheme.Foreground, size.X-4*padX)
	box := image.Rect(0, h+gap, sz.X+2*padX, h+gap+sz.Y+gap)
	box = box.Add(image.Pt(min(max(tip.rect.Min.X, padX), max(size.X-box.Dx()-padX, 0)), 0))
	tr := clip.UniformRRect(box, gtx.Dp(unit.Dp(min(float32(oTheme.Rounding), 6))))
	paint.FillShape(gtx.Ops, oTheme.LighterBackground, tr.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, palette.WithAlpha(oTheme.Foreground, 40), clip.Stroke{Path: tr.Path(gtx.Ops), Width: 1}.Op())
	off := op.Offset(image.Pt(box.Min.X+padX, box.Min.Y+gap/2)).Push(gtx.Ops)
	text.Add(gtx.Ops)
	off.Pop()
}

// The info line shows for infoShow, then fades out over infoFade.
const (
	infoShow = 2500 * time.Millisecond
	infoFade = 500 * time.Millisecond
)

// infoText returns the info line: the diagram's title, which of the
// file's diagrams it is, the zoom, and the key for the key list.
func (v *Viewer) infoText() string {
	parts := []string{"Diagram"}
	if v.AST != nil && v.AST.Title != "" {
		parts[0] = v.AST.Title
	}
	if n := len(v.Diagrams); n > 1 {
		parts = append(parts, fmt.Sprintf("%d of %d", v.CurIndex+1, n))
	}
	parts = append(parts, fmt.Sprintf("%d %%", int(math.Round(float64(v.Zoom*100)))), "? for keys")
	return strings.Join(parts, " · ")
}

// infoAlpha returns how opaque the info line is at now, from 0 to 1.
func (v *Viewer) infoAlpha(now time.Time) float32 {
	if v.infoSince.IsZero() {
		return 0
	}
	since := now.Sub(v.infoSince)
	switch {
	case since < infoShow:
		return 1
	case since < infoShow+infoFade:
		return 1 - float32(since-infoShow)/float32(infoFade)
	}
	return 0
}

// drawInfo draws the info line in the bottom right corner, after a
// diagram opens or a key or zoom, fading after a moment.
func (v *Viewer) drawInfo(gtx layout.Context, th *material.Theme, oTheme *palette.Theme, size image.Point) {
	if v.wakeInfo {
		v.wakeInfo = false
		v.infoSince = gtx.Now
	}
	alpha := v.infoAlpha(gtx.Now)
	if alpha <= 0 {
		return
	}
	if since := gtx.Now.Sub(v.infoSince); since < infoShow {
		gtx.Execute(op.InvalidateCmd{At: v.infoSince.Add(infoShow)})
	} else {
		gtx.Execute(op.InvalidateCmd{})
	}
	fade := func(c color.NRGBA) color.NRGBA {
		c.A = uint8(float32(c.A) * alpha)
		return c
	}
	pad := gtx.Dp(8)
	rec := op.Record(gtx.Ops)
	lbl := material.Body2(th, v.infoText())
	lbl.Color = fade(oTheme.Foreground)
	lbl.MaxLines = 1
	inner := gtx
	inner.Constraints = layout.Constraints{Max: image.Pt(max(size.X/2, 0), size.Y)}
	dims := lbl.Layout(inner)
	text := rec.Stop()

	box := image.Rectangle{Min: image.Pt(size.X-dims.Size.X-3*pad, size.Y-dims.Size.Y-3*pad)}
	box.Max = box.Min.Add(dims.Size.Add(image.Pt(2*pad, 2*pad)))
	r := gtx.Dp(unit.Dp(float32(oTheme.Rounding)))
	paint.FillShape(gtx.Ops, fade(palette.WithAlpha(oTheme.LighterBackground, 230)), clip.UniformRRect(box, r).Op(gtx.Ops))
	off := op.Offset(box.Min.Add(image.Pt(pad, pad))).Push(gtx.Ops)
	text.Add(gtx.Ops)
	off.Pop()
}

// drawFocusHint says at the bottom of the viewer who is focused and how
// to stop.
func (v *Viewer) drawFocusHint(gtx layout.Context, th *material.Theme, oTheme *palette.Theme, size image.Point) {
	pad := gtx.Dp(8)
	rec := op.Record(gtx.Ops)
	lbl := material.Body2(th, "Showing "+v.actorLabel(v.Focus)+" · click it again or press Esc to show all")
	lbl.Color = oTheme.Foreground
	lbl.MaxLines = 1
	inner := gtx
	inner.Constraints = layout.Constraints{Max: image.Pt(max(size.X-4*pad, 0), size.Y)}
	dims := lbl.Layout(inner)
	text := rec.Stop()

	box := image.Rectangle{Min: image.Pt(pad, size.Y-dims.Size.Y-3*pad)}
	box.Max = box.Min.Add(dims.Size.Add(image.Pt(2*pad, 2*pad)))
	r := gtx.Dp(unit.Dp(float32(oTheme.Rounding)))
	paint.FillShape(gtx.Ops, palette.WithAlpha(oTheme.LighterBackground, 230), clip.UniformRRect(box, r).Op(gtx.Ops))
	off := op.Offset(box.Min.Add(image.Pt(pad, pad))).Push(gtx.Ops)
	text.Add(gtx.Ops)
	off.Pop()
}
