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

// Collection returns a FontFace collection containing the bundled Liberation Sans fonts.
func Collection() ([]font.FontFace, error) {
	regFace, err := opentype.Parse(RegularTTF)
	if err != nil {
		return nil, fmt.Errorf("parsing regular font: %w", err)
	}

	boldFace, err := opentype.Parse(BoldTTF)
	if err != nil {
		return nil, fmt.Errorf("parsing bold font: %w", err)
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
	}
	return faces, nil
}
