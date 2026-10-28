// Package seq holds the sequence-diagram pieces shared by its parser,
// layout and drawing subpackages.
package seq

import (
	"image/color"
	"math"
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
	// FrameText is a frame's title and section labels; LabelText the
	// keyword ("loop", "alt", …) in its corner tab.
	FrameText color.NRGBA
	LabelText color.NRGBA
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
		LabelText:        HexColor("#333333", 255),
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
		LabelText:        HexColor("#dddddd", 255),
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
		LabelText:        HexColor("#13540c", 255),
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
		LabelText:        HexColor("#333333", 255),
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
	override(&t.LabelText, "labelTextColor")

	return t
}

// ParseColor parses a CSS colour: hex (#rgb, #rgba, #rrggbb, #rrggbbaa),
// rgb()/rgba(), hsl()/hsla(), or a CSS colour name. It falls back to
// fallback when the colour isn't understood.
func ParseColor(str string, fallback color.NRGBA) color.NRGBA {
	s := strings.TrimSpace(strings.ToLower(str))
	if s == "" || s == "none" || s == "transparent" {
		return color.NRGBA{}
	}
	if c, ok := cssColorNames[s]; ok {
		return c
	}
	if strings.HasPrefix(s, "#") {
		if c, ok := ParseHexColor(s, 255); ok {
			return c
		}
		return fallback
	}
	if c, ok := parseColorFunc(s); ok {
		return c
	}
	return fallback
}

// parseColorFunc parses rgb(), rgba(), hsl() and hsla(), with comma or
// space separated arguments and an optional "/ alpha".
func parseColorFunc(s string) (color.NRGBA, bool) {
	open := strings.IndexByte(s, '(')
	if open < 0 || !strings.HasSuffix(s, ")") {
		return color.NRGBA{}, false
	}
	name := strings.TrimSpace(s[:open])
	args := strings.FieldsFunc(s[open+1:len(s)-1], func(r rune) bool {
		return r == ',' || r == ' ' || r == '/' || r == '\t'
	})
	if len(args) != 3 && len(args) != 4 {
		return color.NRGBA{}, false
	}

	alpha := 1.0
	if len(args) == 4 {
		a, ok := parseNumber(args[3], 1)
		if !ok {
			return color.NRGBA{}, false
		}
		alpha = a
	}

	var r, g, b float64
	switch name {
	case "rgb", "rgba":
		vals := [3]float64{}
		for i := 0; i < 3; i++ {
			v, ok := parseNumber(args[i], 255)
			if !ok {
				return color.NRGBA{}, false
			}
			vals[i] = v / 255
		}
		r, g, b = vals[0], vals[1], vals[2]
	case "hsl", "hsla":
		h, err := strconv.ParseFloat(strings.TrimSuffix(args[0], "deg"), 64)
		if err != nil {
			return color.NRGBA{}, false
		}
		sat, ok1 := parseNumber(args[1], 1)
		light, ok2 := parseNumber(args[2], 1)
		if !ok1 || !ok2 {
			return color.NRGBA{}, false
		}
		r, g, b = hslToRGB(h, sat, light)
	default:
		return color.NRGBA{}, false
	}
	return color.NRGBA{R: unit8(r), G: unit8(g), B: unit8(b), A: unit8(alpha)}, true
}

// parseNumber reads a plain number, or a percentage of scale.
func parseNumber(s string, scale float64) (float64, bool) {
	if strings.HasSuffix(s, "%") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		return v / 100 * scale, err == nil
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func hslToRGB(h, s, l float64) (r, g, b float64) {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	s = math.Max(0, math.Min(1, s))
	l = math.Max(0, math.Min(1, l))
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}

// unit8 maps 0..1 to 0..255, clamped.
func unit8(v float64) uint8 {
	return uint8(math.Round(math.Max(0, math.Min(1, v)) * 255))
}

// ParseHexColor parses a hex string (#rgb, #rgba, #rrggbb, #rrggbbaa) to
// color.NRGBA. alpha is used when the string has no alpha digits.
func ParseHexColor(hex string, alpha uint8) (color.NRGBA, bool) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 || len(hex) == 4 {
		long := make([]byte, 0, 8)
		for i := 0; i < len(hex); i++ {
			long = append(long, hex[i], hex[i])
		}
		hex = string(long)
	}
	if len(hex) != 6 && len(hex) != 8 {
		return color.NRGBA{}, false
	}
	val, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	if len(hex) == 6 {
		return color.NRGBA{R: uint8(val >> 16), G: uint8(val >> 8), B: uint8(val), A: alpha}, true
	}
	return color.NRGBA{R: uint8(val >> 24), G: uint8(val >> 16), B: uint8(val >> 8), A: uint8(val)}, true
}

// HexColor converts a hex string (#rgb, #rrggbb, #rrggbbaa) to color.NRGBA.
func HexColor(hex string, alpha uint8) color.NRGBA {
	if c, ok := ParseHexColor(hex, alpha); ok {
		return c
	}
	return color.NRGBA{R: 200, G: 200, B: 200, A: alpha}
}
