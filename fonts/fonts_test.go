package fonts

import (
	"testing"
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

	if len(faces) != 3 {
		t.Fatalf("expected 3 font faces, got %d", len(faces))
	}
	if got := faces[2].Font.Typeface; got != Mono {
		t.Errorf("monospace face is %q, want %q", got, Mono)
	}
}
