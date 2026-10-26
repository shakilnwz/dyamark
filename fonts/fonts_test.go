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

	if len(faces) != 2 {
		t.Fatalf("expected 2 font faces, got %d", len(faces))
	}
}
