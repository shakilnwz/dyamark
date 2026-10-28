package fonts

import (
	_ "embed"
	"fmt"

	"gioui.org/font"
	"gioui.org/font/opentype"
)

//go:embed LiberationSans-Regular.ttf
var RegularTTF []byte

//go:embed LiberationSans-Bold.ttf
var BoldTTF []byte

//go:embed LiberationSans-Italic.ttf
var ItalicTTF []byte

//go:embed LiberationSans-BoldItalic.ttf
var BoldItalicTTF []byte

//go:embed LiberationMono-Regular.ttf
var MonoTTF []byte

// Mono is the bundled monospace typeface, for source text and code blocks.
// It's in the first frames, before the system fonts arrive.
const Mono = font.Typeface("Liberation Mono")

// Collection returns the bundled fonts: Liberation Sans regular, bold,
// italic and bold italic, and Liberation Mono.
func Collection() ([]font.FontFace, error) {
	regFace, err := opentype.Parse(RegularTTF)
	if err != nil {
		return nil, fmt.Errorf("parsing regular font: %w", err)
	}

	boldFace, err := opentype.Parse(BoldTTF)
	if err != nil {
		return nil, fmt.Errorf("parsing bold font: %w", err)
	}

	italicFace, err := opentype.Parse(ItalicTTF)
	if err != nil {
		return nil, fmt.Errorf("parsing italic font: %w", err)
	}

	boldItalicFace, err := opentype.Parse(BoldItalicTTF)
	if err != nil {
		return nil, fmt.Errorf("parsing bold italic font: %w", err)
	}

	monoFace, err := opentype.Parse(MonoTTF)
	if err != nil {
		return nil, fmt.Errorf("parsing monospace font: %w", err)
	}

	faces := []font.FontFace{
		{
			Font: regFace.Font(),
			Face: regFace,
		},
		{
			Font: boldFace.Font(),
			Face: boldFace,
		},
		{
			Font: italicFace.Font(),
			Face: italicFace,
		},
		{
			Font: boldItalicFace.Font(),
			Face: boldItalicFace,
		},
		{
			Font: monoFace.Font(),
			Face: monoFace,
		},
	}
	return faces, nil
}
