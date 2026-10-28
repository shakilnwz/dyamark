package fonts

import (
	"testing"

	"gioui.org/font"
)

func TestBundledFonts(t *testing.T) {
	if len(RegularTTF) == 0 {
		t.Fatal("RegularTTF is empty")
	}
	if len(BoldTTF) == 0 {
		t.Fatal("BoldTTF is empty")
	}

	faces, err := Collection()
	if err != nil {
		t.Fatalf("Collection() returned error: %v", err)
	}

	if len(faces) != 5 {
		t.Fatalf("expected 5 font faces, got %d", len(faces))
	}
	if got := faces[2].Font.Style; got != font.Italic {
		t.Errorf("third face's style is %v, want italic", got)
	}
	if got := faces[3].Font; got.Style != font.Italic || got.Weight != font.Bold {
		t.Errorf("fourth face is %v, want bold italic", got)
	}
	if got := faces[4].Font.Typeface; got != Mono {
		t.Errorf("monospace face is %q, want %q", got, Mono)
	}
}
