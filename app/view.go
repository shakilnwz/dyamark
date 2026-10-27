package app

import (
	"image"

	"dyamark/seq"
	"dyamark/seq/draw"
	layoutPkg "dyamark/seq/layout"
	"dyamark/seq/parse"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// DefaultSampleDiagram is the sequence diagram shown when no file is passed.
const DefaultSampleDiagram = `sequenceDiagram
title Authentication Sequence Flow
participant Alice as Alice
participant API Gateway as API Gateway
participant Database as Database

Alice->>API Gateway: POST /api/session
activate API Gateway
opt Valid JWT Credentials
    API Gateway->>Database: SELECT user, hash FROM users
    activate Database
    Database-->>API Gateway: 1 row returned
    deactivate Database
    Note over API Gateway: Verify Argon2 hash<br/>& issue token
    API Gateway-->>Alice: 200 OK (JWT Bearer)
end
deactivate API Gateway`

// InlineDiagramView renders a sequence diagram inline, scaled to fit within
// the available column width. A click or Enter opens the fullscreen Viewer.
type InlineDiagramView struct {
	AST     *parse.Diagram
	Diagram *layoutPkg.Diagram
	Config  layoutPkg.Config

	OnClick func()
}

// SpikeView is an alias for InlineDiagramView for Phase 0 compatibility.
type SpikeView = InlineDiagramView

// NewInlineDiagramView creates an inline view for a parsed diagram.
func NewInlineDiagramView(ast *parse.Diagram) *InlineDiagramView {
	v := &InlineDiagramView{
		AST: ast,
	}
	if ast != nil {
		v.Config = layoutPkg.ResolveConfig(ast.Config)
		diag, err := layoutPkg.LayoutWithConfig(ast, v.Config, layoutPkg.Default())
		if err == nil {
			v.Diagram = diag
		}
	}
	return v
}

// Layout draws the sequence diagram inline. All coordinates scale cleanly.
func (v *InlineDiagramView) Layout(gtx layout.Context, mTheme *material.Theme, oTheme *Theme) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{}
	}

	for {
		ev, ok := gtx.Event(
			pointer.Filter{
				Target: v,
				Kinds:  pointer.Press,
			},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
		)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case pointer.Event:
			if e.Kind == pointer.Press && e.Buttons == pointer.ButtonPrimary {
				if v.OnClick != nil {
					v.OnClick()
				}
			}
		case key.Event:
			if e.State == key.Press {
				if v.OnClick != nil {
					v.OnClick()
				}
			}
		}
	}

	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	event.Op(gtx.Ops, v)
	area.Pop()

	if v.Diagram == nil {
		return layout.Dimensions{Size: size}
	}

	diagW := float32(v.Diagram.ViewBox.W)
	diagH := float32(v.Diagram.ViewBox.H)
	if diagW <= 0 || diagH <= 0 {
		return layout.Dimensions{Size: size}
	}

	availW := float32(size.X)
	scale := availW / diagW
	if scale > 1.0 {
		scale = 1.0
	}

	renderW := diagW * scale
	renderH := diagH * scale
	offsetX := (availW - renderW) / 2
	offsetY := float32(20.0)

	trans := op.Affine(f32.Affine2D{}.Offset(f32.Pt(offsetX, offsetY)).Scale(f32.Pt(0, 0), f32.Pt(scale, scale))).Push(gtx.Ops)
	defer trans.Pop()

	themeName := ""
	var themeVars map[string]string
	if v.AST != nil {
		themeName = v.AST.Config.Theme
		themeVars = v.AST.Config.ThemeVariables
	}
	diagTheme := seq.ResolveTheme(oTheme.Seq, themeName, themeVars)

	diagGtx := gtx
	diagGtx.Metric = unit.Metric{PxPerDp: 1.0, PxPerSp: 1.0}
	diagGtx.Constraints = layout.Exact(image.Pt(int(diagW), int(diagH)))

	draw.Draw(diagGtx, mTheme, diagTheme, v.Diagram)

	return layout.Dimensions{Size: image.Pt(int(availW), int(renderH+offsetY*2))}
}
