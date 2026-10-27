package app

import (
	"testing"
)

const markdownWithFences = `# Title

Some text here.

` + "```mermaid" + `
sequenceDiagram
Alice->>Bob: Hello
` + "```" + `

Middle text.

` + "```mermaid" + `
sequenceDiagram
Bob->>Alice: World
` + "```" + `
`

func TestExtractDiagrams(t *testing.T) {
	// Raw .mmd
	raw := `sequenceDiagram
Alice->>Bob: Hello`
	d1 := ExtractDiagrams(raw)
	if len(d1) != 1 {
		t.Fatalf("expected 1 diagram from raw mmd, got %d", len(d1))
	}
	if len(d1[0].Participants) != 2 {
		t.Errorf("expected 2 participants, got %d", len(d1[0].Participants))
	}

	// Markdown with multiple fences
	dMulti := ExtractDiagrams(markdownWithFences)
	if len(dMulti) != 2 {
		t.Fatalf("expected 2 diagrams from markdown, got %d", len(dMulti))
	}

	// Empty
	dEmpty := ExtractDiagrams("just regular markdown without mermaid blocks")
	if len(dEmpty) != 0 {
		t.Errorf("expected 0 diagrams, got %d", len(dEmpty))
	}
}

func TestDefaultSampleDiagramParses(t *testing.T) {
	d := ExtractDiagrams(DefaultSampleDiagram)
	if len(d) != 1 {
		t.Fatalf("expected 1 diagram from DefaultSampleDiagram, got %d", len(d))
	}
	if d[0].Title != "Authentication Sequence Flow" {
		t.Errorf("expected title 'Authentication Sequence Flow', got %q", d[0].Title)
	}
}

func TestRSSKB(t *testing.T) {
	rss, err := RSSKB()
	if err != nil {
		t.Fatalf("RSSKB failed: %v", err)
	}
	if rss == 0 {
		t.Error("expected non-zero RSS")
	}
}
