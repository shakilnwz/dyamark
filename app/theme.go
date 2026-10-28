package app

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"dyamark/seq"

	"github.com/fsnotify/fsnotify"
	"github.com/pelletier/go-toml/v2"
)

// defaultRounding is used when hyprctl can't be asked.
const defaultRounding = 10

// Theme holds the application UI palette, desktop rounding, and diagram theme.
type Theme struct {
	Background        color.NRGBA
	Foreground        color.NRGBA
	Accent            color.NRGBA
	LighterBackground color.NRGBA
	Cursor            color.NRGBA
	SelectionBg       color.NRGBA
	SelectionFg       color.NRGBA
	Colors            [16]color.NRGBA
	Rounding          int

	Seq seq.Theme
}

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

// DefaultTheme returns the fallback dark theme with #FFD60A accent.
func DefaultTheme(rounding int) *Theme {
	t := &Theme{
		Background:        hexColor("#0b0f19", 255),
		Foreground:        hexColor("#e6edf3", 255),
		Accent:            hexColor("#FFD60A", 255),
		LighterBackground: hexColor("#1f242c", 255),
		Cursor:            hexColor("#e6edf3", 255),
		SelectionBg:       hexColor("#2f3746", 255),
		SelectionFg:       hexColor("#ffffff", 255),
		Rounding:          rounding,
	}
	t.deriveSeqTheme()
	return t
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
func LoadTheme(colorsPath string, rounding int) *Theme {
	t := DefaultTheme(rounding)
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
			*dst = hexColor(hex, 255)
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
		t.LighterBackground = mix(t.Background, t.Foreground, 0.08)
	}

	t.deriveSeqTheme()
	return t
}

func (t *Theme) deriveSeqTheme() {
	// Notes use the theme's yellow, falling back to amber.
	noteAccent := hexColor("#ffd166", 255)
	if t.Colors[3].A != 0 {
		noteAccent = t.Colors[3]
	}
	if t.Colors[11].A != 0 {
		noteAccent = t.Colors[11]
	}

	t.Seq = seq.Theme{
		ActorBkg:         t.LighterBackground,
		ActorBorder:      t.Accent,
		ActorText:        t.Foreground,
		Lifeline:         withAlpha(t.Foreground, 110),
		Signal:           t.Accent,
		SignalText:       t.Foreground,
		ActivationBkg:    withAlpha(t.Accent, 60),
		ActivationBorder: t.Accent,
		// Mixed from the page colour, so notes read on light themes too.
		NoteBkg:     mix(t.Background, noteAccent, 0.18),
		NoteBorder:  noteAccent,
		NoteText:    t.Foreground,
		FrameBkg:    withAlpha(t.LighterBackground, 90),
		FrameBorder: withAlpha(t.Accent, 180),
		// The tab is the faint frame fill, so both read in the text colour.
		FrameText: t.Foreground,
		LabelText: t.Foreground,
		Rounding:  float32(t.Rounding),
	}
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
	onChange func(*Theme)

	mu    sync.Mutex
	timer *time.Timer
}

// themeReloadDelay batches the burst of events a theme switch makes.
const themeReloadDelay = 100 * time.Millisecond

// WatchTheme calls onChange with a freshly loaded theme after each change
// under dir (Omarchy's current/ directory).
func WatchTheme(dir string, rounding int, onChange func(*Theme)) (*ThemeWatcher, error) {
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

func hexColor(hex string, alpha uint8) color.NRGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) == 6 {
		val, err := strconv.ParseUint(hex, 16, 32)
		if err == nil {
			return color.NRGBA{
				R: uint8(val >> 16),
				G: uint8(val >> 8),
				B: uint8(val),
				A: alpha,
			}
		}
	} else if len(hex) == 8 {
		val, err := strconv.ParseUint(hex, 16, 32)
		if err == nil {
			return color.NRGBA{
				R: uint8(val >> 24),
				G: uint8(val >> 16),
				B: uint8(val >> 8),
				A: uint8(val),
			}
		}
	}
	return color.NRGBA{R: 200, G: 200, B: 200, A: alpha}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

// mix blends b into a by t (0 gives a, 1 gives b), keeping a's alpha.
func mix(a, b color.NRGBA, t float32) color.NRGBA {
	lerp := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return color.NRGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: a.A}
}
