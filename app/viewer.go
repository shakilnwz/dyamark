package app

import (
	"image"

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

	Zoom       float32
	Pan        f32.Point
	fitted     bool
	lastSize   image.Point
	pointerPos f32.Point

	drag      gesture.Drag
	dragStart f32.Point
	panStart  f32.Point

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
	v.AST = ast
	if ast == nil {
		v.Diagram = nil
		return
	}
	v.Config = layoutPkg.ResolveConfig(ast.Config)
	diag, err := layoutPkg.LayoutWithConfig(ast, v.Config, layoutPkg.Default())
	if err == nil {
		v.Diagram = diag
	}
	v.fitted = false
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

// ToggleSequenceNumbers toggles sequence numbers visibility.
func (v *Viewer) ToggleSequenceNumbers() {
	if v.AST == nil {
		return
	}
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

	// Set cursor shape: open hand (Grab) or closed hand (Grabbing)
	if v.drag.Dragging() {
		pointer.CursorGrabbing.Add(gtx.Ops)
	} else {
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
	v.drawDiagram(gtx, th, diagTheme, size)

	// Draw sticky participant header if scrolled down
	v.drawStickyActors(gtx, th, diagTheme, oTheme, size)

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
		case pointer.Drag:
			delta := ev.Position.Sub(v.dragStart)
			v.Pan = v.panStart.Add(delta)
			gtx.Execute(op.InvalidateCmd{})
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
			key.Filter{Name: "+"},
			key.Filter{Name: "="},
			key.Filter{Name: "-"},
			key.Filter{Name: "_"},
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
			p := v.pointerPos
			if p == (f32.Point{}) {
				p = center
			}

			switch e.Name {
			case key.NameEscape:
				if v.OnClose != nil {
					v.OnClose()
				}
			case "0":
				v.Fit(size.X, size.Y)
			case "1":
				v.Set100(size.X, size.Y)
			case "W", "w":
				v.FitWidth(size.X, size.Y)
			case "H", "h":
				v.FitHeight(size.X, size.Y)
			case "+", "=":
				v.ZoomBy(1.2, p)
			case "-", "_":
				v.ZoomBy(1.0/1.2, p)
			case "N", "n":
				v.ToggleSequenceNumbers()
			case "[":
				v.PrevDiagram()
			case "]":
				v.NextDiagram()
			case "E", "e":
				if v.OnEdit != nil {
					v.OnEdit(v.CurIndex)
				}
			}
		}
	}
}

func (v *Viewer) drawDiagram(gtx layout.Context, th *material.Theme, seqTh seq.Theme, size image.Point) {
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
}

// drawStickyActors draws top actors pinned to the top of the viewer when
// scrolling tall diagrams.
func (v *Viewer) drawStickyActors(gtx layout.Context, th *material.Theme, seqTh seq.Theme, oTheme *Theme, size image.Point) {
	if v.Diagram == nil || len(v.Diagram.Actors) == 0 {
		return
	}

	// Find the top actor row (actors with Y <= actorHeight + margin)
	var topActors []layoutPkg.Actor
	var maxActorH float32
	for _, a := range v.Diagram.Actors {
		if a.Rect.Y < 80 {
			topActors = append(topActors, a)
			if a.Rect.H > maxActorH {
				maxActorH = a.Rect.H
			}
		}
	}
	if len(topActors) == 0 {
		return
	}

	// Top actors position in screen coordinates
	topY := v.Pan.Y + topActors[0].Rect.Y*v.Zoom
	headerH := (maxActorH + 20) * v.Zoom
	if headerH < 44 {
		headerH = 44
	}

	// Only show sticky header if the top actors are scrolled above the viewport
	if topY >= 0 {
		return
	}

	// Draw sticky header background strip
	hdrRect := clip.Rect(image.Rectangle{Max: image.Pt(size.X, int(headerH))})
	hdrClip := hdrRect.Push(gtx.Ops)
	paint.FillShape(gtx.Ops, withAlpha(oTheme.Background, 235), hdrRect.Op())

	// Bottom border for sticky header
	sepY := headerH - 1
	var sep clip.Path
	sep.Begin(gtx.Ops)
	sep.MoveTo(f32.Pt(0, sepY))
	sep.LineTo(f32.Pt(float32(size.X), sepY))
	paint.FillShape(gtx.Ops, withAlpha(oTheme.Foreground, 40), clip.Stroke{Path: sep.End(), Width: 1}.Op())

	// Draw pinned actors: translate X with Pan.X, keep Y fixed at pad
	pinnedY := float32(10.0)
	actorTrans := op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(v.Zoom, v.Zoom)).Offset(f32.Pt(v.Pan.X, pinnedY))).Push(gtx.Ops)

	diagGtx := gtx
	diagGtx.Metric = unit.Metric{PxPerDp: 1.0, PxPerSp: 1.0}

	subDiag := &layoutPkg.Diagram{
		Actors:  topActors,
		ViewBox: v.Diagram.ViewBox,
	}
	draw.Draw(diagGtx, th, seqTh, subDiag)

	actorTrans.Pop()
	hdrClip.Pop()
}
