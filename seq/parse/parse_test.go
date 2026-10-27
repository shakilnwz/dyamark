// Copied from github.com/julianshen/mmgo pkg/parser/sequence/parser_test.go
// and adapted to this package; see UPSTREAM. MIT licence in LICENSE.mmgo.

package parse

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseHeaderRequired(t *testing.T) {
	_, err := Parse("A->>B: hi")
	if err == nil {
		t.Fatal("expected error for missing header")
	}
	if !strings.Contains(err.Error(), "sequenceDiagram") {
		t.Errorf("error should mention sequenceDiagram: %v", err)
	}
}

func TestParseEmptyInput(t *testing.T) {
	_, err := Parse("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseHeaderOnly(t *testing.T) {
	d, err := Parse("sequenceDiagram")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d == nil {
		t.Fatal("diagram is nil")
	}
	if len(d.Participants) != 0 || len(d.Items) != 0 {
		t.Errorf("empty diagram should have no participants/items, got %+v", d)
	}
}

func TestParseHeaderWithLeadingCommentAndBlanks(t *testing.T) {
	input := `%% top comment

sequenceDiagram
    %% inline comment
`
	if _, err := Parse(input); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseParticipants(t *testing.T) {
	input := `sequenceDiagram
    participant Alice
    participant B as Bob
    actor C as Carol
    actor D`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []Participant{
		{ID: "Alice", Kind: ParticipantKindParticipant, BoxIndex: -1, Line: 2},
		{ID: "B", Alias: "Bob", Kind: ParticipantKindParticipant, BoxIndex: -1, Line: 3},
		{ID: "C", Alias: "Carol", Kind: ParticipantKindActor, BoxIndex: -1, Line: 4},
		{ID: "D", Kind: ParticipantKindActor, BoxIndex: -1, Line: 5},
	}
	if len(d.Participants) != len(want) {
		t.Fatalf("got %d participants, want %d: %+v", len(d.Participants), len(want), d.Participants)
	}
	for i, w := range want {
		if !reflect.DeepEqual(d.Participants[i], w) {
			t.Errorf("participant[%d] = %+v, want %+v", i, d.Participants[i], w)
		}
	}
}

func TestParseAutoImpliedParticipants(t *testing.T) {
	input := `sequenceDiagram
    A->>B: hi
    C->>A: there`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	wantIDs := []string{"A", "B", "C"}
	if len(d.Participants) != len(wantIDs) {
		t.Fatalf("got %d participants, want %d", len(d.Participants), len(wantIDs))
	}
	for i, id := range wantIDs {
		if d.Participants[i].ID != id {
			t.Errorf("participant[%d].ID = %q, want %q", i, d.Participants[i].ID, id)
		}
		if d.Participants[i].Kind != ParticipantKindParticipant {
			t.Errorf("auto-registered %q should be ParticipantKindParticipant", id)
		}
	}
}

func TestParseExplicitDeclarationWinsOverImplicit(t *testing.T) {
	input := `sequenceDiagram
    A->>B: ping
    actor B as Bob`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(d.Participants) != 2 {
		t.Fatalf("got %d participants, want 2: %+v", len(d.Participants), d.Participants)
	}
	for _, p := range d.Participants {
		if p.ID == "B" {
			if p.Kind != ParticipantKindActor {
				t.Errorf("B kind = %v, want actor", p.Kind)
			}
			if p.Alias != "Bob" {
				t.Errorf("B alias = %q, want Bob", p.Alias)
			}
			return
		}
	}
	t.Error("participant B not found")
}

func TestParseAllArrowTypes(t *testing.T) {
	cases := []struct {
		src  string
		want ArrowType
	}{
		{"A->>B: x", ArrowTypeSolid},
		{"A->B: x", ArrowTypeSolidNoHead},
		{"A-->>B: x", ArrowTypeDashed},
		{"A-->B: x", ArrowTypeDashedNoHead},
		{"A-xB: x", ArrowTypeSolidCross},
		{"A--xB: x", ArrowTypeDashedCross},
		{"A-)B: x", ArrowTypeSolidOpen},
		{"A--)B: x", ArrowTypeDashedOpen},
		{"A<<->>B: x", ArrowTypeSolidBi},
		{"A<<-->>B: x", ArrowTypeDashedBi},
	}
	for _, tc := range cases {
		t.Run(tc.want.String(), func(t *testing.T) {
			input := "sequenceDiagram\n" + tc.src
			d, err := Parse(input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(d.Items) != 1 || d.Items[0].Message == nil {
				t.Fatalf("expected one message item, got %+v", d.Items)
			}
			m := d.Items[0].Message
			if m.ArrowType != tc.want {
				t.Errorf("ArrowType = %v, want %v", m.ArrowType, tc.want)
			}
			if m.From != "A" || m.To != "B" {
				t.Errorf("From/To = %q/%q, want A/B", m.From, m.To)
			}
			if m.Label != "x" {
				t.Errorf("Label = %q, want x", m.Label)
			}
		})
	}
}

func TestParseMessageLabelTrimmingAndColons(t *testing.T) {
	input := `sequenceDiagram
    A->>B:   hello: world  `
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Items[0].Message.Label != "hello: world" {
		t.Errorf("Label = %q, want %q", d.Items[0].Message.Label, "hello: world")
	}
}

func TestParseMessageLabelContainingArrow(t *testing.T) {
	input := `sequenceDiagram
    A->>B: send --> response`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m := d.Items[0].Message
	if m.From != "A" || m.To != "B" {
		t.Errorf("From/To = %q/%q, want A/B", m.From, m.To)
	}
	if m.ArrowType != ArrowTypeSolid {
		t.Errorf("ArrowType = %v, want solid", m.ArrowType)
	}
	if m.Label != "send --> response" {
		t.Errorf("Label = %q, want %q", m.Label, "send --> response")
	}
}

func TestParseMessageNoLabel(t *testing.T) {
	d, err := Parse("sequenceDiagram\n    A->>B:")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m := d.Items[0].Message
	if m.Label != "" {
		t.Errorf("Label = %q, want empty", m.Label)
	}
}

// Mermaid's grammar requires the colon; mmdc reports a parse error.
func TestParseMessageWithoutColonErrors(t *testing.T) {
	if _, err := Parse("sequenceDiagram\n    A->>B"); err == nil {
		t.Fatal("expected error for a message without ':'")
	}
}

func TestParseAutonumber(t *testing.T) {
	input := `sequenceDiagram
    autonumber
    A->>B: hi`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// A bare autonumber leaves start and step as they were (1 and 1).
	an := d.Items[0].AutoNumber
	if an == nil || *an != (AutoNumber{Visible: true}) {
		t.Errorf("item[0] = %+v, want a visible autonumber", d.Items[0])
	}
}

func TestParseAutonumberStartOnly(t *testing.T) {
	input := "sequenceDiagram\n    autonumber 10\n    A->>B: hi"
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	an := d.Items[0].AutoNumber
	if an == nil || *an != (AutoNumber{Start: 10, Step: 1, Visible: true}) {
		t.Errorf("item[0] = %+v, want autonumber 10 step 1", d.Items[0])
	}
}

func TestParseAutonumberStartAndStep(t *testing.T) {
	input := "sequenceDiagram\n    autonumber 10 5\n    A->>B: hi"
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	an := d.Items[0].AutoNumber
	if an == nil || *an != (AutoNumber{Start: 10, Step: 5, Visible: true}) {
		t.Errorf("item[0] = %+v, want autonumber 10 step 5", d.Items[0])
	}
}

func TestParseAutonumberInvalidNegative(t *testing.T) {
	_, err := Parse("sequenceDiagram\n    autonumber -1")
	if err == nil {
		t.Fatal("expected error for negative start")
	}
}

// Mermaid accepts zero; its renderer then keeps the previous start and step.
func TestParseAutonumberZeroAccepted(t *testing.T) {
	for _, src := range []string{"autonumber 0", "autonumber 5 0"} {
		if _, err := Parse("sequenceDiagram\n    " + src); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
}

func TestParseMessageOrderPreserved(t *testing.T) {
	input := `sequenceDiagram
    A->>B: 1
    B->>A: 2
    A->>B: 3`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(d.Items) != 3 {
		t.Fatalf("got %d items, want 3", len(d.Items))
	}
	for i, want := range []string{"1", "2", "3"} {
		if d.Items[i].Message.Label != want {
			t.Errorf("item[%d].Label = %q, want %q", i, d.Items[i].Message.Label, want)
		}
	}
}

func TestParseUnknownStatementErrors(t *testing.T) {
	input := `sequenceDiagram
    wiggle A B`
	_, err := Parse(input)
	if err == nil {
		t.Fatal("expected error for unknown statement")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should include line number: %v", err)
	}
}

func TestParseActivationMarkers(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want LifelineEffect
		to   string
	}{
		{"activate", "A->>+B: go", LifelineEffectActivate, "B"},
		{"deactivate", "activate A\nA->>-B: done", LifelineEffectDeactivate, "B"},
		{"no marker", "A->>B: plain", LifelineEffectNone, "B"},
		{"activate dashed", "A-->>+B: go", LifelineEffectActivate, "B"},
		{"activate with spaces", "A->> +B : go", LifelineEffectActivate, "B"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse("sequenceDiagram\n" + tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			m := d.Items[len(d.Items)-1].Message
			if m == nil {
				t.Fatalf("expected message, got %+v", d.Items)
			}
			if m.Lifeline != tc.want {
				t.Errorf("Lifeline = %v, want %v", m.Lifeline, tc.want)
			}
			if m.To != tc.to {
				t.Errorf("To = %q, want %q", m.To, tc.to)
			}
			// The +/- must not leak into the auto-registered participant ID.
			for _, p := range d.Participants {
				if strings.ContainsAny(p.ID, "+-") {
					t.Errorf("participant ID %q must not contain +/-", p.ID)
				}
			}
		})
	}
}

func TestParseStandaloneActivateDeactivate(t *testing.T) {
	input := `sequenceDiagram
    participant Client
    participant Server
    Client->>Server: Request
    activate Server
    Server-->>Client: Response
    deactivate Server`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(d.Items) != 4 {
		t.Fatalf("want 4 items, got %d: %+v", len(d.Items), d.Items)
	}
	a1 := d.Items[1].Activation
	if a1 == nil || a1.Participant != "Server" || !a1.Activate {
		t.Errorf("item[1] = %+v, want activate Server", d.Items[1])
	}
	a2 := d.Items[3].Activation
	if a2 == nil || a2.Participant != "Server" || a2.Activate {
		t.Errorf("item[3] = %+v, want deactivate Server", d.Items[3])
	}
}

func TestParseActivationMissingID(t *testing.T) {
	for _, kw := range []string{"activate", "deactivate"} {
		_, err := Parse("sequenceDiagram\n" + kw)
		if err == nil {
			t.Errorf("%s without id: expected error", kw)
		}
	}
}

func TestParseActivationAutoRegistersParticipant(t *testing.T) {
	d, err := Parse("sequenceDiagram\nactivate X")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(d.Participants) != 1 || d.Participants[0].ID != "X" {
		t.Errorf("expected participant X, got %+v", d.Participants)
	}
}

func TestParseTitleDirective(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"colon form", "sequenceDiagram\ntitle: My Diagram\nA->>B: hi", "My Diagram"},
		{"space form", "sequenceDiagram\ntitle My Diagram\nA->>B: hi", "My Diagram"},
		{"frontmatter", "---\ntitle: Frontmatter Title\n---\nsequenceDiagram\nA->>B: hi", "Frontmatter Title"},
		{"frontmatter quoted", "---\ntitle: \"Quoted Title\"\n---\nsequenceDiagram\nA->>B: hi", "Quoted Title"},
		{"no title", "sequenceDiagram\nA->>B: hi", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if d.Title != tc.want {
				t.Errorf("Title = %q, want %q", d.Title, tc.want)
			}
		})
	}
}

func TestParseAccTitleAccDescr(t *testing.T) {
	cases := []struct {
		name                 string
		src                  string
		wantTitle, wantDescr string
	}{
		{
			name:      "single-line both",
			src:       "sequenceDiagram\naccTitle: My Title\naccDescr: One-liner\nA->>B: hi",
			wantTitle: "My Title",
			wantDescr: "One-liner",
		},
		{
			name:      "multi-line accDescr block",
			src:       "sequenceDiagram\naccDescr {\n  first\n  second\n}\nA->>B: hi",
			wantTitle: "",
			wantDescr: "first\nsecond",
		},
		{
			name:      "neither",
			src:       "sequenceDiagram\nA->>B: hi",
			wantTitle: "",
			wantDescr: "",
		},
		{
			// Block content containing diagram keywords like "note" and "end"
			// must NOT be re-dispatched. Ensures the inAccDescrBlock branch
			// short-circuits at the top of parseLine.
			name:      "block content with reserved words",
			src:       "sequenceDiagram\naccDescr {\n  note about end-to-end flow\n  loop the request\n}\nA->>B: hi",
			wantTitle: "",
			wantDescr: "note about end-to-end flow\nloop the request",
		},
		{
			name:      "semicolon and hash are part of the value",
			src:       "sequenceDiagram\naccTitle: A; B # C\nA->>B: hi",
			wantTitle: "A; B # C",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if d.AccTitle != tc.wantTitle {
				t.Errorf("AccTitle = %q, want %q", d.AccTitle, tc.wantTitle)
			}
			if d.AccDescr != tc.wantDescr {
				t.Errorf("AccDescr = %q, want %q", d.AccDescr, tc.wantDescr)
			}
		})
	}
}

func TestParseUnclosedAccDescrBlock(t *testing.T) {
	src := "sequenceDiagram\naccDescr {\n  unterminated\nA->>B: hi"
	_, err := Parse(src)
	if err == nil {
		t.Fatal("expected error for unclosed accDescr block")
	}
	if !strings.Contains(err.Error(), "accDescr") {
		t.Errorf("error should mention accDescr: %v", err)
	}
}

func TestParseFrontmatterIgnoresUnknownKeys(t *testing.T) {
	src := `---
title: Hello
config:
  theme: dark
---
sequenceDiagram
A->>B: hi`
	d, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Title != "Hello" {
		t.Errorf("Title = %q, want Hello", d.Title)
	}
}

func TestParseNoteLeftRight(t *testing.T) {
	input := `sequenceDiagram
    Note left of Alice: first
    Note right of Bob: second`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(d.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(d.Items))
	}
	want := []struct {
		pos  NotePosition
		who  string
		text string
	}{
		{NotePositionLeft, "Alice", "first"},
		{NotePositionRight, "Bob", "second"},
	}
	for i, w := range want {
		n := d.Items[i].Note
		if n == nil {
			t.Fatalf("item[%d] is not a note: %+v", i, d.Items[i])
		}
		if n.Position != w.pos {
			t.Errorf("item[%d] position = %v, want %v", i, n.Position, w.pos)
		}
		if len(n.Participants) != 1 || n.Participants[0] != w.who {
			t.Errorf("item[%d] participants = %v, want [%s]", i, n.Participants, w.who)
		}
		if n.Text != w.text {
			t.Errorf("item[%d] text = %q, want %q", i, n.Text, w.text)
		}
	}
	// Notes auto-register their participants too.
	ids := make([]string, 0, len(d.Participants))
	for _, p := range d.Participants {
		ids = append(ids, p.ID)
	}
	if len(ids) != 2 || ids[0] != "Alice" || ids[1] != "Bob" {
		t.Errorf("participants = %v, want [Alice Bob]", ids)
	}
}

func TestParseNoteOverOneParticipant(t *testing.T) {
	d, err := Parse("sequenceDiagram\n    Note over X: hi")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	n := d.Items[0].Note
	if n.Position != NotePositionOver {
		t.Errorf("position = %v, want over", n.Position)
	}
	if len(n.Participants) != 1 || n.Participants[0] != "X" {
		t.Errorf("participants = %v, want [X]", n.Participants)
	}
}

func TestParseNoteOverTwoParticipants(t *testing.T) {
	d, err := Parse("sequenceDiagram\n    Note over Alice, Bob: between them")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	n := d.Items[0].Note
	if n.Position != NotePositionOver {
		t.Errorf("position = %v, want over", n.Position)
	}
	if len(n.Participants) != 2 || n.Participants[0] != "Alice" || n.Participants[1] != "Bob" {
		t.Errorf("participants = %v, want [Alice Bob]", n.Participants)
	}
	if n.Text != "between them" {
		t.Errorf("text = %q, want %q", n.Text, "between them")
	}
}

func TestParseNoteLowercaseKeyword(t *testing.T) {
	d, err := Parse("sequenceDiagram\n    note over A: lo")
	if err != nil {
		t.Fatalf("lowercase 'note' should be accepted: %v", err)
	}
	n := d.Items[0].Note
	if n.Position != NotePositionOver || n.Text != "lo" {
		t.Errorf("got %+v", n)
	}
}

func TestParseNoteMissingColonErrors(t *testing.T) {
	_, err := Parse("sequenceDiagram\n    Note over A")
	if err == nil {
		t.Fatal("expected error: note missing text")
	}
}

func TestParseNoteInvalidPositionErrors(t *testing.T) {
	_, err := Parse("sequenceDiagram\n    Note under A: nope")
	if err == nil {
		t.Fatal("expected error: unknown note position")
	}
}

func TestParseNoteOverTooManyParticipantsErrors(t *testing.T) {
	_, err := Parse("sequenceDiagram\n    Note over A, B, C: oops")
	if err == nil {
		t.Fatal("expected error: over accepts at most 2 participants")
	}
}

func TestParseNoteLeftRightWithTwoParticipantsErrors(t *testing.T) {
	// Only `Note over` accepts a comma pair; left/right are strictly single.
	_, err := Parse("sequenceDiagram\n    Note left of A, B: oops")
	if err == nil {
		t.Fatal("expected error: left/right notes take exactly one participant")
	}
}

func TestParseParticipantMissingID(t *testing.T) {
	_, err := Parse("sequenceDiagram\nparticipant")
	if err == nil {
		t.Error("expected error for bare participant")
	}
}

func TestParseMessageWithoutTargetIsUnrecognized(t *testing.T) {
	_, err := Parse("sequenceDiagram\nA->> : hi")
	if err == nil {
		t.Fatal("expected error — empty target")
	}
}

// Only whole-line %% comments are comments. Elsewhere Mermaid keeps the
// %% as text, which mmdc confirms: both strings below are drawn as written.
func TestParseCommentStripping(t *testing.T) {
	input := `sequenceDiagram
    %% a full-line comment
    A->>B: hi %% trailing comment
    participant C %% trailing on participant`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := d.Items[0].Message.Label; got != "hi %% trailing comment" {
		t.Errorf("Label = %q", got)
	}
	if _, ok := d.Participant("C %% trailing on participant"); !ok {
		t.Errorf("participants = %+v", d.Participants)
	}
}

func TestAccTitleNeedsColon(t *testing.T) {
	if _, err := Parse("sequenceDiagram\naccTitle Login Flow"); err == nil {
		t.Fatal("expected error: Mermaid requires 'accTitle:'")
	}
}

// --- Slice C: Block structure tests ---

func TestParseSimpleLoop(t *testing.T) {
	input := `sequenceDiagram
    loop every minute
        A->>B: ping
    end`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(d.Items) != 1 {
		t.Fatalf("want 1 top-level item, got %d", len(d.Items))
	}
	b := d.Items[0].Block
	if b == nil {
		t.Fatal("expected block item")
	}
	if b.Kind != BlockKindLoop {
		t.Errorf("Kind = %v, want loop", b.Kind)
	}
	if b.Label != "every minute" {
		t.Errorf("Label = %q, want %q", b.Label, "every minute")
	}
	if len(b.Items) != 1 || b.Items[0].Message == nil {
		t.Fatalf("want 1 message inside loop, got %+v", b.Items)
	}
	if b.Items[0].Message.Label != "ping" {
		t.Errorf("inner message label = %q, want ping", b.Items[0].Message.Label)
	}
}

func TestParseAltElse(t *testing.T) {
	input := `sequenceDiagram
    alt condition
        A->>B: yes
    else other
        A->>B: no
    end`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b := d.Items[0].Block
	if b.Kind != BlockKindAlt {
		t.Errorf("Kind = %v, want alt", b.Kind)
	}
	if b.Label != "condition" {
		t.Errorf("Label = %q", b.Label)
	}
	if len(b.Items) != 1 {
		t.Errorf("main branch should have 1 item, got %d", len(b.Items))
	}
	if len(b.Branches) != 1 {
		t.Fatalf("want 1 else branch, got %d", len(b.Branches))
	}
	eb := b.Branches[0]
	if eb.Label != "other" {
		t.Errorf("else label = %q", eb.Label)
	}
	if len(eb.Items) != 1 {
		t.Errorf("else branch should have 1 item, got %d", len(eb.Items))
	}
}

func TestParseAltMultipleElse(t *testing.T) {
	input := `sequenceDiagram
    alt a
        A->>B: 1
    else b
        A->>B: 2
    else c
        A->>B: 3
    end`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b := d.Items[0].Block
	if len(b.Branches) != 2 {
		t.Fatalf("want 2 else branches, got %d", len(b.Branches))
	}
	if b.Branches[0].Label != "b" || b.Branches[1].Label != "c" {
		t.Errorf("branch labels = %q, %q", b.Branches[0].Label, b.Branches[1].Label)
	}
}

func TestParseParAndCritical(t *testing.T) {
	cases := []struct {
		name     string
		openKw   string
		branchKw string
		want     BlockKind
	}{
		{"par/and", "par", "and", BlockKindPar},
		{"critical/option", "critical", "option", BlockKindCritical},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := "sequenceDiagram\n    " + tc.openKw + " first\n        A->>B: 1\n    " + tc.branchKw + " second\n        A->>B: 2\n    end"
			d, err := Parse(input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			b := d.Items[0].Block
			if b.Kind != tc.want {
				t.Errorf("Kind = %v, want %v", b.Kind, tc.want)
			}
			if len(b.Branches) != 1 {
				t.Fatalf("want 1 branch, got %d", len(b.Branches))
			}
		})
	}
}

func TestParseSingleBranchBlocks(t *testing.T) {
	for _, kw := range []string{"opt", "break", "loop"} {
		t.Run(kw, func(t *testing.T) {
			input := "sequenceDiagram\n    " + kw + " label\n        A->>B: x\n    end"
			d, err := Parse(input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			b := d.Items[0].Block
			if b.Label != "label" {
				t.Errorf("Label = %q", b.Label)
			}
			if len(b.Items) != 1 {
				t.Errorf("want 1 inner item, got %d", len(b.Items))
			}
			if len(b.Branches) != 0 {
				t.Errorf("single-branch block should have no branches, got %d", len(b.Branches))
			}
		})
	}
}

func TestParseNestedBlocks(t *testing.T) {
	input := `sequenceDiagram
    alt outer
        loop inner
            A->>B: deep
        end
    end`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	outer := d.Items[0].Block
	if outer.Kind != BlockKindAlt {
		t.Errorf("outer Kind = %v", outer.Kind)
	}
	if len(outer.Items) != 1 || outer.Items[0].Block == nil {
		t.Fatal("outer should contain one nested block")
	}
	inner := outer.Items[0].Block
	if inner.Kind != BlockKindLoop {
		t.Errorf("inner Kind = %v", inner.Kind)
	}
	if len(inner.Items) != 1 || inner.Items[0].Message == nil {
		t.Fatal("inner loop should contain one message")
	}
	if inner.Items[0].Message.Label != "deep" {
		t.Errorf("deep message label = %q", inner.Items[0].Message.Label)
	}
}

func TestParseBlockWithNotesAndMessages(t *testing.T) {
	input := `sequenceDiagram
    opt check
        A->>B: start
        Note over A: thinking
        B->>A: done
    end`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b := d.Items[0].Block
	if len(b.Items) != 3 {
		t.Fatalf("want 3 items (msg, note, msg), got %d", len(b.Items))
	}
	if b.Items[0].Message == nil || b.Items[1].Note == nil || b.Items[2].Message == nil {
		t.Errorf("wrong item types: %+v", b.Items)
	}
}

func TestParseEndWithoutBlockErrors(t *testing.T) {
	_, err := Parse("sequenceDiagram\n    end")
	if err == nil {
		t.Fatal("expected error for orphan end")
	}
}

func TestParseUnclosedBlockErrors(t *testing.T) {
	_, err := Parse("sequenceDiagram\n    loop forever\n        A->>B: go")
	if err == nil {
		t.Fatal("expected error for unclosed block")
	}
}

func TestParseElseWithoutAltErrors(t *testing.T) {
	_, err := Parse("sequenceDiagram\n    else oops")
	if err == nil {
		t.Fatal("expected error for else without alt")
	}
}

func TestParseBranchKeywordInWrongBlock(t *testing.T) {
	// 'and' only valid inside par, not alt.
	_, err := Parse("sequenceDiagram\n    alt x\n    and y\n    end")
	if err == nil {
		t.Fatal("expected error: 'and' inside alt")
	}
}

func TestParseEmptyBlock(t *testing.T) {
	d, err := Parse("sequenceDiagram\n    alt x\n    end")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b := d.Items[0].Block
	if len(b.Items) != 0 {
		t.Errorf("empty block should have no items, got %d", len(b.Items))
	}
}

func TestParseImmediateBranch(t *testing.T) {
	input := `sequenceDiagram
    alt x
    else y
        A->>B: z
    end`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b := d.Items[0].Block
	if len(b.Items) != 0 {
		t.Errorf("main branch should be empty, got %d items", len(b.Items))
	}
	if len(b.Branches) != 1 {
		t.Fatalf("want 1 else branch, got %d", len(b.Branches))
	}
	if len(b.Branches[0].Items) != 1 {
		t.Errorf("else branch should have 1 item, got %d", len(b.Branches[0].Items))
	}
}

func TestParseBlockNoLabel(t *testing.T) {
	d, err := Parse("sequenceDiagram\n    opt\n        A->>B: x\n    end")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Items[0].Block.Label != "" {
		t.Errorf("expected empty label, got %q", d.Items[0].Block.Label)
	}
}

func TestParseBidirectionalSolid(t *testing.T) {
	d, err := Parse("sequenceDiagram\n    A<<->>B: hi")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(d.Items) != 1 || d.Items[0].Message == nil {
		t.Fatalf("expected one message item, got %+v", d.Items)
	}
	m := d.Items[0].Message
	if m.ArrowType != ArrowTypeSolidBi {
		t.Errorf("ArrowType = %v, want %v", m.ArrowType, ArrowTypeSolidBi)
	}
	if m.From != "A" || m.To != "B" {
		t.Errorf("From/To = %q/%q, want A/B", m.From, m.To)
	}
	if m.Label != "hi" {
		t.Errorf("Label = %q, want hi", m.Label)
	}
}

func TestParseBidirectionalDashed(t *testing.T) {
	d, err := Parse("sequenceDiagram\n    A<<-->>B: hi")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(d.Items) != 1 || d.Items[0].Message == nil {
		t.Fatalf("expected one message item, got %+v", d.Items)
	}
	m := d.Items[0].Message
	if m.ArrowType != ArrowTypeDashedBi {
		t.Errorf("ArrowType = %v, want %v", m.ArrowType, ArrowTypeDashedBi)
	}
	if m.From != "A" || m.To != "B" {
		t.Errorf("From/To = %q/%q, want A/B", m.From, m.To)
	}
	if m.Label != "hi" {
		t.Errorf("Label = %q, want hi", m.Label)
	}
}

func TestParseRectWithRgb(t *testing.T) {
	input := "sequenceDiagram\n    rect rgb(220, 240, 255)\n        A->>B: hi\n    end"
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b := d.Items[0].Block
	if b.Fill != "rgb(220, 240, 255)" {
		t.Errorf("Fill = %q, want %q", b.Fill, "rgb(220, 240, 255)")
	}
	if b.Label != "" {
		t.Errorf("Label = %q, want empty", b.Label)
	}
}

func TestParseRectWithRgba(t *testing.T) {
	input := "sequenceDiagram\n    rect rgba(255, 220, 220, 0.6)\n        A->>B: hi\n    end"
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b := d.Items[0].Block
	if b.Fill != "rgba(255, 220, 220, 0.6)" {
		t.Errorf("Fill = %q, want %q", b.Fill, "rgba(255, 220, 220, 0.6)")
	}
}

// `#` starts a comment, so Mermaid sees `rect` with no colour; mmdc draws
// it unfilled.
func TestParseRectWithHexIsAComment(t *testing.T) {
	input := "sequenceDiagram\n    rect #ff0000\n        A->>B: hi\n    end"
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if b := d.Items[0].Block; b.Fill != "" {
		t.Errorf("Fill = %q, want empty", b.Fill)
	}
}

func TestParseRectWithoutColorKeepsFillEmpty(t *testing.T) {
	input := "sequenceDiagram\n    rect\n        A->>B: hi\n    end"
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b := d.Items[0].Block
	if b.Fill != "" {
		t.Errorf("Fill = %q, want empty", b.Fill)
	}
}

// Mermaid has no rect label: the whole rest of the line is the colour.
func TestParseRectKeepsWholeLineAsColor(t *testing.T) {
	input := "sequenceDiagram\n    rect rgb(220,240,255) my label\n        A->>B: hi\n    end"
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b := d.Items[0].Block
	if b.Fill != "rgb(220,240,255) my label" || b.Label != "" {
		t.Errorf("Fill, Label = %q, %q; want the whole line as Fill", b.Fill, b.Label)
	}
}

func TestParseBidirectionalSolidBeatsOneSided(t *testing.T) {
	d, err := Parse("sequenceDiagram\n    A<<->>B: sync")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m := d.Items[0].Message
	if m.ArrowType != ArrowTypeSolidBi {
		t.Errorf("<<->> should match before ->>, got %v", m.ArrowType)
	}
}

func TestParseBoxBasic(t *testing.T) {
	input := `sequenceDiagram
    box Frontend
        participant A
    end`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(d.Boxes) != 1 {
		t.Fatalf("want 1 box, got %d", len(d.Boxes))
	}
	bx := d.Boxes[0]
	// "Frontend" isn't a colour, so it is all label.
	if bx.Label != "Frontend" {
		t.Errorf("Label = %q, want %q", bx.Label, "Frontend")
	}
	if bx.Fill != "transparent" {
		t.Errorf("Fill = %q, want transparent", bx.Fill)
	}
	if len(bx.Members) != 1 || bx.Members[0] != "A" {
		t.Errorf("Members = %v, want [A]", bx.Members)
	}
	if bx.Line != 2 {
		t.Errorf("Line = %d, want 2", bx.Line)
	}
}

func TestParseBoxWithColor(t *testing.T) {
	cases := []struct{ src, fill, label string }{
		{"box rgb(220,240,255) Backend", "rgb(220,240,255)", "Backend"},
		{"box rgba(255,220,220,0.6) Pink", "rgba(255,220,220,0.6)", "Pink"},
		{"box Aqua Group A", "Aqua", "Group A"},
		{"box green", "green", ""},
		{"box Group 1", "transparent", "Group 1"},
		{"box", "transparent", ""},
		// A space before the parenthesis isn't valid CSS.
		{"box rgb (1,2,3) X", "transparent", "rgb (1,2,3) X"},
		// `#` starts a comment, so hex colours never reach the box.
		{"box #ff0000 Red", "transparent", ""},
	}
	for _, tc := range cases {
		d, err := Parse("sequenceDiagram\n" + tc.src + "\nparticipant A\nend")
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		bx := d.Boxes[0]
		if bx.Fill != tc.fill || bx.Label != tc.label {
			t.Errorf("%s: Fill, Label = %q, %q; want %q, %q", tc.src, bx.Fill, bx.Label, tc.fill, tc.label)
		}
	}
}

func TestParseBoxNestedRejected(t *testing.T) {
	input := `sequenceDiagram
    box Outer
        box Inner
        end
    end`
	_, err := Parse(input)
	if err == nil {
		t.Fatal("expected error for nested boxes")
	}
	if !strings.Contains(err.Error(), "nested") {
		t.Errorf("error should mention nested: %v", err)
	}
}

func TestParseBoxParticipantsTagged(t *testing.T) {
	input := `sequenceDiagram
    box Frontend
        participant A
        participant B as Bob
    end
    participant C`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(d.Boxes) != 1 {
		t.Fatalf("want 1 box, got %d", len(d.Boxes))
	}
	for _, p := range d.Participants {
		want := 0
		if p.ID == "C" {
			want = -1
		}
		if p.BoxIndex != want {
			t.Errorf("%s BoxIndex = %d, want %d", p.ID, p.BoxIndex, want)
		}
	}
}

// Mermaid's grammar only allows declarations inside a box.
func TestParseBoxRejectsMessages(t *testing.T) {
	input := `sequenceDiagram
    box Frontend
        A->>B: hi
    end`
	_, err := Parse(input)
	if err == nil {
		t.Fatal("expected error for a message inside a box")
	}
	if e, ok := err.(*Error); !ok || e.Line != 3 {
		t.Errorf("err = %v, want an *Error on line 3", err)
	}
}

func TestParseBoxParticipantInTwoBoxes(t *testing.T) {
	input := `sequenceDiagram
    box One
        participant A
    end
    box Two
        participant A
    end`
	if _, err := Parse(input); err == nil {
		t.Fatal("expected error for a participant in two boxes")
	}
}

// A participant first met outside any box joins the box that declares it.
func TestParseBoxAdoptsEarlierParticipant(t *testing.T) {
	input := `sequenceDiagram
    A->>B: hi
    box One
        participant A
    end`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if a, _ := d.Participant("A"); a.BoxIndex != 0 {
		t.Errorf("A BoxIndex = %d, want 0", a.BoxIndex)
	}
}

func TestParseUnclosedBox(t *testing.T) {
	if _, err := Parse("sequenceDiagram\nbox X\nparticipant A"); err == nil {
		t.Fatal("expected error for unclosed box")
	}
}

func TestParseCreateParticipant(t *testing.T) {
	input := `sequenceDiagram
    participant Manager
    create participant Worker
    Manager->>Worker: spawn`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	worker, ok := d.Participant("Worker")
	if !ok {
		t.Fatal("Worker participant not found")
	}
	if !worker.Created {
		t.Error("Worker should be marked created")
	}
	if len(d.Items) != 2 || d.Items[0].Create == nil || *d.Items[0].Create != "Worker" {
		t.Fatalf("expected create Worker then a message, got %+v", d.Items)
	}
	if d.Items[1].Message.To != "Worker" {
		t.Errorf("message To = %q, want Worker", d.Items[1].Message.To)
	}
}

func TestParseCreateActor(t *testing.T) {
	input := `sequenceDiagram
    participant System
    create actor User
    System->>User: welcome`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	user, ok := d.Participant("User")
	if !ok {
		t.Fatal("User participant not found")
	}
	if user.Kind != ParticipantKindActor || !user.Created {
		t.Errorf("User = %+v, want a created actor", user)
	}
}

func TestParseCreateWithAlias(t *testing.T) {
	input := `sequenceDiagram
    participant Manager
    create participant W as Worker
    Manager->>W: spawn`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	w, ok := d.Participant("W")
	if !ok {
		t.Fatal("W participant not found")
	}
	if w.Alias != "Worker" || !w.Created {
		t.Errorf("W = %+v, want created with alias Worker", w)
	}
}

func TestParseCreateNeedsItsMessage(t *testing.T) {
	input := `sequenceDiagram
    participant A
    create participant B
    A->>A: not to B`
	if _, err := Parse(input); err == nil {
		t.Fatal("expected error: the message after create must go to the created participant")
	}
}

func TestParseCreateExistingRejected(t *testing.T) {
	input := `sequenceDiagram
    A->>B: hi
    create participant B
    A->>B: again`
	if _, err := Parse(input); err == nil {
		t.Fatal("expected error for creating an existing participant")
	}
}

func TestParseDestroySetsItem(t *testing.T) {
	input := `sequenceDiagram
    participant A
    participant B
    A->>B: work
    destroy B
    A->>B: bye`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b, ok := d.Participant("B")
	if !ok {
		t.Fatal("B participant not found")
	}
	if !b.Destroyed {
		t.Error("B should be marked destroyed")
	}
	if len(d.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(d.Items))
	}
	if d.Items[1].Destroy == nil || *d.Items[1].Destroy != "B" || d.Items[1].Line != 5 {
		t.Errorf("second item should be destroy B on line 5, got %+v", d.Items[1])
	}
}

func TestParseDestroyNeedsItsMessage(t *testing.T) {
	input := `sequenceDiagram
    A->>B: hi
    destroy B
    A->>A: not about B`
	if _, err := Parse(input); err == nil {
		t.Fatal("expected error: the message after destroy must involve the destroyed participant")
	}
}

// Mermaid lets the message after `destroy` introduce the participant.
func TestParseDestroyUnknownAccepted(t *testing.T) {
	input := `sequenceDiagram
    destroy X
    A->>X: bye`
	d, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if x, ok := d.Participant("X"); !ok || !x.Destroyed {
		t.Errorf("X = %+v, want a destroyed participant", x)
	}
}
