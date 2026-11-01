package app

import (
	"image"

	"dyamark/palette"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Prompt is a question over the window, answered with a key or a click.
// Nothing else takes input while it's up.
type Prompt struct {
	Message string
	Choices []*Choice
	// conflict marks the question about the file changing on disk, which
	// goes away by itself when that's settled another way.
	conflict bool
}

// Choice is one answer to a Prompt.
type Choice struct {
	// Key is the key name that picks it, as Gio names keys.
	Key   key.Name
	Label string
	Do    func()

	btn widget.Clickable
}

// closePrompt takes the prompt down. A change on disk still waiting for an
// answer asks again.
func (a *App) closePrompt() {
	a.prompt = nil
	if a.conflict != nil {
		a.prompt = a.conflictPrompt()
		return
	}
	a.focusEditor = a.editing()
}

// layoutPrompt handles the prompt's keys and clicks and draws it over the
// window.
func (a *App) layoutPrompt(gtx layout.Context, theme *palette.Theme) {
	p := a.prompt
	var chosen *Choice
	filters := make([]key.Filter, len(p.Choices))
	for i, c := range p.Choices {
		filters[i] = key.Filter{Name: c.Key, Optional: key.ModShift}
	}
	for _, f := range filters {
		for {
			ev, ok := gtx.Event(f)
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press && chosen == nil {
				for _, c := range p.Choices {
					if c.Key == e.Name {
						chosen = c
					}
				}
			}
		}
	}
	for _, c := range p.Choices {
		if c.btn.Clicked(gtx) && chosen == nil {
			chosen = c
		}
	}

	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, palette.WithAlpha(theme.Background, 180), clip.Rect(image.Rectangle{Max: size}).Op())

	pad := gtx.Dp(20)
	w := min(gtx.Dp(520), size.X-gtx.Dp(32))
	inner := gtx
	inner.Constraints = layout.Constraints{Min: image.Pt(w-2*pad, 0), Max: image.Pt(w-2*pad, size.Y)}
	rec := op.Record(gtx.Ops)
	dims := layout.Flex{Axis: layout.Vertical}.Layout(inner,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body1(a.mTheme, p.Message)
			lbl.Color = theme.Foreground
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, 2*len(p.Choices))
			for i, c := range p.Choices {
				if i > 0 {
					children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(a.mTheme, &c.btn, keyLabel(c.Key)+"  "+c.Label)
					btn.Background = theme.Background
					btn.Color = theme.Foreground
					if i == 0 {
						btn.Background = theme.Accent
						btn.Color = theme.Background
					}
					btn.CornerRadius = unit.Dp(float32(theme.Rounding))
					btn.TextSize = unit.Sp(14)
					return btn.Layout(gtx)
				}))
			}
			return layout.Flex{Spacing: layout.SpaceStart}.Layout(gtx, children...)
		}),
	)
	body := rec.Stop()

	h := dims.Size.Y + 2*pad
	card := image.Rectangle{Min: image.Pt((size.X-w)/2, (size.Y-h)/2)}
	card.Max = card.Min.Add(image.Pt(w, h))
	r := gtx.Dp(unit.Dp(float32(theme.Rounding)))
	paint.FillShape(gtx.Ops, theme.LighterBackground, clip.UniformRRect(card, r).Op(gtx.Ops))
	off := op.Offset(card.Min.Add(image.Pt(pad, pad))).Push(gtx.Ops)
	body.Add(gtx.Ops)
	off.Pop()

	if chosen != nil {
		chosen.Do()
		gtx.Execute(op.InvalidateCmd{})
	}
}

// keyLabel is how a key is written on a button.
func keyLabel(name key.Name) string {
	if name == key.NameEscape {
		return "Esc"
	}
	return string(name)
}

// layoutNotice draws the notice line at the bottom of the window.
func (a *App) layoutNotice(gtx layout.Context, theme *palette.Theme) {
	size := gtx.Constraints.Max
	pad := gtx.Dp(8)
	inner := gtx
	inner.Constraints = layout.Constraints{Max: image.Pt(size.X-4*pad, size.Y)}
	rec := op.Record(gtx.Ops)
	lbl := material.Body2(a.mTheme, a.notice)
	lbl.Color = theme.Foreground
	lbl.MaxLines = 3
	dims := lbl.Layout(inner)
	text := rec.Stop()

	bar := image.Rectangle{Min: image.Pt(pad, size.Y-dims.Size.Y-3*pad), Max: image.Pt(size.X-pad, size.Y-pad)}
	r := gtx.Dp(unit.Dp(float32(theme.Rounding)))
	paint.FillShape(gtx.Ops, theme.LighterBackground, clip.UniformRRect(bar, r).Op(gtx.Ops))
	off := op.Offset(bar.Min.Add(image.Pt(pad, pad))).Push(gtx.Ops)
	text.Add(gtx.Ops)
	off.Pop()
}
