package app

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"
)

// statusHeight is the status line's height, in dp.
const statusHeight = 26

// showStatus reports whether the status line is shown: always while
// editing, and in the view while there are unsaved edits or a save failed.
func (a *App) showStatus() bool {
	switch a.mode {
	case ModeEdit, ModeSplit:
		return true
	case ModeInline:
		return a.dirty || a.saveErr != ""
	}
	return false
}

// statusText returns the status line's parts: the file's name, whether
// it's saved, and the mode with the caret's line and column.
func (a *App) statusText() (file, state, where string) {
	file = "Untitled"
	if a.opts.FilePath != "" {
		file = filepath.Base(a.opts.FilePath)
	}
	switch {
	case a.saveErr != "":
		state = "Couldn't save: " + a.saveErr
	case a.dirty:
		state = "Edited"
	case a.opts.FilePath != "":
		state = "Saved"
	}
	switch a.mode {
	case ModeEdit, ModeSplit:
		line, col := a.editor.CaretPos()
		mode := "Edit"
		if a.mode == ModeSplit {
			mode = "Split"
		}
		where = fmt.Sprintf("%s · Ln %d, Col %d", mode, line+1, col+1)
	default:
		where = "View"
	}
	return file, state, where
}

// layoutStatus draws the status line filling gtx's area.
func (a *App) layoutStatus(gtx layout.Context, theme *Theme) {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, theme.LighterBackground, clip.Rect(image.Rectangle{Max: size}).Op())
	file, state, where := a.statusText()
	pad := gtx.Dp(12)
	muted := withAlpha(theme.Foreground, 210)

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

	call, right := label(where, muted, size.X/2)
	place(call, size.X-pad-right.X, right)
	room := size.X - 3*pad - right.X
	call, name := label(file, theme.Foreground, room)
	place(call, pad, name)
	if state == "" {
		return
	}
	c := muted
	switch {
	case a.saveErr != "":
		c = errorColor(theme)
	case a.dirty:
		c = theme.Accent
	}
	call, sz := label(state, c, room-name.X-pad)
	place(call, pad+name.X+pad, sz)
}

// errorColor is the theme's red, or a plain red when it has none.
func errorColor(theme *Theme) color.NRGBA {
	if theme.Colors[1].A != 0 {
		return theme.Colors[1]
	}
	return color.NRGBA{R: 0xf8, G: 0x51, B: 0x49, A: 0xff}
}
