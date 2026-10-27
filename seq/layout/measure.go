package layout

import (
	"math"
	"strings"
	"sync"

	"dyamark/fonts"

	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// TextDimensions contains the size of a measured piece of text.
type TextDimensions struct {
	Width      float64
	Height     float64
	LineHeight float64
}

// Measurer calculates text dimensions and word-wrapping for layout.
type Measurer interface {
	MeasureText(text string, f Font) TextDimensions
	WrapLabel(label string, maxWidth float64, f Font) string
}

// DefaultMeasurer measures text using bundled Liberation Sans fonts,
// reproducing Chromium's SVG getBBox() metrics.
type DefaultMeasurer struct {
	mu      sync.Mutex
	regular *sfnt.Font
	bold    *sfnt.Font
}

var (
	defaultMeasurerInstance *DefaultMeasurer
	defaultMeasurerOnce     sync.Once
)

// Default returns the shared DefaultMeasurer instance.
func Default() *DefaultMeasurer {
	defaultMeasurerOnce.Do(func() {
		m := &DefaultMeasurer{}
		reg, err := opentype.Parse(fonts.RegularTTF)
		if err == nil {
			m.regular = reg
		}
		bld, err := opentype.Parse(fonts.BoldTTF)
		if err == nil {
			m.bold = bld
		}
		defaultMeasurerInstance = m
	})
	return defaultMeasurerInstance
}

func splitLineBreaks(s string) []string {
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "<br>", "\n")
	return strings.Split(s, "\n")
}

func hasBreaks(s string) bool {
	return strings.Contains(s, "<br>") ||
		strings.Contains(s, "<br/>") ||
		strings.Contains(s, "<br />") ||
		strings.Contains(s, "\n")
}

// MeasureText calculates the width and height of text across multiple lines.
func (m *DefaultMeasurer) MeasureText(text string, f Font) TextDimensions {
	if text == "" {
		return TextDimensions{}
	}
	lines := splitLineBreaks(text)
	var maxW float64
	sz := f.Size
	if sz <= 0 {
		sz = 16
	}

	// Chromium's SVG getBBox fontBoundingBox metrics for Liberation Sans (unitsPerEm = 2048):
	// WinAscent = 1854, WinDescent = 434
	ascent := math.Round(1854.0 * sz / 2048.0)
	descent := math.Round(434.0 * sz / 2048.0)
	lineH := ascent + descent

	for _, line := range lines {
		w := m.measureLineWidth(line, f)
		if w > maxW {
			maxW = w
		}
	}

	totalH := float64(len(lines)) * lineH
	return TextDimensions{
		Width:      math.Round(maxW),
		Height:     totalH,
		LineHeight: lineH,
	}
}

func (m *DefaultMeasurer) measureLineWidth(line string, f Font) float64 {
	if line == "" {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	sFont := m.regular
	if f.Weight >= 700 && m.bold != nil {
		sFont = m.bold
	}
	if sFont == nil {
		return float64(len(line)) * f.Size * 0.5
	}

	var buf sfnt.Buffer
	var totalAdv float64
	sz := f.Size
	if sz <= 0 {
		sz = 16
	}
	scale := sz / 2048.0

	var prevIdx sfnt.GlyphIndex
	hasPrev := false

	for _, r := range line {
		idx, err := sFont.GlyphIndex(&buf, r)
		if err != nil {
			continue
		}
		if hasPrev {
			kern, err := sFont.Kern(&buf, prevIdx, idx, 2048, 0)
			if err == nil && kern != 0 {
				totalAdv += float64(kern) * scale
			}
		}
		adv, err := sFont.GlyphAdvance(&buf, idx, 2048, 0)
		if err == nil {
			totalAdv += float64(adv) * scale
		}
		prevIdx = idx
		hasPrev = true
	}

	return totalAdv
}

// WrapLabel breaks label into multiple lines using <br/> when it exceeds maxWidth.
func (m *DefaultMeasurer) WrapLabel(label string, maxWidth float64, f Font) string {
	if label == "" || hasBreaks(label) || maxWidth <= 0 {
		return label
	}

	words := strings.Fields(label)
	if len(words) == 0 {
		return label
	}

	var completedLines []string
	var nextLine string

	for i, word := range words {
		wordLen := m.measureLineWidth(word+" ", f)
		nextLen := m.measureLineWidth(nextLine, f)

		if wordLen > maxWidth {
			if nextLine != "" {
				completedLines = append(completedLines, nextLine)
				nextLine = ""
			}
			completedLines = append(completedLines, word)
		} else if nextLen+wordLen >= maxWidth && nextLine != "" {
			completedLines = append(completedLines, nextLine)
			nextLine = word
		} else {
			if nextLine == "" {
				nextLine = word
			} else {
				nextLine = nextLine + " " + word
			}
		}

		if i == len(words)-1 && nextLine != "" {
			completedLines = append(completedLines, nextLine)
		}
	}

	if len(completedLines) == 0 {
		return label
	}
	return strings.Join(completedLines, "<br/>")
}
