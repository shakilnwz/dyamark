package layout

import (
	"strings"
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

// Involving keeps what a participant takes part in and drops the rest.
func TestInvolving(t *testing.T) {
	src := `sequenceDiagram
participant A
participant B
participant C
A->>B: one
activate B
B->>C: two
C->>A: three
deactivate B
Note over A,B: ab
Note right of C: c
`
	d, err := parse.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	diag, err := Layout(d, Default())
	if err != nil {
		t.Fatal(err)
	}
	b := diag.Involving("B")
	if len(b.Actors) != 2 || len(b.Lifelines) != 1 || len(b.Activations) != 1 {
		t.Errorf("B: %d actors, %d lifelines, %d activations", len(b.Actors), len(b.Lifelines), len(b.Activations))
	}
	var texts []string
	for _, m := range b.Messages {
		texts = append(texts, m.Text[0].Text)
	}
	if strings.Join(texts, ",") != "one,two" {
		t.Errorf("B's messages: %v", texts)
	}
	if len(b.Notes) != 1 || b.Notes[0].Text[0].Text != "ab" {
		t.Errorf("B's notes: %+v", b.Notes)
	}
	if b.ViewBox != diag.ViewBox {
		t.Error("the viewBox moved")
	}
	if c := diag.Involving("C"); len(c.Notes) != 1 || len(c.Activations) != 0 {
		t.Errorf("C: %d notes, %d activations", len(c.Notes), len(c.Activations))
	}
}
