package app

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"time"

	"dyamark/palette"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// The toolbar is a faint row along the bottom of the window, with no band
// of its own: buttons on the left with the file's state after them, the
// caret's place and the word count on the right.

const (
	// toolbarHeight is the toolbar's height, in dp.
	toolbarHeight = 32
	// toolSize is a button's width and height, and iconSize its icon's,
	// in dp.
	toolSize = 28
	iconSize = 16
	// tooltipDelay is how long the pointer rests on a button before its
	// tooltip shows.
	tooltipDelay = 500 * time.Millisecond
)

// tool is a toolbar button.
type tool struct {
	// name and keys are what the tooltip says: Save · Ctrl+S.
	name, keys string
	// icon draws the icon's strokes into a 16×16 box.
	icon func(p *iconPath)
	do   func(gtx layout.Context)
	// on reports whether what the button turns on is on.
	on func() bool
	// gap is set on the first button of a group.
	gap bool

	click gesture.Click
	// rect is where the button is in the toolbar, as last drawn.
	rect image.Rectangle
	// hoverSince is when the pointer came to rest on the button; hush is
	// set once it's clicked, until the pointer leaves.
	hoverSince time.Time
	hush       bool
}

// iconPath draws an icon's lines in its 16×16 box, scaled to the screen.
type iconPath struct {
	clip.Path
	scale float32
}

func (p *iconPath) pt(x, y float32) f32.Point { return f32.Pt(x*p.scale, y*p.scale) }

// line draws a line through the points, given as x, y pairs; closed
// joins the last to the first.
func (p *iconPath) line(closed bool, xy ...float32) {
	p.MoveTo(p.pt(xy[0], xy[1]))
	for i := 2; i+1 < len(xy); i += 2 {
		p.LineTo(p.pt(xy[i], xy[i+1]))
	}
	if closed {
		p.Close()
	}
}

// circle draws a circle around x, y.
func (p *iconPath) circle(x, y, r float32) {
	p.MoveTo(p.pt(x+r, y))
	f := p.pt(-r, 0)
	p.Arc(f, f, 2*math.Pi)
}

// tools returns the toolbar's buttons, made the first time.
func (a *App) tools() []*tool {
	if a.toolList != nil {
		return a.toolList
	}
	a.toolList = []*tool{
		{name: "Save", keys: "Ctrl+S", do: func(layout.Context) { a.save() },
			icon: func(p *iconPath) {
				// A floppy disk: its outline, shutter and label.
				p.line(true, 2.5, 2.5, 10.5, 2.5, 13.5, 5.5, 13.5, 13.5, 2.5, 13.5)
				p.line(false, 5.5, 2.5, 5.5, 6, 10, 6, 10, 2.5)
				p.line(false, 4.5, 13.5, 4.5, 9.5, 11.5, 9.5, 11.5, 13.5)
			}},
		{name: "Open", keys: "Ctrl+O", do: func(layout.Context) { a.whenSaved(a.openDialog) },
			icon: func(p *iconPath) {
				p.line(true, 2.5, 13, 2.5, 3.5, 6.5, 3.5, 8.5, 5.5, 13.5, 5.5, 13.5, 13)
			}},
		{name: "Source", keys: "Ctrl+E", gap: true, do: func(layout.Context) { a.toggleSource() },
			on: func() bool { return a.mode == ModeEdit },
			icon: func(p *iconPath) {
				p.line(false, 6, 4, 2, 8, 6, 12)
				p.line(false, 10, 4, 14, 8, 10, 12)
			}},
		{name: "Source and view", keys: "Ctrl+\\", do: func(layout.Context) { a.toggleSplit() },
			on: func() bool { return a.mode == ModeSplit },
			icon: func(p *iconPath) {
				p.line(true, 2, 3, 14, 3, 14, 13, 2, 13)
				p.line(false, 8, 3, 8, 13)
			}},
		{name: "Find", keys: "Ctrl+F", gap: true,
			do: func(gtx layout.Context) {
				if a.find.open {
					a.closeFind(gtx)
				} else {
					a.openFind()
				}
			},
			on: func() bool { return a.find.open },
			icon: func(p *iconPath) {
				p.circle(7, 7, 4.5)
				p.line(false, 10.3, 10.3, 14, 14)
			}},
		{name: "Keys", keys: "?", do: func(layout.Context) { a.help = true },
			icon: func(p *iconPath) {
				// A keyboard: three keys and the space bar.
				p.line(true, 1.5, 4, 14.5, 4, 14.5, 12, 1.5, 12)
				p.line(false, 4, 7, 5, 7)
				p.line(false, 7.5, 7, 8.5, 7)
				p.line(false, 11, 7, 12, 7)
				p.line(false, 5, 9.5, 11, 9.5)
			}},
	}
	return a.toolList
}

// statusText returns the toolbar's message about the file, and whether
// it's about a problem: a notice for a few seconds, then whether the file
// is saved.
func (a *App) statusText() (msg string, problem bool) {
	name := "Untitled"
	if a.opts.FilePath != "" {
		name = filepath.Base(a.opts.FilePath)
	}
	switch {
	case a.notice != "":
		return a.notice, true
	case a.saveErr != "":
		return fmt.Sprintf("Couldn't save %s: %s", name, a.saveErr), true
	case a.dirty && a.opts.FilePath == "":
		return "Unsaved draft", false
	case a.dirty:
		return "Unsaved changes to " + name, false
	case a.opts.FilePath == "":
		return name, false
	}
	return a.fileEvent + " " + name, false
}

// whereText returns the toolbar's right side: the caret's line and column
// while editing, and the word count.
func (a *App) whereText() string {
	words := fmt.Sprintf("%d words", a.doc.Words)
	if a.doc.Words == 1 {
		words = "1 word"
	}
	if !a.editing() {
		return words
	}
	line, col := a.editor.CaretPos()
	return fmt.Sprintf("Ln %d, Col %d · %s", line+1, col+1, words)
}

// layoutToolbar handles the toolbar's clicks and draws it filling gtx's
// area, with a button's tooltip above it.
func (a *App) layoutToolbar(gtx layout.Context, theme *palette.Theme) {
	size := gtx.Constraints.Max
	muted := palette.WithAlpha(theme.Foreground, 180)
	tools := a.tools()
	for _, t := range tools {
		for {
			e, ok := t.click.Update(gtx.Source)
			if !ok {
				break
			}
			if e.Kind == gesture.KindClick {
				t.hush = true
				t.do(gtx)
				a.window.Invalidate()
			}
		}
	}

	bs := gtx.Dp(toolSize)
	is := gtx.Dp(iconSize)
	x := gtx.Dp(8)
	a.tip = nil
	for _, t := range tools {
		if t.gap {
			x += gtx.Dp(8)
		}
		t.rect = image.Rectangle{Min: image.Pt(x, (size.Y-bs)/2)}
		t.rect.Max = t.rect.Min.Add(image.Pt(bs, bs))
		x += bs

		on := t.on != nil && t.on()
		hovered := t.click.Hovered() && gtx.Enabled()
		r := gtx.Dp(6)
		switch {
		case on:
			paint.FillShape(gtx.Ops, palette.WithAlpha(theme.Accent, 40), clip.UniformRRect(t.rect, r).Op(gtx.Ops))
		case hovered:
			paint.FillShape(gtx.Ops, palette.WithAlpha(theme.Foreground, 24), clip.UniformRRect(t.rect, r).Op(gtx.Ops))
		}
		c := palette.WithAlpha(theme.Foreground, 150)
		switch {
		case on:
			c = theme.Accent
		case hovered:
			c = theme.Foreground
		}
		drawIcon(gtx, t.icon, t.rect.Min.Add(image.Pt((bs-is)/2, (bs-is)/2)), is, c)

		area := clip.Rect(t.rect).Push(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		t.click.Add(gtx.Ops)
		area.Pop()

		switch {
		case !hovered:
			t.hoverSince, t.hush = time.Time{}, false
		case t.hoverSince.IsZero():
			t.hoverSince = gtx.Now
		}
		if hovered && !t.hush {
			if at := t.hoverSince.Add(tooltipDelay); gtx.Now.Before(at) {
				gtx.Execute(op.InvalidateCmd{At: at})
			} else {
				a.tip = t
			}
		}
	}

	pad := gtx.Dp(12)
	// label records s in one line at most maxW wide.
	label := func(s string, c color.NRGBA, maxW int) (op.CallOp, image.Point) {
		gtx := gtx
		gtx.Constraints = layout.Constraints{Max: image.Pt(max(maxW, 0), size.Y)}
		rec := op.Record(gtx.Ops)
		l := material.Caption(a.mTheme, s)
		l.Color = c
		l.MaxLines = 1
		dims := l.Layout(gtx)
		return rec.Stop(), dims.Size
	}
	place := func(call op.CallOp, x int, sz image.Point) {
		off := op.Offset(image.Pt(x, (size.Y-sz.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		off.Pop()
	}
	call, right := label(a.whereText(), muted, size.X/3)
	place(call, size.X-pad-right.X, right)
	msg, problem := a.statusText()
	c := muted
	if problem {
		c = theme.Error()
	}
	x += pad
	call, sz := label(msg, c, size.X-x-2*pad-right.X)
	place(call, x, sz)

	if a.tip != nil {
		a.drawTooltip(gtx, theme, a.tip)
	}
}

// drawIcon strokes icon in a box size px wide at at.
func drawIcon(gtx layout.Context, icon func(*iconPath), at image.Point, size int, c color.NRGBA) {
	off := op.Offset(at).Push(gtx.Ops)
	p := iconPath{scale: float32(size) / iconSize}
	p.Begin(gtx.Ops)
	icon(&p)
	paint.FillShape(gtx.Ops, c, clip.Stroke{Path: p.End(), Width: 1.4 * p.scale}.Op())
	off.Pop()
}

// drawTooltip says what t does and its key, just above it.
func (a *App) drawTooltip(gtx layout.Context, theme *palette.Theme, t *tool) {
	text := t.name
	if t.keys != "" {
		text += " · " + t.keys
	}
	pad := gtx.Dp(6)
	inner := gtx
	inner.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(400), gtx.Dp(100))}
	rec := op.Record(gtx.Ops)
	l := material.Caption(a.mTheme, text)
	l.Color = theme.Foreground
	l.MaxLines = 1
	dims := l.Layout(inner)
	call := rec.Stop()

	w, h := dims.Size.X+2*pad, dims.Size.Y+2*pad
	x := min(t.rect.Min.X, max(gtx.Constraints.Max.X-w-pad, 0))
	box := image.Rect(x, t.rect.Min.Y-h-gtx.Dp(4), x+w, t.rect.Min.Y-gtx.Dp(4))
	r := gtx.Dp(unit.Dp(min(float32(theme.Rounding), 6)))
	paint.FillShape(gtx.Ops, theme.LighterBackground, clip.UniformRRect(box, r).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, palette.WithAlpha(theme.Foreground, 40),
		clip.Stroke{Path: clip.UniformRRect(box, r).Path(gtx.Ops), Width: 1}.Op())
	off := op.Offset(box.Min.Add(image.Pt(pad, pad))).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
}
