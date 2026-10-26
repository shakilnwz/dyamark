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

	"github.com/fsnotify/fsnotify"
	"github.com/pelletier/go-toml/v2"
)

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

	Seq SeqTheme
}

// SeqTheme defines colors and style for Mermaid sequence diagram elements.
type SeqTheme struct {
	ActorBkg         color.NRGBA
	ActorBorder      color.NRGBA
	ActorText        color.NRGBA
	Lifeline         color.NRGBA
	Signal           color.NRGBA
	SignalText       color.NRGBA
	ActivationBkg    color.NRGBA
	ActivationBorder color.NRGBA
	NoteBkg          color.NRGBA
	NoteBorder       color.NRGBA
	NoteText         color.NRGBA
	FrameBkg         color.NRGBA
	FrameBorder      color.NRGBA
	FrameText        color.NRGBA
	Rounding         float32
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
func DefaultTheme() *Theme {
	t := &Theme{
		Background:        hexColor("#0b0f19", 255),
		Foreground:        hexColor("#e6edf3", 255),
		Accent:            hexColor("#FFD60A", 255),
		LighterBackground: hexColor("#1f242c", 255),
		Cursor:            hexColor("#e6edf3", 255),
		SelectionBg:       hexColor("#2f3746", 255),
		SelectionFg:       hexColor("#ffffff", 255),
		Rounding:          10,
	}
	t.deriveSeqTheme()
	return t
}

// GetHyprlandRounding queries hyprctl for decoration:rounding.
func GetHyprlandRounding() int {
	out, err := exec.Command("hyprctl", "getoption", "decoration:rounding", "-j").Output()
	if err != nil {
		return 10
	}
	var opt HyprlandOption
	if err := json.Unmarshal(out, &opt); err != nil {
		return 10
	}
	if opt.Int > 0 {
		return opt.Int
	}
	return 10
}

// LoadTheme reads colors from the Omarchy path if present, otherwise returns the default theme.
func LoadTheme() *Theme {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultTheme()
	}

	colorsPath := filepath.Join(home, ".local", "state", "omarchy", "current", "theme", "colors.toml")
	data, err := os.ReadFile(colorsPath)
	if err != nil {
		return DefaultTheme()
	}

	var raw RawOmarchyColors
	if err := toml.Unmarshal(data, &raw); err != nil {
		return DefaultTheme()
	}

	t := DefaultTheme()
	if raw.Background != "" {
		t.Background = hexColor(raw.Background, 255)
	}
	if raw.Foreground != "" {
		t.Foreground = hexColor(raw.Foreground, 255)
	}
	if raw.Accent != "" {
		t.Accent = hexColor(raw.Accent, 255)
	}
	if raw.LighterBackground != "" {
		t.LighterBackground = hexColor(raw.LighterBackground, 255)
	}
	if raw.Cursor != "" {
		t.Cursor = hexColor(raw.Cursor, 255)
	}
	if raw.SelectionBackground != "" {
		t.SelectionBg = hexColor(raw.SelectionBackground, 255)
	}
	if raw.SelectionForeground != "" {
		t.SelectionFg = hexColor(raw.SelectionForeground, 255)
	}

	rawColors := []string{
		raw.Color0, raw.Color1, raw.Color2, raw.Color3,
		raw.Color4, raw.Color5, raw.Color6, raw.Color7,
		raw.Color8, raw.Color9, raw.Color10, raw.Color11,
		raw.Color12, raw.Color13, raw.Color14, raw.Color15,
	}
	for i, c := range rawColors {
		if c != "" {
			t.Colors[i] = hexColor(c, 255)
		}
	}

	t.Rounding = GetHyprlandRounding()
	t.deriveSeqTheme()
	return t
}

func (t *Theme) deriveSeqTheme() {
	r := float32(t.Rounding)
	if r <= 0 {
		r = 6
	}

	// Actor background and border
	actorBkg := t.LighterBackground
	if actorBkg.A == 0 {
		actorBkg = t.Background
	}

	// Notes: use warm yellow / amber (color11 or color3 or #ffd166)
	noteBkg := hexColor("#3d3514", 240)
	noteBorder := hexColor("#ffd166", 255)
	if t.Colors[3].A != 0 {
		noteBorder = t.Colors[3]
	}
	if t.Colors[11].A != 0 {
		noteBorder = t.Colors[11]
	}

	t.Seq = SeqTheme{
		ActorBkg:         actorBkg,
		ActorBorder:      t.Accent,
		ActorText:        t.Foreground,
		Lifeline:         hexColorAlpha(t.Foreground, 110),
		Signal:           t.Accent,
		SignalText:       t.Foreground,
		ActivationBkg:    hexColorAlpha(t.Accent, 60),
		ActivationBorder: t.Accent,
		NoteBkg:          noteBkg,
		NoteBorder:       noteBorder,
		NoteText:         hexColor("#fff3c4", 255),
		FrameBkg:         hexColorAlpha(t.LighterBackground, 90),
		FrameBorder:      hexColorAlpha(t.Accent, 180),
		FrameText:        t.Accent,
		Rounding:         r,
	}
}

// WatchTheme monitors the Omarchy theme directory and notifies onChange whenever colors.toml is updated.
func WatchTheme(onChange func(*Theme)) (*fsnotify.Watcher, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	themeDir := filepath.Join(home, ".local", "state", "omarchy", "current", "theme")
	if _, err := os.Stat(themeDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("theme directory does not exist: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	var mu sync.Mutex
	rearmWatcher := func() {
		mu.Lock()
		defer mu.Unlock()
		_ = watcher.Add(themeDir)
	}

	rearmWatcher()

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Theme switches frequently replace the directory or file
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
					newTheme := LoadTheme()
					if onChange != nil {
						onChange(newTheme)
					}
					// Re-arm watcher in case the target was replaced
					rearmWatcher()
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return watcher, nil
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

func hexColorAlpha(c color.NRGBA, a uint8) color.NRGBA {
	return color.NRGBA{
		R: c.R,
		G: c.G,
		B: c.B,
		A: a,
	}
}
