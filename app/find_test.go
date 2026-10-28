package app

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"
)

// Matches ignore case, don't overlap, and are counted in runes.
func TestSearch(t *testing.T) {
	for _, tt := range []struct {
		text, query string
		want        [][2]int
	}{
		{"Beta beta BETA", "beta", [][2]int{{0, 4}, {5, 9}, {10, 14}}},
		{"aaaa", "aa", [][2]int{{0, 2}, {2, 4}}},
		{"café Café", "CAFÉ", [][2]int{{0, 4}, {5, 9}}},
		{"x", "", nil},
		{"x", "y", nil},
	} {
		if got := search(tt.text, tt.query); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%q in %q: got %v, want %v", tt.query, tt.text, got, tt.want)
		}
	}
}

// Ctrl+F in the editor finds as you type and selects the match; Enter and
// Shift+Enter step through the matches; Esc closes the bar and gives the
// keyboard back to the editor.
func TestFindInEditor(t *testing.T) {
	a, _ := openApp(t, "a.md", "alpha\n\nBeta one\n\nbeta two\n")
	r, frame := appFrames(t, a)
	now := time.Now()
	a.setMode(ModeEdit)
	frame(now)
	frame(now)
	press := func(name key.Name, mods key.Modifiers) {
		r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
		frame(now)
		frame(now)
	}
	press("F", key.ModCtrl)
	if !a.find.open {
		t.Fatal("Ctrl+F didn't open the find bar")
	}
	r.Queue(key.EditEvent{Text: "beta"})
	frame(now)
	frame(now)
	selected := func() string { return a.editor.SelectedText() }
	if got := a.findStatus(); got != "1 of 2" || selected() != "Beta" {
		t.Fatalf("typed: %q, selected %q", got, selected())
	}
	if a.editor.Text() != "alpha\n\nBeta one\n\nbeta two\n" {
		t.Fatalf("typing went into the document: %q", a.editor.Text())
	}
	press(key.NameReturn, 0)
	if got := a.findStatus(); got != "2 of 2" || selected() != "beta" {
		t.Errorf("Enter: %q, selected %q", got, selected())
	}
	press(key.NameReturn, key.ModShift)
	if got := a.findStatus(); got != "1 of 2" {
		t.Errorf("Shift+Enter: %q", got)
	}
	press(key.NameEscape, 0)
	if a.find.open || a.mode != ModeEdit {
		t.Fatalf("Esc: open %v, mode %v", a.find.open, a.mode)
	}
	frame(now)
	r.Queue(key.EditEvent{Text: "Z"})
	frame(now)
	if !strings.Contains(a.editor.Text(), "Z") {
		t.Errorf("the editor didn't get the keyboard back: %q", a.editor.Text())
	}
}

// In the view a match scrolls its block into view and marks it; a space
// typed into the find bar doesn't scroll the view.
func TestFindInView(t *testing.T) {
	var src strings.Builder
	for i := range 80 {
		fmt.Fprintf(&src, "Paragraph %d.\n\n", i)
	}
	src.WriteString("The needle is here.\n")
	a, _ := openApp(t, "a.md", src.String())
	r, frame := appFrames(t, a)
	now := time.Now()
	frame(now)
	r.Queue(key.Event{Name: "F", Modifiers: key.ModCtrl, State: key.Press})
	frame(now)
	frame(now)
	r.Queue(key.EditEvent{Text: "needle"})
	frame(now)
	frame(now)
	frame(now)
	last := len(a.doc.Blocks) - 1
	if got := a.findStatus(); got != "1 of 1" || a.mdView.Mark != last {
		t.Fatalf("status %q, marked %d, want %d", got, a.mdView.Mark, last)
	}
	if p := a.mdView.List.Position; p.First+p.Count <= last {
		t.Errorf("view shows blocks %d to %d, not %d", p.First, p.First+p.Count-1, last)
	}

	pos := a.mdView.List.Position
	r.Queue(key.EditEvent{Range: key.Range{Start: 6, End: 6}, Text: " "}, key.Event{Name: key.NameSpace, State: key.Press})
	frame(now)
	frame(now)
	if a.mdView.List.Position.First != pos.First || a.find.ed.Text() != "needle " {
		t.Errorf("space: view at %d (was %d), search %q", a.mdView.List.Position.First, pos.First, a.find.ed.Text())
	}
}
