// Package seq holds the sequence-diagram pieces shared by its parser,
// layout and drawing subpackages.
package seq

import (
	"image/color"
	"strconv"
	"strings"
)

// Theme defines colours and style for sequence diagram elements.
type Theme struct {
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
	// Rounding is the corner radius in dp. Zero means square corners.
	Rounding float32
}

// DefaultTheme returns Mermaid's standard default sequence diagram theme.
func DefaultTheme() Theme {
	return Theme{
		ActorBkg:         HexColor("#eaeaea", 255),
		ActorBorder:      HexColor("#666666", 255),
		ActorText:        HexColor("#333333", 255),
		Lifeline:         HexColor("#999999", 255),
		Signal:           HexColor("#333333", 255),
		SignalText:       HexColor("#333333", 255),
		ActivationBkg:    HexColor("#f4f4f4", 255),
		ActivationBorder: HexColor("#666666", 255),
		NoteBkg:          HexColor("#fff5ad", 255),
		NoteBorder:       HexColor("#aaaa33", 255),
		NoteText:         HexColor("#333333", 255),
		FrameBkg:         HexColor("#efefef", 180),
		FrameBorder:      HexColor("#444444", 255),
		FrameText:        HexColor("#333333", 255),
		Rounding:         3,
	}
}

// DarkTheme returns Mermaid's standard dark sequence diagram theme.
func DarkTheme() Theme {
	return Theme{
		ActorBkg:         HexColor("#2b2b2b", 255),
		ActorBorder:      HexColor("#8a8a8a", 255),
		ActorText:        HexColor("#dddddd", 255),
		Lifeline:         HexColor("#8a8a8a", 160),
		Signal:           HexColor("#c1c1c1", 255),
		SignalText:       HexColor("#dddddd", 255),
		ActivationBkg:    HexColor("#3b3b3b", 255),
		ActivationBorder: HexColor("#8a8a8a", 255),
		NoteBkg:          HexColor("#333333", 255),
		NoteBorder:       HexColor("#555555", 255),
		NoteText:         HexColor("#dddddd", 255),
		FrameBkg:         HexColor("#242424", 180),
		FrameBorder:      HexColor("#8a8a8a", 255),
		FrameText:        HexColor("#dddddd", 255),
		Rounding:         3,
	}
}

// ForestTheme returns Mermaid's standard forest sequence diagram theme.
func ForestTheme() Theme {
	return Theme{
		ActorBkg:         HexColor("#cde498", 255),
		ActorBorder:      HexColor("#13540c", 255),
		ActorText:        HexColor("#13540c", 255),
		Lifeline:         HexColor("#13540c", 160),
		Signal:           HexColor("#13540c", 255),
		SignalText:       HexColor("#13540c", 255),
		ActivationBkg:    HexColor("#e0f0d0", 255),
		ActivationBorder: HexColor("#13540c", 255),
		NoteBkg:          HexColor("#edf2ae", 255),
		NoteBorder:       HexColor("#6eaa49", 255),
		NoteText:         HexColor("#13540c", 255),
		FrameBkg:         HexColor("#cde498", 160),
		FrameBorder:      HexColor("#13540c", 255),
		FrameText:        HexColor("#13540c", 255),
		Rounding:         3,
	}
}

// NeutralTheme returns Mermaid's standard neutral sequence diagram theme.
func NeutralTheme() Theme {
	return Theme{
		ActorBkg:         HexColor("#eeeeee", 255),
		ActorBorder:      HexColor("#999999", 255),
		ActorText:        HexColor("#333333", 255),
		Lifeline:         HexColor("#999999", 160),
		Signal:           HexColor("#666666", 255),
		SignalText:       HexColor("#333333", 255),
		ActivationBkg:    HexColor("#eeeeee", 255),
		ActivationBorder: HexColor("#999999", 255),
		NoteBkg:          HexColor("#fff5ad", 255),
		NoteBorder:       HexColor("#aaaa33", 255),
		NoteText:         HexColor("#333333", 255),
		FrameBkg:         HexColor("#eeeeee", 160),
		FrameBorder:      HexColor("#999999", 255),
		FrameText:        HexColor("#333333", 255),
		Rounding:         3,
	}
}

// ResolveTheme returns the active Theme given a base theme (e.g. from
// Omarchy), an optional diagram theme override ("default", "dark", "forest",
// "neutral", "base"), and any Mermaid themeVariables specified in the diagram.
func ResolveTheme(base Theme, themeName string, themeVars map[string]string) Theme {
	t := base
	switch strings.ToLower(strings.TrimSpace(themeName)) {
	case "default":
		t = DefaultTheme()
	case "dark":
		t = DarkTheme()
	case "forest":
		t = ForestTheme()
	case "neutral":
		t = NeutralTheme()
	case "base":
		// Base keeps the passed in base theme
	}

	if len(themeVars) == 0 {
		return t
	}

	override := func(dst *color.NRGBA, key string) {
		if val, ok := themeVars[key]; ok && strings.TrimSpace(val) != "" {
			*dst = ParseColor(val, *dst)
		}
	}

	override(&t.ActorBkg, "actorBkg")
	override(&t.ActorBorder, "actorBorder")
	override(&t.ActorText, "actorTextColor")
	override(&t.Lifeline, "actorLineColor")
	override(&t.Signal, "signalColor")
	override(&t.SignalText, "signalTextColor")
	override(&t.ActivationBkg, "activationBkgColor")
	override(&t.ActivationBorder, "activationBorderColor")
	override(&t.NoteBkg, "noteBkgColor")
	override(&t.NoteBorder, "noteBorderColor")
	override(&t.NoteText, "noteTextColor")
	override(&t.FrameBkg, "labelBoxBkgColor")
	override(&t.FrameBorder, "labelBoxBorderColor")
	override(&t.FrameText, "loopTextColor")

	return t
}

// ParseColor parses a CSS hex color or simple color name, falling back to
// fallback when invalid.
func ParseColor(str string, fallback color.NRGBA) color.NRGBA {
	s := strings.TrimSpace(strings.ToLower(str))
	if s == "" || s == "none" || s == "transparent" {
		return color.NRGBA{}
	}
	switch s {
	case "black":
		return color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	case "white":
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case "red":
		return color.NRGBA{R: 255, G: 0, B: 0, A: 255}
	case "green":
		return color.NRGBA{R: 0, G: 128, B: 0, A: 255}
	case "blue":
		return color.NRGBA{R: 0, G: 0, B: 255, A: 255}
	case "yellow":
		return color.NRGBA{R: 255, G: 255, B: 0, A: 255}
	}
	if c, ok := ParseHexColor(s, fallback.A); ok {
		return c
	}
	return fallback
}

// ParseHexColor parses a hex string (#rgb, #rrggbb, #rrggbbaa) to color.NRGBA.
func ParseHexColor(hex string, alpha uint8) (color.NRGBA, bool) {
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
			}, true
		}
	} else if len(hex) == 8 {
		val, err := strconv.ParseUint(hex, 16, 32)
		if err == nil {
			return color.NRGBA{
				R: uint8(val >> 24),
				G: uint8(val >> 16),
				B: uint8(val >> 8),
				A: uint8(val),
			}, true
		}
	}
	return color.NRGBA{}, false
}

// HexColor converts a hex string (#rgb, #rrggbb, #rrggbbaa) to color.NRGBA.
func HexColor(hex string, alpha uint8) color.NRGBA {
	if c, ok := ParseHexColor(hex, alpha); ok {
		return c
	}
	return color.NRGBA{R: 200, G: 200, B: 200, A: alpha}
}
