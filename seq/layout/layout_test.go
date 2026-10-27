package layout

import (
	"testing"

	"dyamark/seq/parse"
)

func TestBasicLayout(t *testing.T) {
	src := `sequenceDiagram
Alice->>Bob: Hello Bob
Bob-->>Alice: Hi Alice
`
	d, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	diag, err := Layout(d, Default())
	if err != nil {
		t.Fatalf("layout failed: %v", err)
	}

	if len(diag.Actors) != 4 { // 2 top + 2 bottom
		t.Fatalf("expected 4 actors, got %d", len(diag.Actors))
	}
	if len(diag.Lifelines) != 2 {
		t.Fatalf("expected 2 lifelines, got %d", len(diag.Lifelines))
	}
	if len(diag.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(diag.Messages))
	}

	t.Logf("ViewBox: %+v", diag.ViewBox)
	t.Logf("Actors: %+v", diag.Actors)
	t.Logf("Messages: %+v", diag.Messages)
}
