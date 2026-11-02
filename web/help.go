package web

import (
	"image"

	"dyamark/palette"

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

type helpGroup struct {
	title string
	keys  [][2]string
}

var (
	helpReading = helpGroup{"Reading", [][2]string{
		{"↑ ↓  PgUp PgDn  Space  Home End", "Scroll"},
		{"Tab  Shift+Tab", "Move between diagrams"},
		{"Enter or click", "Open the diagram"},
		{"Click a link", "Open it"},
		{"Ctrl+=  Ctrl+-  Ctrl+0", "Text size"},
	}}
	helpViewer = helpGroup{"Diagram viewer", [][2]string{
		{"+  -", "Zoom in, out"},
		{"0  1", "Fit, 100 %"},
		{"W  H", "Fit width, fit height"},
		{"[  ]", "Previous, next diagram"},
		{"N", "Sequence numbers"},
		{"E", "Edit this diagram in the IDE"},
		{"Click a participant", "Focus participant"},
		{"Esc", "Close"},
	}}
	helpEverywhere = helpGroup{"Everywhere", [][2]string{
		{"?  F1", "These keys"},
	}}
)

func (p *Preview) helpGroups() []helpGroup {
	if p.viewing {
		return []helpGroup{helpViewer, helpReading, helpEverywhere}
	}
	return []helpGroup{helpReading, helpViewer, helpEverywhere}
}

func (p *Preview) layoutHelp(gtx layout.Context) {
	filters := []event.Filter{
		key.Filter{Name: key.NameF1},
		key.Filter{Name: "?", Optional: key.ModShift},
		key.Filter{Name: key.NameEscape},
		pointer.Filter{Target: &p.help, Kinds: pointer.Press},
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case key.Event:
			if e.State == key.Press {
				p.help = false
			}
		case pointer.Event:
			p.help = false
		}
	}
	if !p.help {
		gtx.Execute(op.InvalidateCmd{})
		return
	}

	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, palette.WithAlpha(p.theme.Background, 200), clip.Rect(image.Rectangle{Max: size}).Op())
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &p.help)
	area.Pop()

	pad := gtx.Dp(20)
	w := min(gtx.Dp(620), size.X-gtx.Dp(32))
	keyW := min(gtx.Dp(240), (w-2*pad)/2)
	descW := w - 2*pad - keyW
	label := func(text string, sz unit.Sp) material.LabelStyle {
		l := material.Label(p.th, sz, text)
		l.Color = p.theme.Foreground
		return l
	}
	place := func(l material.LabelStyle, x, y, maxW int) int {
		g := gtx
		g.Constraints = layout.Constraints{Max: image.Pt(max(maxW, 0), size.Y)}
		off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		h := l.Layout(g).Size.Y
		off.Pop()
		return h
	}

	rec := op.Record(gtx.Ops)
	y := 0
	for i, g := range p.helpGroups() {
		if i > 0 {
			y += gtx.Dp(14)
		}
		title := label(g.title, 15)
		title.Font.Weight = font.Bold
		y += place(title, 0, y, w-2*pad) + gtx.Dp(6)
		for _, k := range g.keys {
			kl := label(k[0], 13)
			kl.Color = p.theme.Accent
			kl.Font.Weight = font.Bold
			kh := place(kl, 0, y, keyW-gtx.Dp(8))
			dh := place(label(k[1], 13), keyW, y, descW)
			y += max(kh, dh) + gtx.Dp(4)
		}
	}
	body := rec.Stop()

	h := y + 2*pad
	card := image.Rectangle{Min: image.Pt((size.X-w)/2, max((size.Y-h)/2, 0))}
	card.Max = card.Min.Add(image.Pt(w, h))
	r := gtx.Dp(unit.Dp(float32(p.theme.Rounding)))
	paint.FillShape(gtx.Ops, p.theme.LighterBackground, clip.UniformRRect(card, r).Op(gtx.Ops))
	off := op.Offset(card.Min.Add(image.Pt(pad, pad))).Push(gtx.Ops)
	body.Add(gtx.Ops)
	off.Pop()
}
