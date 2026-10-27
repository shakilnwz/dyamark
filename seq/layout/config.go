package layout

import (
	"strconv"
	"strings"

	"dyamark/seq/parse"
)

// Look is Mermaid's rendering style for the diagram.
type Look int8

const (
	// LookNeo is Mermaid 12's default for sequence diagrams: actors as
	// tall as their glyph-and-label stack, rounder corners.
	LookNeo Look = iota
	LookClassic
)

// Mermaid's default font-family. The trailing semicolon makes it invalid
// CSS, so Mermaid's inline font-family is dropped and Chromium measures
// text in its default font, which is serif. Every Mermaid 12 sequence
// diagram is sized that way unless the config sets fontFamily.
const defaultFontFamily = `"trebuchet ms", verdana, arial, sans-serif;`

// Config is the sequence configuration in effect, Mermaid's defaults
// overlaid with the diagram's own settings.
type Config struct {
	Look  Look
	Theme string

	ActivationWidth float64
	DiagramMarginX  float64
	DiagramMarginY  float64
	ActorMargin     float64
	Width           float64
	Height          float64
	BoxMargin       float64
	BoxTextMargin   float64
	NoteMargin      float64
	MessageMargin   float64
	BottomMarginAdj float64
	WrapPadding     float64
	LabelBoxWidth   float64
	LabelBoxHeight  float64

	MessageAlign string
	NoteAlign    string

	MirrorActors           bool
	RightAngles            bool
	ShowSequenceNumbers    bool
	HideUnusedParticipants bool

	// FontFamily is the font-family Mermaid gives text it measures and
	// draws, as written (it may be invalid CSS, as the default is).
	FontFamily string
	// RenderedFamily is the family drawn text actually gets: FontFamily
	// when it's valid, otherwise the theme's. Mermaid measures some
	// heights from drawn elements, which use it.
	RenderedFamily string
	FontSize       float64

	ActorFontWeight   int
	NoteFontWeight    int
	MessageFontWeight int
}

// ResolveConfig works out the configuration Mermaid 12 renders a diagram
// with.
//
// Mermaid copies the top-level fontSize and fontFamily over the sequence
// ones (setConf in sequenceRenderer.ts), and both always have defaults, so
// sequence.messageFontSize and friends have no effect. Look and theme come
// from the top level when set, else from the sequence defaults: neo and
// redux-color.
func ResolveConfig(c parse.Config) Config {
	cfg := Config{
		Look:              LookNeo,
		Theme:             "redux-color",
		ActivationWidth:   10,
		DiagramMarginX:    50,
		DiagramMarginY:    10,
		ActorMargin:       50,
		Width:             150,
		Height:            65,
		BoxMargin:         10,
		BoxTextMargin:     5,
		NoteMargin:        10,
		MessageMargin:     35,
		BottomMarginAdj:   1,
		WrapPadding:       10,
		LabelBoxWidth:     50,
		LabelBoxHeight:    20,
		MessageAlign:      "center",
		NoteAlign:         "center",
		MirrorActors:      true,
		FontFamily:        defaultFontFamily,
		FontSize:          16,
		ActorFontWeight:   400,
		NoteFontWeight:    400,
		MessageFontWeight: 400,
	}
	s := c.Sequence
	setF := func(dst *float64, v *float64) {
		if v != nil {
			*dst = *v
		}
	}
	setB := func(dst *bool, v *bool) {
		if v != nil {
			*dst = *v
		}
	}
	setS := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	setW := func(dst *int, v *string) {
		if v != nil {
			if n, err := strconv.Atoi(*v); err == nil {
				*dst = n
			} else if *v == "bold" {
				*dst = 700
			} else if *v == "normal" {
				*dst = 400
			}
		}
	}
	setF(&cfg.ActivationWidth, s.ActivationWidth)
	setF(&cfg.DiagramMarginX, s.DiagramMarginX)
	setF(&cfg.DiagramMarginY, s.DiagramMarginY)
	setF(&cfg.ActorMargin, s.ActorMargin)
	setF(&cfg.Width, s.Width)
	setF(&cfg.Height, s.Height)
	setF(&cfg.BoxMargin, s.BoxMargin)
	setF(&cfg.BoxTextMargin, s.BoxTextMargin)
	setF(&cfg.NoteMargin, s.NoteMargin)
	setF(&cfg.MessageMargin, s.MessageMargin)
	setF(&cfg.BottomMarginAdj, s.BottomMarginAdj)
	setF(&cfg.WrapPadding, s.WrapPadding)
	setF(&cfg.LabelBoxWidth, s.LabelBoxWidth)
	setF(&cfg.LabelBoxHeight, s.LabelBoxHeight)
	setS(&cfg.MessageAlign, s.MessageAlign)
	setS(&cfg.NoteAlign, s.NoteAlign)
	setB(&cfg.MirrorActors, s.MirrorActors)
	setB(&cfg.RightAngles, s.RightAngles)
	setB(&cfg.ShowSequenceNumbers, s.ShowSequenceNumbers)
	setB(&cfg.HideUnusedParticipants, s.HideUnusedParticipants)
	setW(&cfg.ActorFontWeight, s.ActorFontWeight)
	setW(&cfg.NoteFontWeight, s.NoteFontWeight)
	setW(&cfg.MessageFontWeight, s.MessageFontWeight)
	if w, ok := c.Raw["fontWeight"]; ok {
		var top string
		switch w := w.(type) {
		case string:
			top = w
		case float64:
			top = strconv.FormatFloat(w, 'f', -1, 64)
		}
		setW(&cfg.ActorFontWeight, &top)
		setW(&cfg.NoteFontWeight, &top)
		setW(&cfg.MessageFontWeight, &top)
	}

	if c.FontSize != nil {
		cfg.FontSize = *c.FontSize
	}
	userFamily := c.FontFamily != ""
	if userFamily {
		cfg.FontFamily = c.FontFamily
	}
	if c.Theme != "" {
		cfg.Theme = c.Theme
	}
	look, _ := c.Raw["look"].(string)
	if look == "" {
		if seq, ok := c.Raw["sequence"].(map[string]any); ok {
			look, _ = seq["look"].(string)
		}
	}
	if look == "classic" {
		cfg.Look = LookClassic
	}

	switch {
	case userFamily:
		cfg.RenderedFamily = cfg.FontFamily
	case strings.HasPrefix(cfg.Theme, "redux"):
		cfg.RenderedFamily = `"Recursive Variable", arial, sans-serif`
	default:
		cfg.RenderedFamily = `"trebuchet ms", verdana, arial, sans-serif`
	}
	return cfg
}

// Font is a text style as Mermaid passes it to its measuring code.
type Font struct {
	Family string
	Size   float64
	Weight int
}

type font = Font

func (c *Config) MessageFont() Font {
	return Font{c.FontFamily, c.FontSize, c.MessageFontWeight}
}

func (c *Config) NoteFont() Font {
	return Font{c.FontFamily, c.FontSize, c.NoteFontWeight}
}

func (c *Config) ActorFont() Font {
	return Font{c.FontFamily, c.FontSize, c.ActorFontWeight}
}

func (c *Config) messageFont() font {
	return c.MessageFont()
}

func (c *Config) noteFont() font {
	return c.NoteFont()
}

func (c *Config) actorFont() font {
	return c.ActorFont()
}

// rendered is f as it's drawn, for heights Mermaid reads back from drawn
// elements with getBBox().
func (c *Config) rendered(f font) font {
	f.Family = c.RenderedFamily
	return f
}
