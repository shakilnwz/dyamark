package app

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

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

	// infoSince is when the info line was last shown; wakeInfo shows it
	// again at the next frame.
	infoSince time.Time
	wakeInfo  bool

	// Callbacks
	OnClose func()
	OnEdit  func(index int)
}

// NewViewer creates a new diagram viewer.
func NewViewer(diagrams []*parse.Diagram, initialIndex int) *Viewer {
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
func (v *Viewer) Layout(gtx layout.Context, th *material.Theme, oTheme *Theme) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{}
	}

	if !v.fitted || size != v.lastSize {
		v.Fit(size.X, size.Y)
		v.fitted = true
		v.lastSize = size
	}

	v.handleEvents(gtx, size)

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
				if e.Scroll.Y < 0 {
					v.ZoomBy(1.15, e.Position)
					gtx.Execute(op.InvalidateCmd{})
				} else if e.Scroll.Y > 0 {
					v.ZoomBy(1.0/1.15, e.Position)
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
				v.Fit(size.X, size.Y)
			case "1":
				v.Set100(size.X, size.Y)
			case "W":
				v.FitWidth(size.X, size.Y)
			case "H":
				v.FitHeight(size.X, size.Y)
			case "+", "=":
				v.ZoomBy(1.2, p)
			case "-", "_":
				v.ZoomBy(1.0/1.2, p)
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

	draw.Draw(diagGtx, th, seqTh, v.Diagram)
	if v.Focus != "" {
		v.dimExcept(diagGtx, th, seqTh, v.Diagram, bg)
	}
}

// clickSlop is how far, in px, a press may move and still be a click.
const clickSlop = 4

// dimExcept washes diag over with bg and draws the parts involving the
// focused participant again on top, at full strength.
func (v *Viewer) dimExcept(gtx layout.Context, th *material.Theme, seqTh seq.Theme, diag *layoutPkg.Diagram, bg color.NRGBA) {
	vb := diag.ViewBox
	paint.FillShape(gtx.Ops, withAlpha(bg, 200), clip.Rect(image.Rect(0, 0, int(vb.W)+1, int(vb.H)+1)).Op())
	draw.Draw(gtx, th, seqTh, diag.Involving(v.Focus))
}

// stickyRow returns the participant row as it's pinned to the top of the
// viewer once the diagram is scrolled up past it: the actors in it, its
// header's height and where it puts the actors' top, in screen px. ok is
// false while the row is still on screen by itself.
func (v *Viewer) stickyRow() (actors []layoutPkg.Actor, headerH, top, scale float32, ok bool) {
	if v.Diagram == nil {
		return nil, 0, 0, 0, false
	}
	// The row is the heads at the top; a participant created later has
	// its head further down, and the feet are at the bottom.
	rowY := float32(math.Inf(1))
	for _, a := range v.Diagram.Actors {
		if !a.Footer {
			rowY = min(rowY, a.Rect.Y)
		}
	}
	var maxH float32
	for _, a := range v.Diagram.Actors {
		if !a.Footer && a.Rect.Y <= rowY+1 {
			actors = append(actors, a)
			maxH = max(maxH, a.Rect.H)
		}
	}
	if len(actors) == 0 {
		return nil, 0, 0, 0, false
	}
	if v.Pan.Y+(rowY-v.Diagram.ViewBox.Y)*v.Zoom >= 0 {
		return nil, 0, 0, 0, false
	}
	// Zoomed in, the row stays at 100 % so it doesn't take the window.
	scale = min(v.Zoom, 1)
	return actors, max((maxH+20)*scale, 44), stickyPad, scale, true
}

// pinnedRect returns where actor a of the pinned row, drawn at scale with
// its top at top, is on screen: centred on its lifeline.
func (v *Viewer) pinnedRect(a layoutPkg.Actor, top, scale float32) (min, max f32.Point) {
	cx := v.Pan.X + (a.Rect.X+a.Rect.W/2-v.Diagram.ViewBox.X)*v.Zoom
	min = f32.Pt(cx-a.Rect.W/2*scale, top)
	return min, min.Add(f32.Pt(a.Rect.W*scale, a.Rect.H*scale))
}

// stickyPad is the gap above the pinned participant row.
const stickyPad = 10

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
	if row, headerH, top, scale, ok := v.stickyRow(); ok && p.Y < headerH {
		// The pinned row covers the diagram under it.
		for _, a := range row {
			lo, hi := v.pinnedRect(a, top, scale)
			if p.X >= lo.X && p.X <= hi.X && p.Y >= lo.Y && p.Y <= hi.Y {
				return a.ID
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

// drawStickyActors pins the participant row to the top of the viewer once
// a tall diagram is scrolled up past it.
func (v *Viewer) drawStickyActors(gtx layout.Context, th *material.Theme, seqTh seq.Theme, oTheme *Theme, size image.Point) {
	row, headerH, top, scale, ok := v.stickyRow()
	if !ok {
		return
	}

	// Draw sticky header background strip
	hdrRect := clip.Rect(image.Rectangle{Max: image.Pt(size.X, int(headerH))})
	hdrClip := hdrRect.Push(gtx.Ops)
	paint.FillShape(gtx.Ops, oTheme.Background, hdrRect.Op())

	// Bottom border for sticky header
	sepY := headerH - 1
	var sep clip.Path
	sep.Begin(gtx.Ops)
	sep.MoveTo(f32.Pt(0, sepY))
	sep.LineTo(f32.Pt(float32(size.X), sepY))
	paint.FillShape(gtx.Ops, withAlpha(oTheme.Foreground, 40), clip.Stroke{Path: sep.End(), Width: 1}.Op())

	diagGtx := gtx
	diagGtx.Metric = unit.Metric{PxPerDp: 1.0, PxPerSp: 1.0}
	// Each participant is drawn on its own over its lifeline: the row's
	// scale can be less than the diagram's.
	for _, a := range row {
		lo, _ := v.pinnedRect(a, top, scale)
		trans := op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(scale, scale)).Offset(lo)).Push(gtx.Ops)
		vb := v.Diagram.ViewBox
		vb.X, vb.Y = a.Rect.X, a.Rect.Y
		subDiag := &layoutPkg.Diagram{Actors: []layoutPkg.Actor{a}, ViewBox: vb}
		draw.Draw(diagGtx, th, seqTh, subDiag)
		if v.Focus != "" {
			v.dimExcept(diagGtx, th, seqTh, subDiag, oTheme.Background)
		}
		trans.Pop()
	}
	hdrClip.Pop()
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
func (v *Viewer) drawInfo(gtx layout.Context, th *material.Theme, oTheme *Theme, size image.Point) {
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
	paint.FillShape(gtx.Ops, fade(withAlpha(oTheme.LighterBackground, 230)), clip.UniformRRect(box, r).Op(gtx.Ops))
	off := op.Offset(box.Min.Add(image.Pt(pad, pad))).Push(gtx.Ops)
	text.Add(gtx.Ops)
	off.Pop()
}

// drawFocusHint says at the bottom of the viewer who is focused and how
// to stop.
func (v *Viewer) drawFocusHint(gtx layout.Context, th *material.Theme, oTheme *Theme, size image.Point) {
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
	paint.FillShape(gtx.Ops, withAlpha(oTheme.LighterBackground, 230), clip.UniformRRect(box, r).Op(gtx.Ops))
	off := op.Offset(box.Min.Add(image.Pt(pad, pad))).Push(gtx.Ops)
	text.Add(gtx.Ops)
	off.Pop()
}
