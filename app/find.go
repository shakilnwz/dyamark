package app

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"unicode"

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

// findHeight is the find bar's height, in dp.
const findHeight = 36

// finder is the find bar's state.
type finder struct {
	open bool
	ed   widget.Editor
	// focus asks for the field to take the keyboard next frame.
	focus bool
	// matches holds where each match starts and ends in the text, in
	// runes, and cur which one is shown, or -1.
	matches [][2]int
	cur     int
	// at is where the last match shown starts: a new search starts there.
	at int
	// queryChanged is set when the search changed and the first match is
	// to be shown; textChanged when the text did, and only the matches
	// are found again.
	queryChanged, textChanged bool
}

// search returns where query is in text, ignoring case, in runes.
func search(text, query string) [][2]int {
	lower := func(s string) []rune {
		r := []rune(s)
		for i, c := range r {
			r[i] = unicode.ToLower(c)
		}
		return r
	}
	t, q := lower(text), lower(query)
	if len(q) == 0 {
		return nil
	}
	var out [][2]int
	for i := 0; i+len(q) <= len(t); {
		if string(t[i:i+len(q)]) == string(q) {
			out = append(out, [2]int{i, i + len(q)})
			i += len(q)
			continue
		}
		i++
	}
	return out
}

// openFind shows the find bar with its field ready for typing, filled
// with the selected text when that's one line.
func (a *App) openFind() {
	if a.mode == ModeViewer {
		return
	}
	f := &a.find
	// Enter steps through the matches.
	f.ed.SingleLine, f.ed.Submit = true, true
	if !f.open {
		f.open = true
		if sel := a.editor.SelectedText(); a.editing() && sel != "" && !strings.Contains(sel, "\n") {
			f.ed.SetText(sel)
		}
		f.at, f.cur = 0, -1
		if a.editing() {
			f.at, _ = a.editor.Selection()
		}
		f.queryChanged = true
	}
	f.focus = true
	f.ed.SetCaret(f.ed.Len(), 0)
	a.editor.PaintSelectionWhenUnfocused = true
	a.window.Invalidate()
}

// closeFind hides the find bar and gives the keyboard back.
func (a *App) closeFind(gtx layout.Context) {
	f := &a.find
	f.open = false
	f.matches, f.cur = nil, -1
	a.mdView.Mark = -1
	a.editor.PaintSelectionWhenUnfocused = false
	if a.editing() {
		a.focusEditor = true
	} else {
		gtx.Execute(key.FocusCmd{})
	}
}

// stepFind shows the next match, or the previous one when back is set.
func (a *App) stepFind(back bool) {
	f := &a.find
	n := len(f.matches)
	if n == 0 {
		return
	}
	switch {
	case f.cur < 0:
		f.cur = 0
	case back:
		f.cur = (f.cur - 1 + n) % n
	default:
		f.cur = (f.cur + 1) % n
	}
	a.showMatch()
}

// showMatch selects the current match in the source, and scrolls the view
// to the block it's in and marks that.
func (a *App) showMatch() {
	f := &a.find
	m := f.matches[f.cur]
	f.at = m[0]
	a.editor.SetCaret(m[0], m[1])
	text := []rune(a.editor.Text())
	line := 1 + strings.Count(string(text[:m[0]]), "\n")
	if b := a.mdView.Doc.BlockAt(line); b >= 0 {
		a.mdView.ShowBlock(b)
		a.mdView.Mark = b
	}
}

// updateFind finds the matches again after the search or the text
// changed, and shows the first match from where the last one was.
func (a *App) updateFind() {
	f := &a.find
	if !f.queryChanged && !f.textChanged {
		return
	}
	f.matches = search(a.editor.Text(), f.ed.Text())
	f.cur = -1
	if f.queryChanged {
		f.queryChanged, f.textChanged = false, false
		a.mdView.Mark = -1
		for i, m := range f.matches {
			if m[0] >= f.at {
				f.cur = i
				break
			}
		}
		if f.cur < 0 && len(f.matches) > 0 {
			f.cur = 0
		}
		if f.cur >= 0 {
			a.showMatch()
		}
		return
	}
	// The text changed under the matches: keep the one shown, if it's
	// still there.
	f.textChanged = false
	for i, m := range f.matches {
		if m[0] == f.at {
			f.cur = i
		}
	}
}

// findStatus says which match is shown, out of how many.
func (a *App) findStatus() string {
	f := &a.find
	switch {
	case f.ed.Len() == 0:
		return ""
	case len(f.matches) == 0:
		return "No matches"
	case f.cur < 0:
		return fmt.Sprintf("%d matches", len(f.matches))
	}
	return fmt.Sprintf("%d of %d", f.cur+1, len(f.matches))
}

// layoutFind handles the find bar's input and draws it filling gtx's
// area.
func (a *App) layoutFind(gtx layout.Context, theme *palette.Theme) {
	f := &a.find
	if f.focus {
		f.focus = false
		gtx.Execute(key.FocusCmd{Tag: &f.ed})
	}
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &f.ed, Name: key.NameEscape},
			key.Filter{Focus: &f.ed, Name: key.NameReturn, Required: key.ModShift},
			key.Filter{Focus: &f.ed, Name: key.NameEnter, Required: key.ModShift},
		)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		if e.Name == key.NameEscape {
			a.closeFind(gtx)
			return
		}
		a.stepFind(true)
	}
	for {
		ev, ok := f.ed.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			f.queryChanged = true
		case widget.SubmitEvent:
			a.updateFind()
			a.stepFind(false)
		}
	}
	a.updateFind()

	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, theme.LighterBackground, clip.Rect(image.Rectangle{Max: size}).Op())
	paint.FillShape(gtx.Ops, palette.WithAlpha(theme.Foreground, 40), clip.Rect(image.Rect(0, 0, size.X, 1)).Op())
	pad := gtx.Dp(12)
	muted := palette.WithAlpha(theme.Foreground, 210)
	label := func(s string, x int, c color.NRGBA) int {
		l := material.Caption(a.mTheme, s)
		l.Color = c
		l.MaxLines = 1
		g := gtx
		g.Constraints = layout.Constraints{Max: image.Pt(max(size.X-x, 0), size.Y)}
		rec := op.Record(gtx.Ops)
		dims := l.Layout(g)
		call := rec.Stop()
		off := op.Offset(image.Pt(x, (size.Y-dims.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		off.Pop()
		return dims.Size.X
	}

	x := pad
	x += label("Find", x, theme.Foreground) + pad

	// The field: a box with the search in it.
	fieldW := min(gtx.Dp(320), max(size.X-x-gtx.Dp(260), gtx.Dp(120)))
	inset := gtx.Dp(6)
	field := image.Rect(x, inset, x+fieldW, size.Y-inset)
	paint.FillShape(gtx.Ops, theme.Background, clip.UniformRRect(field, gtx.Dp(4)).Op(gtx.Ops))
	border := palette.WithAlpha(theme.Foreground, 60)
	if gtx.Focused(&f.ed) {
		border = theme.Accent
	}
	paint.FillShape(gtx.Ops, border, clip.Stroke{Path: clip.UniformRRect(field, gtx.Dp(4)).Path(gtx.Ops), Width: 1}.Op())
	ed := material.Editor(a.mTheme, &f.ed, "")
	ed.TextSize = unit.Sp(13)
	ed.Color = theme.Foreground
	ed.SelectionColor = palette.WithAlpha(theme.SelectionBg, 160)
	eg := gtx
	eg.Constraints = layout.Constraints{Max: image.Pt(fieldW-2*inset, field.Dy())}
	rec := op.Record(gtx.Ops)
	dims := ed.Layout(eg)
	call := rec.Stop()
	clipField := clip.Rect(field).Push(gtx.Ops)
	off := op.Offset(image.Pt(x+inset, (size.Y-dims.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	clipField.Pop()
	x += fieldW + pad

	c := muted
	if len(f.matches) == 0 && f.ed.Len() > 0 {
		c = theme.Error()
	}
	if s := a.findStatus(); s != "" {
		x += label(s, x, c) + 2*pad
	}
	label("Enter next · Shift+Enter previous · Esc close", x, palette.WithAlpha(theme.Foreground, 190))
}
