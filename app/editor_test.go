package app

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dyamark/fonts"
	"dyamark/markdown"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
)

// openApp writes src to name in a temporary directory and opens it.
func openApp(t *testing.T, name, src string) (*App, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{FilePath: p})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if a.fileWatcher != nil {
			a.fileWatcher.Close()
		}
	})
	return a, p
}

// typeText replaces the editor's text as typing would, at now.
func typeText(a *App, text string, now time.Time) {
	a.editor.SetText(text)
	a.edited(now)
}

// The view follows typing once it stops, and the file is saved a moment
// later.
func TestTypingReparsesThenSaves(t *testing.T) {
	a, p := openApp(t, "a.md", "# one\n")
	now := time.Now()
	typeText(a, "# one\n\ntwo\n", now)
	if !a.dirty || !strings.HasPrefix(a.title, "• a.md") {
		t.Errorf("not marked as edited: dirty %v, title %q", a.dirty, a.title)
	}

	a.tick(now.Add(parseDelay / 2))
	if len(a.doc.Blocks) != 1 {
		t.Error("parsed before typing stopped")
	}
	if next := a.tick(now.Add(parseDelay)); len(a.doc.Blocks) != 2 || next.IsZero() {
		t.Fatalf("not parsed after the delay: %d blocks, next %v", len(a.doc.Blocks), next)
	}
	if got, _ := os.ReadFile(p); string(got) != "# one\n" {
		t.Error("saved before typing stopped")
	}
	if next := a.tick(now.Add(autoSaveDelay)); !next.IsZero() {
		t.Errorf("still waiting on something: %v", next)
	}
	if got, _ := os.ReadFile(p); string(got) != "# one\n\ntwo\n" {
		t.Errorf("not auto-saved: %q", got)
	}
	if a.dirty || a.title != "a.md — dyamark" {
		t.Errorf("still marked as edited: dirty %v, title %q", a.dirty, a.title)
	}
}

// An untitled draft is never auto-saved.
func TestUntitledNotAutoSaved(t *testing.T) {
	a, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	typeText(a, "# draft\n", now)
	if next := a.tick(now.Add(time.Hour)); !next.IsZero() {
		t.Errorf("something still due: %v", next)
	}
	if !a.dirty || a.title != "• Untitled — dyamark" {
		t.Errorf("dirty %v, title %q", a.dirty, a.title)
	}
}

// A file named on the command line that isn't there yet opens empty, in
// the editor, and is made on the first save.
func TestNewFileArgument(t *testing.T) {
	p := filepath.Join(t.TempDir(), "new.md")
	a, err := New(Options{FilePath: p})
	if err != nil {
		t.Fatal(err)
	}
	defer a.fileWatcher.Close()
	if a.mode != ModeEdit || a.editor.Text() != "" {
		t.Fatalf("mode %v, text %q", a.mode, a.editor.Text())
	}
	typeText(a, "# hello\n", time.Now())
	a.save()
	if got, _ := os.ReadFile(p); string(got) != "# hello\n" {
		t.Errorf("saved %q", got)
	}
}

// A change on disk is taken in quietly when there are no edits, and our
// own save coming back changes nothing.
func TestDiskChangeWithoutEdits(t *testing.T) {
	a, _ := openApp(t, "a.md", "# one\n")
	a.diskChanged([]byte("# two\n\nbody\n"))
	if a.editor.Text() != "# two\n\nbody\n" || len(a.doc.Blocks) != 2 || a.prompt != nil {
		t.Fatalf("text %q, %d blocks, prompt %v", a.editor.Text(), len(a.doc.Blocks), a.prompt)
	}

	typeText(a, "# mine\n", time.Now())
	a.save()
	a.diskChanged([]byte("# mine\n"))
	if a.prompt != nil || a.dirty {
		t.Error("our own save was taken as someone else's change")
	}
}

// A change on disk while there are edits asks which to keep, and holds
// auto-save until it's answered.
func TestDiskChangeWithEdits(t *testing.T) {
	for _, keep := range []bool{true, false} {
		a, p := openApp(t, "a.md", "# one\n")
		now := time.Now()
		typeText(a, "# mine\n", now)
		os.WriteFile(p, []byte("# theirs\n"), 0o644)
		a.diskChanged([]byte("# theirs\n"))
		if a.prompt == nil || !a.prompt.conflict {
			t.Fatal("no question about the change on disk")
		}
		a.tick(now.Add(autoSaveDelay))
		if got, _ := os.ReadFile(p); string(got) != "# theirs\n" {
			t.Fatalf("auto-save overwrote the file before the answer: %q", got)
		}

		choice := map[bool]key.Name{true: "K", false: "L"}[keep]
		for _, c := range a.prompt.Choices {
			if c.Key == choice {
				c.Do()
			}
		}
		want := map[bool]string{true: "# mine\n", false: "# theirs\n"}[keep]
		got, _ := os.ReadFile(p)
		if string(got) != want || a.editor.Text() != want {
			t.Errorf("keep %v: file %q, editor %q, want %q", keep, got, a.editor.Text(), want)
		}
		if a.prompt != nil || a.conflict != nil || a.dirty {
			t.Errorf("keep %v: still unsettled", keep)
		}
	}
}

// Closing saves a file's edits; an untitled draft's ask first.
func TestClosingWithEdits(t *testing.T) {
	a, p := openApp(t, "a.md", "# one\n")
	typeText(a, "# two\n", time.Now())
	a.closing(nil)
	if got, _ := os.ReadFile(p); string(got) != "# two\n" || a.prompt != nil {
		t.Errorf("file %q, prompt %v", got, a.prompt)
	}

	draft, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	typeText(draft, "# draft\n", time.Now())
	draft.whenSaved(draft.quit)
	if draft.prompt == nil || draft.quitting {
		t.Fatal("an untitled draft closed without asking")
	}
	for _, c := range draft.prompt.Choices {
		if c.Key == key.NameEscape {
			c.Do()
		}
	}
	if draft.prompt != nil || draft.quitting {
		t.Error("cancel didn't keep the window")
	}
}

// E in the viewer shows the source with the caret on the diagram.
func TestViewerEditJumpsToDiagram(t *testing.T) {
	src := "# T\n\n```mermaid\nsequenceDiagram\nA->>B: one\n```\n\n```mermaid\nsequenceDiagram\nC->>D: two\n```\n"
	a, _ := openApp(t, "a.md", src)
	a.mdView.OnOpenViewer(a.doc.Sequences[1])
	a.viewer.OnEdit(a.viewer.CurIndex)
	if a.mode != ModeEdit {
		t.Fatalf("mode %v", a.mode)
	}
	want := len([]rune(src[:strings.Index(src, "sequenceDiagram\nC")]))
	if start, end := a.editor.Selection(); start != want || end != want {
		t.Errorf("caret at %d-%d, want %d, the second diagram's first line", start, end, want)
	}
	a.setMode(ModeInline)
	if a.backMode != ModeEdit {
		t.Errorf("viewer would go back to %v", a.backMode)
	}
}

// appFrames returns a router feeding a and a function that draws one
// frame of it at now.
func appFrames(t *testing.T, a *App) (*input.Router, func(now time.Time)) {
	t.Helper()
	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	a.mTheme.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	r := new(input.Router)
	ops := new(op.Ops)
	return r, func(now time.Time) {
		ops.Reset()
		gtx := layout.Context{
			Ops:         ops,
			Now:         now,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1200, 800)),
			Source:      r.Source(),
		}
		a.frame(gtx)
		r.Frame(ops)
	}
}

// In split view, letters typed into the editor go into the text, not to
// the view's shortcuts.
func TestSplitTypingStaysInEditor(t *testing.T) {
	a, _ := openApp(t, "a.md", "```mermaid\nflowchart TD\nA-->B\n```\n")
	rendered := false
	a.mdView.OnRenderMermaid = func(*markdown.MermaidOtherBlock) { rendered = true }
	r, frame := appFrames(t, a)
	now := time.Now()
	frame(now)

	r.Queue(key.Event{Name: "\\", Modifiers: key.ModCtrl, State: key.Press})
	frame(now)
	frame(now)
	if a.mode != ModeSplit || !r.Source().Focused(&a.editor) {
		t.Fatalf("mode %v, editor focused %v", a.mode, r.Source().Focused(&a.editor))
	}

	r.Queue(key.Event{Name: "R", State: key.Press}, key.EditEvent{Text: "r"})
	frame(now)
	frame(now)
	if rendered {
		t.Error("typing r rendered a diagram")
	}
	if !strings.HasPrefix(a.editor.Text(), "r") {
		t.Errorf("r not typed: %q", a.editor.Text())
	}
	frame(now.Add(parseDelay))
	if a.parsed != a.editor.Text() {
		t.Error("view not updated after typing stopped")
	}
}

// A prompt takes the keys: K answers it and nothing is typed.
func TestPromptTakesKeys(t *testing.T) {
	a, p := openApp(t, "a.md", "# one\n")
	r, frame := appFrames(t, a)
	now := time.Now()
	a.setMode(ModeEdit)
	frame(now)
	frame(now)
	typeText(a, "# mine\n", now)
	a.diskChanged([]byte("# theirs\n"))
	frame(now)

	r.Queue(key.Event{Name: "K", Modifiers: key.ModShift, State: key.Press}, key.EditEvent{Text: "K"})
	frame(now)
	frame(now)
	if a.prompt != nil {
		t.Fatal("K didn't answer the prompt")
	}
	if got, _ := os.ReadFile(p); string(got) != "# mine\n" || a.editor.Text() != "# mine\n" {
		t.Errorf("file %q, editor %q", got, a.editor.Text())
	}
}

// Back in the view after editing, the view's keys work again: the editor
// no longer takes them.
func TestViewKeysAfterEditing(t *testing.T) {
	var src strings.Builder
	for range 80 {
		src.WriteString("A paragraph of text.\n\n")
	}
	a, _ := openApp(t, "a.md", src.String())
	r, frame := appFrames(t, a)
	now := time.Now()
	a.setMode(ModeEdit)
	frame(now)
	frame(now)
	a.setMode(ModeInline)
	frame(now)
	r.Queue(key.Event{Name: key.NamePageDown, State: key.Press})
	frame(now)
	frame(now)
	if a.mdView.List.Position.First == 0 {
		t.Error("Page Down didn't scroll the view after editing")
	}
}

// In split view the view follows the caret, but only while the editor has
// the keyboard: after a click in the view, it stays where it's scrolled.
func TestSplitFollowsCaret(t *testing.T) {
	var src strings.Builder
	for range 80 {
		src.WriteString("A paragraph of text.\n\n")
	}
	a, _ := openApp(t, "a.md", src.String())
	r, frame := appFrames(t, a)
	now := time.Now()
	a.setMode(ModeSplit)
	frame(now)
	frame(now)
	end := a.editor.Len()
	a.editor.SetCaret(end, end)
	frame(now)
	frame(now)
	if p := a.mdView.List.Position; p.First+p.Count != len(a.doc.Blocks) {
		t.Fatalf("caret at the end, view shows blocks %d to %d", p.First, p.First+p.Count-1)
	}

	r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(900, 400)})
	frame(now)
	a.editor.SetCaret(0, 0)
	frame(now)
	frame(now)
	if a.mdView.List.Position.First == 0 {
		t.Error("view followed the caret after a click in it")
	}
}
