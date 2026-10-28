package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveFile(t *testing.T) {
	dir := t.TempDir()

	// A new file is made, with no temporary file left behind.
	p := filepath.Join(dir, "new.md")
	if err := SaveFile(p, []byte("# new\n")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p); string(got) != "# new\n" {
		t.Errorf("saved %q", got)
	}
	if _, err := os.Stat(p + ".dyamark-tmp"); !os.IsNotExist(err) {
		t.Error("temporary file left behind")
	}

	// An existing file keeps its mode.
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveFile(p, []byte("# again\n")); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(p); info.Mode().Perm() != 0o600 {
		t.Errorf("mode changed to %v", info.Mode().Perm())
	}

	// Saving through a symlink writes the file it points at.
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(p, link); err != nil {
		t.Fatal(err)
	}
	if err := SaveFile(link, []byte("# via link\n")); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Lstat(link); info.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink was replaced by a file")
	}
	if got, _ := os.ReadFile(p); string(got) != "# via link\n" {
		t.Errorf("target holds %q", got)
	}

	// A failed save leaves the old file alone.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	if err := SaveFile(p, []byte("# lost\n")); err == nil {
		t.Error("save into a read-only directory succeeded")
	}
	if got, _ := os.ReadFile(p); string(got) != "# via link\n" {
		t.Errorf("failed save changed the file to %q", got)
	}
}
