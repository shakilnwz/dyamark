package app

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultTheme(t *testing.T) {
	th := DefaultTheme(10)

	// Accent should match Omarchy fallback #FFD60A
	expectedAccent := hexColor("#FFD60A", 255)
	if th.Accent != expectedAccent {
		t.Errorf("expected accent %v, got %v", expectedAccent, th.Accent)
	}
	if th.Seq.ActorBorder.A == 0 {
		t.Error("expected non-zero alpha for actor border")
	}
	if th.Seq.Signal.A == 0 {
		t.Error("expected non-zero alpha for signal color")
	}
}

func TestZeroRoundingIsKept(t *testing.T) {
	th := DefaultTheme(0)
	if th.Rounding != 0 || th.Seq.Rounding != 0 {
		t.Errorf("rounding 0 should give square corners, got %d / %v", th.Rounding, th.Seq.Rounding)
	}
}

func TestHexColor(t *testing.T) {
	cases := []struct {
		input    string
		alpha    uint8
		expected color.NRGBA
	}{
		{"#fff", 255, color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
		{"#000000", 255, color.NRGBA{R: 0, G: 0, B: 0, A: 255}},
		{"#7186fd", 200, color.NRGBA{R: 0x71, G: 0x86, B: 0xfd, A: 200}},
		{"#12345678", 255, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0x78}},
		{"invalid", 255, color.NRGBA{R: 200, G: 200, B: 200, A: 255}},
	}

	for _, tc := range cases {
		got := hexColor(tc.input, tc.alpha)
		if got != tc.expected {
			t.Errorf("hexColor(%q, %d) = %v, expected %v", tc.input, tc.alpha, got, tc.expected)
		}
	}
}

const lightColors = `
accent = "#1e66f5"
foreground = "#4c4f69"
background = "#eff1f5"
color3 = "#df8e1d"
`

func writeColors(t *testing.T, dir, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "colors.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadTheme(t *testing.T) {
	path := writeColors(t, t.TempDir(), lightColors)
	th := LoadTheme(path, 4)

	if want := hexColor("#eff1f5", 255); th.Background != want {
		t.Errorf("background = %v, want %v", th.Background, want)
	}
	if want := hexColor("#1e66f5", 255); th.Seq.Signal != want {
		t.Errorf("signal = %v, want the accent %v", th.Seq.Signal, want)
	}
	if th.Seq.Rounding != 4 {
		t.Errorf("seq rounding = %v, want 4", th.Seq.Rounding)
	}
	// No lighter_background given: it is derived, not left as the dark default.
	if th.LighterBackground == DefaultTheme(4).LighterBackground {
		t.Error("lighter background should be derived from a light theme's background")
	}
}

func TestNotesFollowLightTheme(t *testing.T) {
	th := LoadTheme(writeColors(t, t.TempDir(), lightColors), 0)
	if luminance(th.Seq.NoteBkg) < 0.5 {
		t.Errorf("note background %v is dark on a light theme", th.Seq.NoteBkg)
	}
	if th.Seq.NoteText != th.Foreground {
		t.Errorf("note text = %v, want the foreground %v", th.Seq.NoteText, th.Foreground)
	}
}

func TestLoadThemeMissingFile(t *testing.T) {
	th := LoadTheme(filepath.Join(t.TempDir(), "nope.toml"), 7)
	if th.Background != DefaultTheme(7).Background {
		t.Error("a missing colors.toml should give the default theme")
	}
}

// TestWatchThemeFollowsSwap replays what omarchy-theme-set does: build
// next-theme, delete theme, move next-theme into its place, write
// theme.name.
func TestWatchThemeFollowsSwap(t *testing.T) {
	dir := t.TempDir()
	writeColors(t, filepath.Join(dir, "theme"), `background = "#000000"`)

	got := make(chan *Theme, 8)
	w, err := WatchTheme(dir, 0, func(th *Theme) { got <- th })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	swap := func(bg string) {
		next := filepath.Join(dir, "next-theme")
		writeColors(t, next, `background = "`+bg+`"`)
		if err := os.RemoveAll(filepath.Join(dir, "theme")); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(next, filepath.Join(dir, "theme")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "theme.name"), []byte(bg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(want string) {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			select {
			case th := <-got:
				if th.Background == hexColor(want, 255) {
					return
				}
			case <-deadline:
				t.Fatalf("no reload with background %s", want)
			}
		}
	}

	// Twice, because the first swap is what used to drop the watch.
	swap("#111111")
	wait("#111111")
	swap("#222222")
	wait("#222222")

	// A later retint of the new theme directory is picked up too.
	writeColors(t, filepath.Join(dir, "theme"), `background = "#333333"`)
	wait("#333333")
}

func luminance(c color.NRGBA) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}
