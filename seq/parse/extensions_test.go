package parse

import (
	"strings"
	"testing"
)

func TestFrontmatterConfig(t *testing.T) {
	d := mustParse(t, `---
title: From frontmatter
config:
  theme: forest
  themeVariables:
    primaryColor: "#ff0000"
    fontSize: 14
  sequence:
    mirrorActors: false
    actorMargin: 80
    messageFontSize: 16px
    noteFontWeight: 600
---
sequenceDiagram
A->>B: hi`)
	c := d.Config
	if d.Title != "From frontmatter" || c.Theme != "forest" {
		t.Errorf("title, theme = %q, %q", d.Title, c.Theme)
	}
	if c.ThemeVariables["primaryColor"] != "#ff0000" || c.ThemeVariables["fontSize"] != "14" {
		t.Errorf("themeVariables = %v", c.ThemeVariables)
	}
	s := c.Sequence
	if s.MirrorActors == nil || *s.MirrorActors {
		t.Errorf("mirrorActors = %v", s.MirrorActors)
	}
	if s.ActorMargin == nil || *s.ActorMargin != 80 {
		t.Errorf("actorMargin = %v", s.ActorMargin)
	}
	if s.MessageFontSize == nil || *s.MessageFontSize != 16 {
		t.Errorf("messageFontSize = %v", s.MessageFontSize)
	}
	if s.NoteFontWeight == nil || *s.NoteFontWeight != "600" {
		t.Errorf("noteFontWeight = %v", s.NoteFontWeight)
	}
	if s.BoxMargin != nil {
		t.Errorf("unset keys stay nil, boxMargin = %v", *s.BoxMargin)
	}
	// Line numbers count the frontmatter.
	if d.Items[0].Line != 15 {
		t.Errorf("message line = %d, want 15", d.Items[0].Line)
	}
}

func TestDirectiveOverridesFrontmatter(t *testing.T) {
	d := mustParse(t, `---
config:
  theme: forest
  sequence:
    mirrorActors: false
    actorMargin: 80
---
%%{init: {"theme": "dark", "sequence": {"actorMargin": 20}}}%%
sequenceDiagram
A->>B: hi`)
	c := d.Config
	if c.Theme != "dark" {
		t.Errorf("theme = %q, want the directive's", c.Theme)
	}
	// Merged deeply: the directive's actorMargin, the frontmatter's mirrorActors.
	if *c.Sequence.ActorMargin != 20 || c.Sequence.MirrorActors == nil || *c.Sequence.MirrorActors {
		t.Errorf("sequence = %+v", c.Sequence)
	}
}

func TestSeveralInitDirectivesMerge(t *testing.T) {
	d := mustParse(t, "%%{init: {'theme': 'base'}}%%\n%%{initialize: {'sequence': {'showSequenceNumbers': true}}}%%\nsequenceDiagram\nA->>B: hi")
	if d.Config.Theme != "base" || d.Config.Sequence.ShowSequenceNumbers == nil || !*d.Config.Sequence.ShowSequenceNumbers {
		t.Errorf("config = %+v", d.Config)
	}
}

// Mermaid drops every init directive when one isn't valid JSON, and says
// nothing; here it's a warning.
func TestInvalidInitDirectiveIsIgnored(t *testing.T) {
	d := mustParse(t, "%%{init: {'theme': 'dark'}}%%\n%%{init: {theme: forest}}%%\nsequenceDiagram\nA->>B: hi")
	if d.Config.Theme != "" {
		t.Errorf("theme = %q, want none", d.Config.Theme)
	}
	if len(d.Warnings) != 1 || d.Warnings[0].Line != 2 {
		t.Errorf("warnings = %+v, want one on line 2", d.Warnings)
	}
}

func TestMultilineDirectiveKeepsLineNumbers(t *testing.T) {
	d := mustParse(t, "%%{\n  init: {\n    'theme': 'dark'\n  }\n}%%\nsequenceDiagram\nA->>B: hi")
	if d.Config.Theme != "dark" {
		t.Errorf("theme = %q", d.Config.Theme)
	}
	if d.Items[0].Line != 7 {
		t.Errorf("message line = %d, want 7", d.Items[0].Line)
	}
}

func TestTopLevelWrapWinsOverSequenceWrap(t *testing.T) {
	d := mustParse(t, "%%{init: {'wrap': false, 'sequence': {'wrap': true}}}%%\nsequenceDiagram\nA->>B: hi")
	if d.Items[0].Message.Wrap {
		t.Error("top-level wrap: false should win")
	}
}

func TestBadFrontmatterIsAnError(t *testing.T) {
	e := mustFail(t, "---\nconfig: [unclosed\n---\nsequenceDiagram")
	if e.Line != 1 {
		t.Errorf("line = %d, want 1", e.Line)
	}
}

func TestEntityCodes(t *testing.T) {
	d := mustParse(t, `sequenceDiagram
participant A as Alice#59; the first
A->>B: semi#59;colon #quot;quoted#quot; #9829; #amp; #nosuchentity;
Note over A: #35;hash`)
	if a, _ := d.Participant("A"); a.Label() != "Alice; the first" {
		t.Errorf("alias = %q", a.Label())
	}
	if got := d.Items[0].Message.Label; got != `semi;colon "quoted" ♥ & #nosuchentity;` {
		t.Errorf("label = %q", got)
	}
	if got := d.Items[1].Note.Text; got != "#hash" {
		t.Errorf("note = %q", got)
	}
}

func TestKeywordsAreCaseInsensitive(t *testing.T) {
	d := mustParse(t, `SEQUENCEDIAGRAM
PARTICIPANT A AS Alice
Actor B
ALT every minute
A->>B: ping
ELSE
END
NOTE Right Of B: hi
AutoNumber OFF`)
	if b, _ := d.Participant("B"); b.Kind != ParticipantKindActor {
		t.Errorf("B = %+v", b)
	}
	if len(d.Items) != 3 {
		t.Fatalf("items = %+v", d.Items)
	}
}

func TestKeywordNeedsWordBoundary(t *testing.T) {
	d := mustParse(t, "sequenceDiagram\nEndpoint->>Loopback: x\nNotes->>Participants: y")
	if got := participantIDs(d); strings.Join(got, ",") != "Endpoint,Loopback,Notes,Participants" {
		t.Errorf("participants = %v", got)
	}
}

func TestItemLines(t *testing.T) {
	d := mustParse(t, `sequenceDiagram
participant A
alt yes
  A->>B: one
else no
  Note over A: two
end
autonumber
activate A`)
	alt := d.Items[0].Block
	if d.Items[0].Line != 3 || alt.Line != 3 || alt.EndLine != 7 {
		t.Errorf("alt lines = %d %d %d", d.Items[0].Line, alt.Line, alt.EndLine)
	}
	if alt.Items[0].Line != 4 || alt.Branches[0].Line != 5 || alt.Branches[0].Items[0].Line != 6 {
		t.Errorf("inner lines = %d %d %d", alt.Items[0].Line, alt.Branches[0].Line, alt.Branches[0].Items[0].Line)
	}
	if d.Items[1].Line != 8 || d.Items[2].Line != 9 {
		t.Errorf("lines = %d %d", d.Items[1].Line, d.Items[2].Line)
	}
	if a, _ := d.Participant("A"); a.Line != 2 {
		t.Errorf("A declared on %d", a.Line)
	}
	if b, _ := d.Participant("B"); b.Line != 4 {
		t.Errorf("B first mentioned on %d", b.Line)
	}
}

func TestErrorLines(t *testing.T) {
	cases := []struct {
		src  string
		line int
	}{
		{"sequenceDiagram\nA->>B: ok\nwiggle", 3},
		{"sequenceDiagram\nloop x\nA->>B: ok", 2},
		{"---\ntitle: t\n---\nsequenceDiagram\nend", 5},
		{"sequenceDiagram\nA->>B: one; deactivate A", 2},
		{"not a diagram", 1},
	}
	for _, tc := range cases {
		if e := mustFail(t, tc.src); e.Line != tc.line {
			t.Errorf("%q: error %q on line %d, want %d", tc.src, e.Msg, e.Line, tc.line)
		}
	}
}

func TestCRLF(t *testing.T) {
	d := mustParse(t, "sequenceDiagram\r\nA->>B: hi\r\nB->>A: yo\r\n")
	if len(d.Items) != 2 || d.Items[1].Message.Label != "yo" {
		t.Errorf("items = %+v", d.Items)
	}
}

func TestHashInsideLabelEndsIt(t *testing.T) {
	d := mustParse(t, "sequenceDiagram\nA->>B: issue # 42\nparticipant C # a comment")
	if got := d.Items[0].Message.Label; got != "issue" {
		t.Errorf("label = %q", got)
	}
	if _, ok := d.Participant("C"); !ok {
		t.Errorf("participants = %v", participantIDs(d))
	}
}

func TestUnknownParticipantTypeWarns(t *testing.T) {
	d := mustParse(t, "sequenceDiagram\nparticipant X@{ \"type\": \"spaceship\" }")
	if d.Participants[0].Kind != ParticipantKindParticipant || len(d.Warnings) != 1 {
		t.Errorf("participant %+v, warnings %+v", d.Participants[0], d.Warnings)
	}
}

func TestBadLinksWarn(t *testing.T) {
	d := mustParse(t, "sequenceDiagram\nlinks A: not json\nlink A: no at sign")
	if len(d.Warnings) != 2 || d.Warnings[0].Line != 2 {
		t.Errorf("warnings = %+v", d.Warnings)
	}
}

func TestBranchInWrongBlockStaysAnError(t *testing.T) {
	mustFail(t, "sequenceDiagram\nloop x\nelse y\nend")
	mustFail(t, "sequenceDiagram\npar_over x\noption y\nend")
	mustParse(t, "sequenceDiagram\npar_over x\nA->>B: 1\nand y\nA->>B: 2\nend")
}

// Checked against mmdc, which reports a parse error: `-x` ends the actor
// name, leaving a second arrow.
func TestTwoArrowsInOneStatement(t *testing.T) {
	mustFail(t, "sequenceDiagram\nA-xavier->>B: x")
}
