package app

import (
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"
)

// ? shows the key list in the view; in the editor it's typed, and F1
// shows the list instead. Esc closes it, and the keys under it don't work
// while it's up.
func TestHelpKeys(t *testing.T) {
	a, _ := openApp(t, "a.md", "# a\n")
	r, frame := appFrames(t, a)
	now := time.Now()
	frame(now)
	press := func(name key.Name, mods key.Modifiers) {
		r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
		frame(now)
		frame(now)
	}
	press("?", key.ModShift)
	if !a.help {
		t.Fatal("? didn't show the key list")
	}
	press("E", key.ModCtrl)
	if a.mode != ModeInline {
		t.Error("Ctrl+E worked under the key list")
	}
	press(key.NameEscape, 0)
	if a.help {
		t.Fatal("Esc didn't close the key list")
	}

	a.setMode(ModeEdit)
	frame(now)
	frame(now)
	r.Queue(key.EditEvent{Text: "?"}, key.Event{Name: "?", Modifiers: key.ModShift, State: key.Press})
	frame(now)
	frame(now)
	if a.help || !strings.HasPrefix(a.editor.Text(), "?") {
		t.Errorf("? in the editor: help %v, text %q", a.help, a.editor.Text())
	}
	press(key.NameF1, 0)
	if !a.help {
		t.Error("F1 in the editor didn't show the key list")
	}
}
