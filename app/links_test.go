package app

import (
	"os"
	"path/filepath"
	"testing"
)

// A web link goes to the desktop, a Markdown file next to this one opens
// here, another file goes to the desktop by its full path, and a link
// within the page does nothing.
func TestOpenLink(t *testing.T) {
	a, p := openApp(t, "a.md", "# a\n")
	dir := filepath.Dir(p)
	other := filepath.Join(dir, "other.md")
	if err := os.WriteFile(other, []byte("# other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var launched []string
	old := launch
	launch = func(target string) error { launched = append(launched, target); return nil }
	t.Cleanup(func() { launch = old })

	a.openLink("https://x.org/a?b=1")
	a.openLink("#top")
	a.openLink("img%20one.png")
	a.openLink("other.md")
	want := []string{"https://x.org/a?b=1", filepath.Join(dir, "img one.png")}
	if len(launched) != len(want) || launched[0] != want[0] || launched[1] != want[1] {
		t.Errorf("launched %q, want %q", launched, want)
	}
	if a.opts.FilePath != other {
		t.Errorf("open file %q, want %q", a.opts.FilePath, other)
	}

	// An untitled draft has no folder to find a relative file in.
	launched = nil
	a.setPath("")
	a.openLink("img.png")
	if len(launched) != 0 {
		t.Errorf("launched %q from an untitled draft", launched)
	}
}
