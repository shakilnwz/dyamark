package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
)

// The toolbar says what last happened to the file; a failed save stays
// there, through more typing, until a save works, and a notice shows for
// a while over the rest.
func TestStatusText(t *testing.T) {
	a, p := openApp(t, "plan.md", "# one\n")
	check := func(when, want string, wantProblem bool) {
		t.Helper()
		msg, problem := a.statusText()
		if msg != want || problem != wantProblem {
			t.Errorf("%s: %q (problem %v), want %q (%v)", when, msg, problem, want, wantProblem)
		}
	}
	check("opened", "Opened plan.md", false)

	now := time.Now()
	typeText(a, "# one\n\ntwo", now)
	check("after typing", "Unsaved changes to plan.md", false)

	if a.saveTo(filepath.Join(t.TempDir(), "gone", "plan.md")) {
		t.Fatal("saved into a folder that isn't there")
	}
	typeText(a, "# one\n\ntwo\nthree", now)
	if msg, problem := a.statusText(); !strings.HasPrefix(msg, "Couldn't save plan.md: ") || strings.Contains(msg, "gone") || !problem {
		t.Errorf("after a failed save: %q (problem %v)", msg, problem)
	}

	if !a.saveTo(p) {
		t.Fatal("save failed")
	}
	check("after saving", "Saved plan.md", false)

	a.notify("Couldn't open x.md: gone")
	check("notice", "Couldn't open x.md: gone", true)
	a.tick(a.noticeUntil)
	check("notice over", "Saved plan.md", false)

	if err := os.WriteFile(p, []byte("# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.diskChanged([]byte("# changed\n"))
	check("changed on disk", "Reloaded plan.md", false)

	a.opts.FilePath = ""
	check("untitled", "Untitled", false)
	a.dirty = true
	check("untitled draft", "Unsaved draft", false)

	b, err := New(Options{FilePath: filepath.Join(t.TempDir(), "new.md")})
	if err != nil {
		t.Fatal(err)
	}
	defer b.fileWatcher.Close()
	if msg, _ := b.statusText(); msg != "New new.md" {
		t.Errorf("a file not made yet: %q", msg)
	}
}

// The right side counts the words, and while editing says where the
// caret is.
func TestWhereText(t *testing.T) {
	a, _ := openApp(t, "a.md", "one\ntwo **three**\n")
	_, frame := appFrames(t, a)
	now := time.Now()
	frame(now)
	if got := a.whereText(); got != "3 words" {
		t.Errorf("view: %q", got)
	}
	a.setMode(ModeEdit)
	frame(now)
	a.editor.SetCaret(6, 6)
	frame(now)
	if got := a.whereText(); got != "Ln 2, Col 3 · 3 words" {
		t.Errorf("caret on two's o: %q", got)
	}
	typeText(a, "one", now)
	a.tick(now.Add(parseDelay))
	a.setMode(ModeInline)
	if got := a.whereText(); got != "1 word" {
		t.Errorf("one word: %q", got)
	}
}

// toolAt returns the middle of the toolbar button named name, on screen,
// in the 1200×800 frames of appFrames.
func toolAt(t *testing.T, a *App, name string) f32.Point {
	t.Helper()
	for _, tl := range a.tools() {
		if tl.name == name {
			c := tl.rect.Min.Add(tl.rect.Max).Div(2)
			return f32.Pt(float32(c.X), float32(800-toolbarHeight+c.Y))
		}
	}
	t.Fatalf("no %s button", name)
	return f32.Point{}
}

// clickAt clicks the primary button at p.
func clickAt(r *input.Router, frame func(time.Time), now time.Time, p f32.Point) {
	r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p})
	frame(now)
	r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
	frame(now)
}

// The toolbar's buttons do what their keys do, and leave the keyboard
// with the editor.
func TestToolbarButtons(t *testing.T) {
	a, p := openApp(t, "a.md", "# one\n")
	r, frame := appFrames(t, a)
	now := time.Now()
	frame(now)
	press := func(name string) {
		t.Helper()
		clickAt(r, frame, now, toolAt(t, a, name))
		frame(now)
	}

	press("Source")
	if a.mode != ModeEdit || !r.Source().Focused(&a.editor) {
		t.Fatalf("Source: mode %v, editor focused %v", a.mode, r.Source().Focused(&a.editor))
	}
	typeText(a, "# two\n", now)
	press("Save")
	if got, _ := os.ReadFile(p); string(got) != "# two\n" || a.dirty {
		t.Errorf("Save: file %q, dirty %v", got, a.dirty)
	}
	if !r.Source().Focused(&a.editor) {
		t.Error("Save took the keyboard from the editor")
	}
	press("Source")
	if a.mode != ModeInline {
		t.Errorf("Source again: mode %v", a.mode)
	}
	press("Source and view")
	if a.mode != ModeSplit {
		t.Errorf("Source and view: mode %v", a.mode)
	}
	press("Source and view")
	if a.mode != ModeInline {
		t.Errorf("Source and view again: mode %v", a.mode)
	}
	press("Find")
	if !a.find.open {
		t.Error("Find didn't open the find bar")
	}
	press("Find")
	if a.find.open {
		t.Error("Find again didn't close the find bar")
	}
	press("Keys")
	if !a.help {
		t.Error("Keys didn't show the key list")
	}
}

// Resting the pointer on a button shows its tooltip after a moment; a
// click hides it until the pointer leaves.
func TestToolbarTooltip(t *testing.T) {
	a, _ := openApp(t, "a.md", "# one\n")
	r, frame := appFrames(t, a)
	now := time.Now()
	frame(now)
	at := toolAt(t, a, "Find")
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: at})
	frame(now)
	if a.tip != nil {
		t.Fatal("tooltip shown at once")
	}
	frame(now.Add(tooltipDelay))
	if a.tip == nil || a.tip.name != "Find" {
		t.Fatalf("tooltip %v after resting", a.tip)
	}

	clickAt(r, frame, now.Add(tooltipDelay), at)
	frame(now.Add(2 * tooltipDelay))
	if a.tip != nil {
		t.Error("tooltip shown after a click")
	}
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(600, 300)})
	frame(now.Add(3 * tooltipDelay))
	if a.tip != nil {
		t.Error("tooltip shown after the pointer left")
	}
}
