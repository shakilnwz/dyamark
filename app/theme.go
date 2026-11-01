package app

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"dyamark/palette"
	"dyamark/seq"

	"github.com/fsnotify/fsnotify"
	"github.com/pelletier/go-toml/v2"
)

// defaultRounding is used when hyprctl can't be asked.
const defaultRounding = 10

// RawOmarchyColors maps the keys in ~/.local/state/omarchy/current/theme/colors.toml
type RawOmarchyColors struct {
	Accent              string `toml:"accent"`
	Cursor              string `toml:"cursor"`
	Foreground          string `toml:"foreground"`
	Background          string `toml:"background"`
	LighterBackground   string `toml:"lighter_background"`
	SelectionForeground string `toml:"selection_foreground"`
	SelectionBackground string `toml:"selection_background"`
	Color0              string `toml:"color0"`
	Color1              string `toml:"color1"`
	Color2              string `toml:"color2"`
	Color3              string `toml:"color3"`
	Color4              string `toml:"color4"`
	Color5              string `toml:"color5"`
	Color6              string `toml:"color6"`
	Color7              string `toml:"color7"`
	Color8              string `toml:"color8"`
	Color9              string `toml:"color9"`
	Color10             string `toml:"color10"`
	Color11             string `toml:"color11"`
	Color12             string `toml:"color12"`
	Color13             string `toml:"color13"`
	Color14             string `toml:"color14"`
	Color15             string `toml:"color15"`
}

// HyprlandOption represents the JSON returned by `hyprctl getoption ... -j`
type HyprlandOption struct {
	Option string `json:"option"`
	Int    int    `json:"int"`
	Set    bool   `json:"set"`
}

// OmarchyDir is the directory Omarchy swaps the current theme into.
func OmarchyDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "omarchy", "current"), nil
}

// HyprlandRounding asks hyprctl for decoration:rounding. ok is false when
// hyprctl isn't there or its answer can't be read; zero is a valid answer.
func HyprlandRounding() (rounding int, ok bool) {
	out, err := exec.Command("hyprctl", "getoption", "decoration:rounding", "-j").Output()
	if err != nil {
		return defaultRounding, false
	}
	var opt HyprlandOption
	if err := json.Unmarshal(out, &opt); err != nil || opt.Int < 0 {
		return defaultRounding, false
	}
	return opt.Int, true
}

// LoadTheme reads colors.toml at colorsPath, falling back to the default
// theme for anything missing or unreadable.
func LoadTheme(colorsPath string, rounding int) *palette.Theme {
	t := palette.Default(rounding)
	data, err := os.ReadFile(colorsPath)
	if err != nil {
		return t
	}
	var raw RawOmarchyColors
	if err := toml.Unmarshal(data, &raw); err != nil {
		return t
	}

	set := func(dst *color.NRGBA, hex string) {
		if hex != "" {
			*dst = seq.HexColor(hex, 255)
		}
	}
	set(&t.Background, raw.Background)
	set(&t.Foreground, raw.Foreground)
	set(&t.Accent, raw.Accent)
	set(&t.LighterBackground, raw.LighterBackground)
	set(&t.Cursor, raw.Cursor)
	set(&t.SelectionBg, raw.SelectionBackground)
	set(&t.SelectionFg, raw.SelectionForeground)

	rawColors := []string{
		raw.Color0, raw.Color1, raw.Color2, raw.Color3,
		raw.Color4, raw.Color5, raw.Color6, raw.Color7,
		raw.Color8, raw.Color9, raw.Color10, raw.Color11,
		raw.Color12, raw.Color13, raw.Color14, raw.Color15,
	}
	for i, c := range rawColors {
		set(&t.Colors[i], c)
	}
	// Only the background was given: derive a lighter one, so actor boxes
	// still stand out from the page.
	if raw.Background != "" && raw.LighterBackground == "" {
		t.LighterBackground = palette.Mix(t.Background, t.Foreground, 0.08)
	}

	t.DeriveSeq()
	return t
}

// ThemeWatcher reloads the theme when Omarchy switches or retints it.
//
// omarchy-theme-set replaces current/theme wholesale (rm -rf, then mv), which
// silently drops any watch on that directory, and then writes
// current/theme.name. So the parent is watched for the swap, and the theme
// directory is watched again after each swap for later edits to colors.toml.
type ThemeWatcher struct {
	watcher  *fsnotify.Watcher
	dir      string
	rounding int
	onChange func(*palette.Theme)

	mu    sync.Mutex
	timer *time.Timer
}

// themeReloadDelay batches the burst of events a theme switch makes.
const themeReloadDelay = 100 * time.Millisecond

// WatchTheme calls onChange with a freshly loaded theme after each change
// under dir (Omarchy's current/ directory).
func WatchTheme(dir string, rounding int, onChange func(*palette.Theme)) (*ThemeWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, fmt.Errorf("watching %s: %w", dir, err)
	}
	tw := &ThemeWatcher{watcher: w, dir: dir, rounding: rounding, onChange: onChange}
	tw.watchThemeDir()
	go tw.run()
	return tw, nil
}

// Close stops watching.
func (tw *ThemeWatcher) Close() error {
	tw.mu.Lock()
	if tw.timer != nil {
		tw.timer.Stop()
	}
	tw.mu.Unlock()
	return tw.watcher.Close()
}

func (tw *ThemeWatcher) themeDir() string { return filepath.Join(tw.dir, "theme") }

// watchThemeDir (re-)adds the theme directory. Adding a path that is
// already watched is a no-op, and a missing directory is picked up after
// the next swap.
func (tw *ThemeWatcher) watchThemeDir() {
	_ = tw.watcher.Add(tw.themeDir())
}

func (tw *ThemeWatcher) run() {
	for {
		select {
		case ev, ok := <-tw.watcher.Events:
			if !ok {
				return
			}
			if tw.relevant(ev) {
				tw.scheduleReload()
			}
		case err, ok := <-tw.watcher.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "dyamark: theme watcher: %v\n", err)
		}
	}
}

func (tw *ThemeWatcher) relevant(ev fsnotify.Event) bool {
	switch filepath.Dir(ev.Name) {
	case tw.dir:
		base := filepath.Base(ev.Name)
		return base == "theme" || base == "theme.name"
	case tw.themeDir():
		return filepath.Base(ev.Name) == "colors.toml" && !ev.Has(fsnotify.Remove)
	}
	return false
}

func (tw *ThemeWatcher) scheduleReload() {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.timer != nil {
		tw.timer.Stop()
	}
	tw.timer = time.AfterFunc(themeReloadDelay, func() {
		tw.watchThemeDir()
		tw.onChange(LoadTheme(filepath.Join(tw.themeDir(), "colors.toml"), tw.rounding))
	})
}
