package app

import (
	"errors"
	"fmt"
	"image"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"dyamark/fonts"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const (
	// parseDelay is how long typing has to stop for before the view is
	// laid out again.
	parseDelay = 80 * time.Millisecond
	// autoSaveDelay is how long typing has to stop for before a document
	// with a file is saved.
	autoSaveDelay = time.Second
	// editorTextSize is the source text size, before Ctrl+= and Ctrl+-.
	editorTextSize = 14
)

// The editor holds the document: the view, the viewer and saving all work
// from its text.

// load makes data the document, as read from its file: nothing to save.
func (a *App) load(data []byte) {
	a.editor.SetText(string(data))
	a.saved = string(data)
	a.dirty = false
	a.saveErr = ""
	a.parseAt, a.saveAt = time.Time{}, time.Time{}
	a.dropConflict()
}

// edited notes a change to the editor's text: the view is laid out again
// once typing stops, and a document with a file saved a moment later.
func (a *App) edited(now time.Time) {
	text := a.editor.Text()
	a.dirty = text != a.saved
	a.updateTitle()
	if text == a.parsed {
		// An undo got back to what's shown.
		return
	}
	a.parseAt = now.Add(parseDelay)
	if a.dirty && a.opts.FilePath != "" {
		a.saveAt = now.Add(autoSaveDelay)
	}
}

// tick does the parse and the save that are due at now, and returns when
// the next thing is due, or the zero time.
func (a *App) tick(now time.Time) time.Time {
	if !a.parseAt.IsZero() && !now.Before(a.parseAt) {
		a.parseAt = time.Time{}
		a.reload([]byte(a.editor.Text()))
	}
	if !a.saveAt.IsZero() && !now.Before(a.saveAt) {
		a.saveAt = time.Time{}
		// Auto-save waits while the file changed on disk is unresolved.
		if a.dirty && a.opts.FilePath != "" && a.conflict == nil {
			a.saveTo(a.opts.FilePath)
		}
	}
	if !a.noticeUntil.IsZero() && !now.Before(a.noticeUntil) {
		a.notice, a.noticeUntil = "", time.Time{}
	}
	var next time.Time
	for _, t := range []time.Time{a.parseAt, a.saveAt, a.noticeUntil} {
		if !t.IsZero() && (next.IsZero() || t.Before(next)) {
			next = t
		}
	}
	return next
}

// flushParse lays out the text now if typing left a parse waiting.
func (a *App) flushParse() {
	if !a.parseAt.IsZero() {
		a.parseAt = time.Time{}
		a.reload([]byte(a.editor.Text()))
	}
}

// save saves the document to its file, or asks where when it has none.
func (a *App) save() {
	if a.opts.FilePath == "" {
		a.saveAs(nil)
		return
	}
	a.saveTo(a.opts.FilePath)
}

// saveTo writes the document to path, and reports whether it did.
func (a *App) saveTo(path string) bool {
	text := a.editor.Text()
	if err := SaveFile(path, []byte(text)); err != nil {
		a.notify(fmt.Sprintf("Couldn't save %s: %v", filepath.Base(path), err))
		// The path is in the status line already; keep only the reason.
		var pe *fs.PathError
		if errors.As(err, &pe) {
			err = pe.Err
		}
		a.saveErr = err.Error()
		return false
	}
	a.saved = text
	a.dirty = false
	a.saveErr = ""
	a.saveAt = time.Time{}
	// Saving over the file settles which version wins.
	a.dropConflict()
	a.updateTitle()
	return true
}

// saveAs asks where to save with the desktop's dialog, saves there and
// makes that the document's file; then runs after a save.
func (a *App) saveAs(then func()) {
	if a.dialogOpen {
		return
	}
	a.dialogOpen = true
	current := a.opts.FilePath
	go func() {
		path, err := PortalSave("Save As", current)
		a.post(func() {
			a.dialogOpen = false
			switch {
			case err != nil:
				a.notify(err.Error())
			case path == "":
				// Cancelled.
			case a.saveTo(path):
				a.setPath(path)
				if then != nil {
					then()
				}
			}
		})
	}()
}

// openDialog asks for a file with the desktop's dialog and opens it.
func (a *App) openDialog() {
	if a.dialogOpen {
		return
	}
	a.dialogOpen = true
	go func() {
		path, err := PortalOpen("Open")
		a.post(func() {
			a.dialogOpen = false
			switch {
			case err != nil:
				a.notify(err.Error())
			case path != "":
				a.open(path)
			}
		})
	}()
}

// open opens path, saying so in the window if it can't.
func (a *App) open(path string) {
	if err := a.OpenFile(path); err != nil {
		a.notify(fmt.Sprintf("Couldn't open %s: %v", filepath.Base(path), err))
	}
}

// setPath makes path the document's file: the title names it and its
// changes on disk are followed.
func (a *App) setPath(path string) {
	if a.fileWatcher != nil {
		a.fileWatcher.Close()
		a.fileWatcher = nil
	}
	a.opts.FilePath = path
	a.updateTitle()
	if path == "" {
		return
	}
	fw, err := a.watch(path)
	if err != nil {
		a.notify(fmt.Sprintf("Not following changes to %s: %v", filepath.Base(path), err))
	}
	a.fileWatcher = fw
}

// diskChanged takes the file's new text, after it changed on disk.
func (a *App) diskChanged(data []byte) {
	disk := string(data)
	text := a.editor.Text()
	switch {
	case disk == a.saved:
		// Our own save coming back, or a write that changed nothing.
		a.dropConflict()
	case text == a.saved:
		// No edits here to lose.
		a.editor.SetText(disk)
		a.saved = disk
		a.reload(data)
		a.dropConflict()
	case disk == text:
		// The file caught up with the edits.
		a.saved = disk
		a.dirty = false
		a.dropConflict()
	default:
		// Both changed: ask which to keep, and don't auto-save meanwhile.
		a.conflict = &disk
		a.prompt = a.conflictPrompt()
	}
	a.updateTitle()
}

// conflictPrompt asks which version to keep when the file changed on disk
// while the editor had edits.
func (a *App) conflictPrompt() *Prompt {
	return &Prompt{
		conflict: true,
		Message:  fmt.Sprintf("%s changed on disk while you were editing it.", filepath.Base(a.opts.FilePath)),
		Choices: []*Choice{
			{Key: "K", Label: "Keep mine", Do: func() {
				a.closePrompt()
				a.saveTo(a.opts.FilePath)
			}},
			{Key: "L", Label: "Load the file", Do: func() {
				disk := []byte(*a.conflict)
				a.closePrompt()
				a.load(disk)
				a.reload(disk)
				a.updateTitle()
			}},
		},
	}
}

// dropConflict forgets a change on disk that no longer needs a choice.
func (a *App) dropConflict() {
	if a.conflict == nil {
		return
	}
	a.conflict = nil
	if a.prompt != nil && a.prompt.conflict {
		a.prompt = nil
		a.focusEditor = a.editing()
	}
}

// whenSaved runs next once the edits are safe: saved to the document's
// file, or, for an untitled draft or one at odds with its file, after
// asking whether to save them.
func (a *App) whenSaved(next func()) {
	if !a.isDirty() {
		next()
		return
	}
	if a.opts.FilePath != "" && a.conflict == nil {
		if a.saveTo(a.opts.FilePath) {
			next()
		}
		return
	}
	name := "Untitled"
	if a.opts.FilePath != "" {
		name = filepath.Base(a.opts.FilePath)
	}
	a.prompt = &Prompt{
		Message: fmt.Sprintf("Save your changes to %s?", name),
		Choices: []*Choice{
			{Key: "S", Label: "Save", Do: func() {
				a.closePrompt()
				if a.opts.FilePath == "" {
					a.saveAs(next)
				} else if a.saveTo(a.opts.FilePath) {
					next()
				}
			}},
			{Key: "D", Label: "Don't save", Do: func() {
				a.prompt = nil
				next()
			}},
			{Key: key.NameEscape, Label: "Cancel", Do: a.closePrompt},
		},
	}
}

// isDirty reports whether the editor has text that isn't saved.
func (a *App) isDirty() bool {
	a.dirty = a.editor.Text() != a.saved
	return a.dirty
}

// quit closes the window without asking about unsaved work again.
func (a *App) quit() {
	a.quitting = true
	a.window.Perform(system.ActionClose)
}

// editing reports whether the source is on screen.
func (a *App) editing() bool {
	return a.mode == ModeEdit || a.mode == ModeSplit
}

// setMode switches between the view, the source and both.
func (a *App) setMode(m Mode) {
	if m == a.mode {
		return
	}
	// The view shows what was typed up to now.
	a.flushParse()
	a.mode = m
	a.focusEditor = a.editing()
	a.window.Invalidate()
}

// editAt shows the source with the caret at the start of line, 1-based.
func (a *App) editAt(line int) {
	if a.mode != ModeSplit {
		a.setMode(ModeEdit)
	}
	a.focusEditor = true
	text := a.editor.Text()
	offset := 0
	for l := 1; l < line; l++ {
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			break
		}
		offset += len([]rune(text[:i+1]))
		text = text[i+1:]
	}
	a.editor.SetCaret(offset, offset)
}

// updateTitle names the file in the window title, marked while it has
// unsaved edits.
func (a *App) updateTitle() {
	title := "dyamark"
	switch {
	case a.opts.FilePath != "":
		title = filepath.Base(a.opts.FilePath) + " — dyamark"
	case a.dirty:
		title = "Untitled — dyamark"
	}
	if a.dirty {
		title = "• " + title
	}
	if title != a.title {
		a.title = title
		a.window.Option(app.Title(title))
	}
}

// notify shows msg at the bottom of the window for a few seconds.
func (a *App) notify(msg string) {
	a.notice = msg
	a.noticeUntil = time.Now().Add(5 * time.Second)
	a.window.Invalidate()
}

// layoutEditor handles the editor's input and draws it filling gtx's
// area, its text in a column like the view's when there's room.
func (a *App) layoutEditor(gtx layout.Context, theme *Theme, column bool) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, theme.Background, clip.Rect(image.Rectangle{Max: size}).Op())

	if a.focusEditor {
		a.focusEditor = false
		gtx.Execute(key.FocusCmd{Tag: &a.editor})
	}
	// Tab indents rather than moving focus away.
	for {
		ev, ok := gtx.Event(key.Filter{Focus: &a.editor, Name: key.NameTab})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			a.editor.Insert("    ")
		}
	}
	for {
		ev, ok := a.editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.ChangeEvent); ok {
			a.edited(gtx.Now)
		}
	}

	pad := gtx.Dp(24)
	w := size.X - 2*pad
	if column {
		w = min(w, gtx.Dp(860))
	}
	w = max(w, gtx.Dp(100))
	x := (size.X - w) / 2
	inner := gtx
	inner.Constraints = layout.Exact(image.Pt(w, max(size.Y-2*pad, 0)))
	off := op.Offset(image.Pt(x, pad)).Push(gtx.Ops)
	scale := a.mdView.TextScale
	if scale <= 0 {
		scale = 1
	}
	ed := material.Editor(a.mTheme, &a.editor, "")
	ed.Font.Typeface = fonts.Mono
	ed.TextSize = unit.Sp(editorTextSize * scale)
	ed.Color = theme.Foreground
	ed.SelectionColor = withAlpha(theme.SelectionBg, 160)
	ed.Layout(inner)
	off.Pop()
	return layout.Dimensions{Size: size}
}
