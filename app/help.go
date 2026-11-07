package app

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

// helpGroup is a set of keys that work in one place.
type helpGroup struct {
	title string
	// keys holds each key and what it does.
	keys [][2]string
}

var (
	helpEverywhere = helpGroup{"Everywhere", [][2]string{
		{"Ctrl+O", "Open"},
		{"Ctrl+F", "Find; Enter, Shift+Enter for the next, previous"},
		{"Ctrl+S  Ctrl+Shift+S", "Save, save as"},
		{"Ctrl+E", "Source or view"},
		{"Ctrl+\\", "Source and view side by side"},
		{"Ctrl+=  Ctrl+-  Ctrl+0", "Text size"},
		{"?  F1", "These keys"},
		{"Ctrl+Q", "Quit"},
	}}
	helpReading = helpGroup{"Reading", [][2]string{
		{"↑ ↓  PgUp PgDn  Space  Home End", "Scroll"},
		{"Tab  Shift+Tab", "Move between diagrams"},
		{"Enter or click", "Open the diagram, or edit a broken one"},
		{"R", "Render another Mermaid type with mmdc"},
		{"Click a link", "Open it"},
	}}
	helpViewer = helpGroup{"Diagram viewer", [][2]string{
		{"Scroll  Shift+Scroll", "Move up and down, sideways"},
		{"Ctrl+Scroll", "Zoom at the pointer"},
		{"+  -", "Zoom in, out"},
		{"0  1", "Fit, 100 %"},
		{"W  H", "Fit width, fit height"},
		{"[  ]", "Previous, next diagram"},
		{"N", "Sequence numbers"},
		{"E", "Edit this diagram's source"},
		{"Click a participant", "Show only what it takes part in"},
		{"Esc", "Close"},
	}}
)

// helpGroups returns the key groups, the ones for the current mode first.
func (a *App) helpGroups() []helpGroup {
	if a.mode == ModeViewer {
		return []helpGroup{helpViewer, helpEverywhere, helpReading}
	}
	return []helpGroup{helpReading, helpEverywhere, helpViewer}
}

// helpKeys returns the filters for the keys that show the key list: ? isn't
// one while the editor takes typing.
func (a *App) helpKeys(gtx layout.Context) []event.Filter {
	filters := []event.Filter{key.Filter{Name: key.NameF1}}
	if !gtx.Focused(&a.editor) && !gtx.Focused(&a.find.ed) {
		filters = append(filters, key.Filter{Name: "?", Optional: key.ModShift})
	}
	return filters
}

// layoutHelp draws the key list over the window; a key that shows it, Esc
// or a click closes it.
func (a *App) layoutHelp(gtx layout.Context, theme *palette.Theme) {
	filters := append(a.helpKeys(gtx), key.Filter{Name: key.NameEscape},
		pointer.Filter{Target: &a.help, Kinds: pointer.Press})
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case key.Event:
			if e.State == key.Press {
				a.help = false
			}
		case pointer.Event:
			a.help = false
		}
	}
	if !a.help {
		gtx.Execute(op.InvalidateCmd{})
		return
	}

	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, palette.WithAlpha(theme.Background, 200), clip.Rect(image.Rectangle{Max: size}).Op())
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &a.help)
	area.Pop()

	pad := gtx.Dp(20)
	w := min(gtx.Dp(620), size.X-gtx.Dp(32))
	keyW := min(gtx.Dp(240), (w-2*pad)/2)
	descW := w - 2*pad - keyW
	label := func(text string, size unit.Sp) material.LabelStyle {
		l := material.Label(a.mTheme, size, text)
		l.Color = theme.Foreground
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

	// The rows are recorded first: the card is as tall as they are.
	rec := op.Record(gtx.Ops)
	y := 0
	for i, g := range a.helpGroups() {
		if i > 0 {
			y += gtx.Dp(14)
		}
		title := label(g.title, 15)
		title.Font.Weight = font.Bold
		y += place(title, 0, y, w-2*pad) + gtx.Dp(6)
		for _, k := range g.keys {
			kl := label(k[0], 13)
			kl.Color = theme.Accent
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
	r := gtx.Dp(unit.Dp(float32(theme.Rounding)))
	paint.FillShape(gtx.Ops, theme.LighterBackground, clip.UniformRRect(card, r).Op(gtx.Ops))
	off := op.Offset(card.Min.Add(image.Pt(pad, pad))).Push(gtx.Ops)
	body.Add(gtx.Ops)
	off.Pop()
}
