package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The status line names the file and says whether it's saved; a failed
// save stays there, through more typing, until a save works.
func TestStatusText(t *testing.T) {
	a, p := openApp(t, "plan.md", "# one\n")
	if a.showStatus() {
		t.Error("status line shown in the view with nothing to say")
	}
	a.setMode(ModeEdit)
	file, state, where := a.statusText()
	if file != "plan.md" || state != "Saved" || where != "Edit · Ln 1, Col 1" {
		t.Errorf("opened: %q %q %q", file, state, where)
	}

	now := time.Now()
	typeText(a, "# one\n\ntwo", now)
	if _, state, _ := a.statusText(); state != "Edited" {
		t.Errorf("after typing: %q", state)
	}

	if a.saveTo(filepath.Join(t.TempDir(), "gone", "plan.md")) {
		t.Fatal("saved into a folder that isn't there")
	}
	typeText(a, "# one\n\ntwo\nthree", now)
	if _, state, _ := a.statusText(); !strings.HasPrefix(state, "Couldn't save: ") || strings.Contains(state, "gone") {
		t.Errorf("after a failed save: %q", state)
	}
	a.setMode(ModeInline)
	if !a.showStatus() {
		t.Error("failed save not shown in the view")
	}

	if !a.saveTo(p) {
		t.Fatal("save failed")
	}
	if _, state, _ := a.statusText(); state != "Saved" {
		t.Errorf("after saving: %q", state)
	}
	if a.showStatus() {
		t.Error("status line still shown in the view once saved")
	}
	a.setMode(ModeSplit)
	if _, _, where := a.statusText(); !strings.HasPrefix(where, "Split · ") {
		t.Errorf("split: %q", where)
	}
}

// In edit mode the status line follows the caret; a draft is Untitled.
func TestStatusFollowsCaret(t *testing.T) {
	a, _ := openApp(t, "a.md", "one\ntwo\nthree\n")
	_, frame := appFrames(t, a)
	now := time.Now()
	a.setMode(ModeEdit)
	frame(now)
	a.editor.SetCaret(6, 6)
	frame(now)
	if _, _, where := a.statusText(); where != "Edit · Ln 2, Col 3" {
		t.Errorf("caret on two's o: %q", where)
	}

	a.opts.FilePath = ""
	a.dirty = true
	if file, state, _ := a.statusText(); file != "Untitled" || state != "Edited" {
		t.Errorf("untitled draft: %q %q", file, state)
	}
}
