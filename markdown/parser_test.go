package markdown

import (
	"testing"
)

const fence = "```"

// A ```mermaid block is a sequence diagram only when its first statement,
// after front matter, directives and comments, is sequenceDiagram.
func TestMermaidTypeDetection(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		sequence bool
		diagType string
	}{
		{"plain sequence", "sequenceDiagram\nA->>B: hi\n", true, ""},
		{"front matter sequence", "---\ntitle: Login\n---\nsequenceDiagram\nA->>B: hi\n", true, ""},
		{"init sequence", "%%{init: {'theme': 'dark'}}%%\nsequenceDiagram\nA->>B: hi\n", true, ""},
		{"comment sequence", "%% the login flow\n\nsequenceDiagram\nA->>B: hi\n", true, ""},
		{"multi-line init sequence", "%%{init: {\n  'theme': 'dark'\n}}%%\nsequenceDiagram\nA->>B: hi\n", true, ""},
		{"front matter flowchart", "---\ntitle: Flow\n---\nflowchart TD\nA-->B\n", false, "flowchart"},
		{"init flowchart", "%%{init: {'theme': 'dark'}}%%\nflowchart TD\nA-->B\n", false, "flowchart"},
		{"front matter and init class", "---\nconfig:\n  look: handDrawn\n---\n%%{init: {}}%%\n%% c\nclassDiagram\nclass A\n", false, "classDiagram"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse([]byte(fence+"mermaid\n"+tt.code+fence+"\n"), nil)
			if len(doc.Blocks) != 1 {
				t.Fatalf("got %d blocks, want 1", len(doc.Blocks))
			}
			switch b := doc.Blocks[0].(type) {
			case *SequenceBlock:
				if !tt.sequence {
					t.Fatalf("drawn as a sequence diagram, want %s with the mmdc hint", tt.diagType)
				}
				if b.Error != nil {
					t.Errorf("sequence diagram failed: %v", b.Error)
				}
			case *MermaidOtherBlock:
				if tt.sequence {
					t.Fatalf("got %s with the mmdc hint, want a sequence diagram", b.DiagramType)
				}
				if b.DiagramType != tt.diagType {
					t.Errorf("diagram type %q, want %q", b.DiagramType, tt.diagType)
				}
			default:
				t.Fatalf("got %T", b)
			}
		})
	}
}

// A raw .mmd file is wrapped in a fence when it's a sequence diagram, with
// or without front matter in front.
func TestRawSequenceSource(t *testing.T) {
	for _, src := range []string{
		"sequenceDiagram\nA->>B: hi\n",
		"---\ntitle: Login\n---\nsequenceDiagram\nA->>B: hi\n",
		"%%{init: {'theme': 'forest'}}%%\nsequenceDiagram\nA->>B: hi\n",
	} {
		doc := Parse([]byte(src), nil)
		if len(doc.Blocks) != 1 || len(doc.Sequences) != 1 {
			t.Errorf("%q: got %d blocks and %d sequence diagrams, want one diagram", src, len(doc.Blocks), len(doc.Sequences))
			continue
		}
		if err := doc.Sequences[0].Error; err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}

	// A Markdown file with YAML front matter stays Markdown.
	doc := Parse([]byte("---\ntitle: Notes\n---\n\n# Notes\n"), nil)
	if len(doc.Sequences) != 0 {
		t.Error("Markdown with front matter was read as a diagram")
	}
}
