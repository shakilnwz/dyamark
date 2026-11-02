package markdown

import "testing"

func TestAnchor(t *testing.T) {
	doc := Parse([]byte("# Intro\n\n"+
		"Some text.\n\n"+
		"## What's new? (v2.0)\n\n"+
		"## `setSource` and more\n\n"+
		"## Ünïcode Straße\n\n"+
		"## Intro\n\n"+
		"## Intro\n\n"+
		"## Intro-1\n"), nil)
	for _, c := range []struct {
		fragment string
		want     int
	}{
		{"intro", 0},
		{"whats-new-v20", 2},
		{"setsource-and-more", 3},
		{"ünïcode-straße", 4},
		{"%C3%BCn%C3%AFcode-stra%C3%9Fe", 4},
		// Later headings of the same name are numbered, as on GitHub, and
		// a heading named like a numbered one gets the next number.
		{"intro-1", 5},
		{"intro-2", 6},
		{"intro-1-1", 7},
		// Written by hand with capitals.
		{"Intro", 0},
		{"missing", -1},
		{"", -1},
	} {
		if got := doc.Anchor(c.fragment); got != c.want {
			t.Errorf("Anchor(%q) = %d, want %d", c.fragment, got, c.want)
		}
	}
}
