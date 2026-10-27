package parse

// Cases ported from Mermaid 12.0.0's sequenceDiagram.spec.js (see
// UPSTREAM). Mermaid's tests look at its flat message list; these check the
// same facts in the tree.

import (
	"reflect"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *Diagram {
	t.Helper()
	d, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return d
}

func mustFail(t *testing.T, src string) *Error {
	t.Helper()
	_, err := Parse(src)
	if err == nil {
		t.Fatalf("expected a parse error for:\n%s", src)
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("error %v is %T, want *Error", err, err)
	}
	return e
}

func participantIDs(d *Diagram) []string {
	var ids []string
	for _, p := range d.Participants {
		ids = append(ids, p.ID)
	}
	return ids
}

// signal is one entry of a flattened diagram, close to Mermaid's message list.
type signal struct {
	kind, from, to, text string
}

// flatten lists the diagram the way Mermaid's db.getMessages() does:
// block starts, branches and ends in line with messages and notes, and
// activations from +/- as separate entries.
func flatten(items []Item) []signal {
	var out []signal
	for _, it := range items {
		switch {
		case it.Message != nil:
			m := it.Message
			out = append(out, signal{m.ArrowType.String(), m.From, m.To, m.Label})
			switch m.Lifeline {
			case LifelineEffectActivate:
				out = append(out, signal{"activate", m.To, "", ""})
			case LifelineEffectDeactivate:
				out = append(out, signal{"deactivate", m.From, "", ""})
			}
		case it.Note != nil:
			n := it.Note
			to := n.Participants[len(n.Participants)-1]
			out = append(out, signal{"note", n.Participants[0], to, n.Text})
		case it.Activation != nil:
			kind := "deactivate"
			if it.Activation.Activate {
				kind = "activate"
			}
			out = append(out, signal{kind, it.Activation.Participant, "", ""})
		case it.Block != nil:
			b := it.Block
			text := b.Label
			if b.Kind == BlockKindRect {
				text = b.Fill
			}
			out = append(out, signal{b.Kind.String() + "-start", "", "", text})
			out = append(out, flatten(b.Items)...)
			for _, br := range b.Branches {
				out = append(out, signal{b.Kind.String() + "-branch", "", "", br.Label})
				out = append(out, flatten(br.Items)...)
			}
			out = append(out, signal{b.Kind.String() + "-end", "", "", ""})
		case it.AutoNumber != nil:
			out = append(out, signal{"autonumber", "", "", ""})
		}
	}
	return out
}

const aliceBob = `
sequenceDiagram
Alice->Bob:Hello Bob, how are you?
Note right of Bob: Bob thinks
Bob-->Alice: I am good thanks!`

func TestMermaidBasic(t *testing.T) {
	d := mustParse(t, aliceBob)
	if a, _ := d.Participant("Alice"); a.Label() != "Alice" {
		t.Errorf("Alice label = %q", a.Label())
	}
	s := flatten(d.Items)
	if len(s) != 3 || s[0].from != "Alice" || s[2].from != "Bob" {
		t.Errorf("signals = %+v", s)
	}
}

func TestMermaidAutonumberDecimals(t *testing.T) {
	d := mustParse(t, "sequenceDiagram\nautonumber 10.1 .01\nAlice->Bob:Hello")
	if an := d.Items[0].AutoNumber; an.Start != 10.1 || an.Step != 0.01 {
		t.Errorf("autonumber = %+v", an)
	}
	mustFail(t, "sequenceDiagram\nautonumber 10.001\nAlice->Bob:Hello")
}

func TestMermaidAutonumberOff(t *testing.T) {
	d := mustParse(t, "sequenceDiagram\nautonumber\nA->>B: one\nautonumber off\nA->>B: two")
	if an := d.Items[2].AutoNumber; an == nil || an.Visible {
		t.Errorf("item[2] = %+v, want autonumber off", d.Items[2])
	}
}

func TestMermaidTitles(t *testing.T) {
	for _, src := range []string{"title: Diagram Title", "title Diagram Title"} {
		d := mustParse(t, "sequenceDiagram\n"+src+"\nAlice->Bob:Hello")
		if d.Title != "Diagram Title" {
			t.Errorf("%s: title = %q", src, d.Title)
		}
	}
}

func TestMermaidAccessibility(t *testing.T) {
	d := mustParse(t, `
sequenceDiagram
title: Diagram Title
accTitle: This is the title
accDescr: Accessibility Description
Alice->Bob:Hello Bob, how are you?
`)
	if d.AccTitle != "This is the title" || d.AccDescr != "Accessibility Description" {
		t.Errorf("acc = %q / %q", d.AccTitle, d.AccDescr)
	}
	d = mustParse(t, `
sequenceDiagram
accTitle: This is the title
accDescr {
Accessibility
Description
}
Alice->Bob:Hello Bob, how are you?
`)
	if d.AccDescr != "Accessibility\nDescription" {
		t.Errorf("multiline accDescr = %q", d.AccDescr)
	}
}

func TestMermaidActorNames(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"Alice->Bob:Hello Bob, how are - you?\nBob-->Alice: I am good thanks!", []string{"Alice", "Bob"}},
		{"Alice-in-Wonderland->Bob:Hello Bob, how are - you?\nBob-->Alice-in-Wonderland:I am good thanks!", []string{"Alice-in-Wonderland", "Bob"}},
		{"participant Alice=Wonderland\nparticipant Bob\nAlice=Wonderland->Bob:Hello\nBob-->Alice=Wonderland:Hi", []string{"Alice=Wonderland", "Bob"}},
		{"participant Alice\nparticipant Alice\nparticipant Alice", []string{"Alice"}},
	}
	for _, tc := range cases {
		d := mustParse(t, "sequenceDiagram\n"+tc.src)
		if got := participantIDs(d); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: participants = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestMermaidAliases(t *testing.T) {
	d := mustParse(t, `
sequenceDiagram
  actor Alice as Alice2
  actor Bob
  participant John as John2
  participant Mandy
  Alice->>Bob: Hi Bob
  Bob->>Alice: Hi Alice
  Alice->>John: Hi John
  John->>Mandy: Hi Mandy
  Mandy ->>Joan: Hi Joan`)
	if got := participantIDs(d); !reflect.DeepEqual(got, []string{"Alice", "Bob", "John", "Mandy", "Joan"}) {
		t.Errorf("participants = %v", got)
	}
	alice, _ := d.Participant("Alice")
	john, _ := d.Participant("John")
	joan, _ := d.Participant("Joan")
	if alice.Label() != "Alice2" || alice.Kind != ParticipantKindActor {
		t.Errorf("Alice = %+v", alice)
	}
	if john.Label() != "John2" || john.Kind != ParticipantKindParticipant || joan.Kind != ParticipantKindParticipant {
		t.Errorf("John, Joan = %+v, %+v", john, joan)
	}
	if m := d.Items[4].Message; m.To != "Joan" {
		t.Errorf("last message to %q", m.To)
	}
}

func TestMermaidArrowTypes(t *testing.T) {
	cases := []struct {
		arrow    string
		want     ArrowType
		lineType int
	}{
		{"-x", ArrowTypeSolidCross, 3},
		{"--x", ArrowTypeDashedCross, 4},
		{"-)", ArrowTypeSolidOpen, 24},
		{"--)", ArrowTypeDashedOpen, 25},
		{"->>", ArrowTypeSolid, 0},
		{"-->>", ArrowTypeDashed, 1},
		{"<<->>", ArrowTypeSolidBi, 33},
		{"<<-->>", ArrowTypeDashedBi, 34},
		{"->", ArrowTypeSolidNoHead, 5},
		{"-->", ArrowTypeDashedNoHead, 6},
	}
	for _, tc := range cases {
		d := mustParse(t, "sequenceDiagram\nAlice"+tc.arrow+"Bob:Hello Bob, how are you?")
		m := d.Items[0].Message
		if m.ArrowType != tc.want || m.ArrowType.MermaidLineType() != tc.lineType {
			t.Errorf("%s: %v (LINETYPE %d), want %v (%d)", tc.arrow, m.ArrowType, m.ArrowType.MermaidLineType(), tc.want, tc.lineType)
		}
		if m.From != "Alice" || m.To != "Bob" {
			t.Errorf("%s: %s -> %s", tc.arrow, m.From, m.To)
		}
	}
}

// "1 should parse": the half arrows, including the reverse forms written
// before the target.
func TestMermaidHalfArrows(t *testing.T) {
	cases := []struct {
		line string
		want ArrowType
	}{
		{`Bob -|\ Alice: Hello`, ArrowTypeSolidTop},
		{`Bob -|/ Alice: Hello`, ArrowTypeSolidBottom},
		{`Bob -// Alice: Hello`, ArrowTypeStickBottom},
		{`Bob -\\ Alice: Hello`, ArrowTypeStickTop},
		{`Bob \|- Alice: Hello`, ArrowTypeSolidBottomReverse},
		{`Bob /|- Alice: Hello`, ArrowTypeSolidTopReverse},
		{`Bob //- Alice: Hello`, ArrowTypeStickTopReverse},
		{`Bob \\- Alice: Hello`, ArrowTypeStickBottomReverse},
		{`Bob --|\ Alice: Hello`, ArrowTypeSolidTopDashed},
		{`Bob \\-- Alice: Hello`, ArrowTypeStickBottomReverseDashed},
	}
	for _, tc := range cases {
		d := mustParse(t, "sequenceDiagram\nactor Bob\nactor Alice\n"+tc.line)
		m := d.Items[0].Message
		if m.ArrowType != tc.want || m.From != "Bob" || m.To != "Alice" {
			t.Errorf("%s: %v %s -> %s, want %v Bob -> Alice", tc.line, m.ArrowType, m.From, m.To, tc.want)
		}
	}
}

func TestMermaidCentralConnections(t *testing.T) {
	cases := []struct {
		line  string
		arrow ArrowType
		want  CentralConnection
	}{
		{"Alice ()->>() Bob: Hello", ArrowTypeSolid, CentralBoth},
		{"Alice ()-->>() Bob: Hello", ArrowTypeDashed, CentralBoth},
		{"Alice ->>() Bob: Hello", ArrowTypeSolid, CentralTarget},
		{"Alice ()-->> Bob: Hello", ArrowTypeDashed, CentralSource},
		{"Alice ()->> Bob: Hello", ArrowTypeSolid, CentralSource},
		{"Alice ()<<-->>() Bob: Hello", ArrowTypeDashedBi, CentralBoth},
		{"Alice ()<<->>() Bob: hey?", ArrowTypeSolidBi, CentralBoth},
		{"Alice ->> Bob: Hello", ArrowTypeSolid, CentralNone},
	}
	for _, tc := range cases {
		d := mustParse(t, "sequenceDiagram\nparticipant Alice\nparticipant Bob\n"+tc.line)
		m := d.Items[0].Message
		if m.ArrowType != tc.arrow || m.Central != tc.want || m.From != "Alice" || m.To != "Bob" {
			t.Errorf("%s: got %+v", tc.line, m)
		}
		if m.Label != "Hello" && m.Label != "hey?" {
			t.Errorf("%s: label %q", tc.line, m.Label)
		}
	}
}

func TestMermaidActivations(t *testing.T) {
	d := mustParse(t, `
sequenceDiagram
Alice-->>Bob:Hello Bob, how are you?
activate Bob
Bob-->>Alice:Hello Alice, I'm fine and  you?
deactivate Bob`)
	want := []signal{
		{"dashed", "Alice", "Bob", "Hello Bob, how are you?"},
		{"activate", "Bob", "", ""},
		{"dashed", "Bob", "Alice", "Hello Alice, I'm fine and  you?"},
		{"deactivate", "Bob", "", ""},
	}
	if got := flatten(d.Items); !reflect.DeepEqual(got, want) {
		t.Errorf("signals = %+v", got)
	}

	d = mustParse(t, `
      sequenceDiagram
      Alice-->>+Bob:Hello Bob, how are you?
      Bob-->>+Carol:Carol, let me introduce Alice?
      Bob-->>- Alice:Hello Alice, please meet Carol?
      Carol->>- Bob:Oh Bob, I'm so happy to be here!`)
	s := flatten(d.Items)
	if len(s) != 8 || s[1] != (signal{"activate", "Bob", "", ""}) || s[3] != (signal{"activate", "Carol", "", ""}) ||
		s[5] != (signal{"deactivate", "Bob", "", ""}) || s[7] != (signal{"deactivate", "Carol", "", ""}) {
		t.Errorf("stacked activations = %+v", s)
	}
}

func TestMermaidDeactivateInactiveFails(t *testing.T) {
	e := mustFail(t, `
      sequenceDiagram
      participant user as End User
      participant Server as Server
      participant System as System
      participant System2 as System2

      user->>+Server: Test
      user->>+Server: Test2
      user->>System: Test
      Server->>-user: Test
      Server->>-user: Test2

      %% The following deactivation of Server will fail
      Server->>-user: Test3`)
	if e.Line != 15 {
		t.Errorf("error line = %d, want 15", e.Line)
	}
}

func TestMermaidCommentsAndBlankLines(t *testing.T) {
	for _, src := range []string{
		"\nsequenceDiagram\nAlice->Bob: Hello Bob, how are you?\n%% Comment\nNote right of Bob: Bob thinks\nBob-->Alice: I am good thanks!",
		"\n      sequenceDiagram\n      Alice->Bob: Hello Bob, how are you?\n\n      %% Comment\n      Note right of Bob: Bob thinks\n      Bob-->Alice: I am good thanks!\n      ",
		"\nsequenceDiagram\n Alice->Bob: Hello Bob, how are you?\n\n%% Comment\nNote right of Bob: Bob thinks\nBob-->Alice: I am good thanks!",
	} {
		s := flatten(mustParse(t, src).Items)
		if len(s) != 3 || s[0].from != "Alice" || s[2].from != "Bob" {
			t.Errorf("signals = %+v", s)
		}
	}
}

func TestMermaidSemicolons(t *testing.T) {
	d := mustParse(t, "\nsequenceDiagram;Alice->Bob: Hello Bob, how are you?;Note right of Bob: Bob thinks;Bob-->Alice: I am good thanks!;")
	s := flatten(d.Items)
	if len(s) != 3 || s[0].from != "Alice" || s[1].kind != "note" || s[2].from != "Bob" {
		t.Errorf("signals = %+v", s)
	}
	for _, it := range d.Items {
		if it.Line != 2 {
			t.Errorf("item on line %d, want 2", it.Line)
		}
	}
}

func TestMermaidLeadingSpacesAndLoop(t *testing.T) {
	d := mustParse(t, `
sequenceDiagram
participant Alice
participant Bob
Alice->John: Hello John, how are you?
    loop Healthcheck
John->John: Fight against hypochondria
 end
Note right of John: Rational thoughts<br/>prevail...
    John-->Alice: Great!
    John->Bob: How about you?
Bob-->John: Jolly good!`)
	s := flatten(d.Items)
	if len(s) != 8 || s[0].from != "Alice" || s[2].from != "John" {
		t.Errorf("signals = %+v", s)
	}
	if s[4].text != "Rational thoughts\nprevail..." {
		t.Errorf("note text = %q", s[4].text)
	}
}

// Mermaid keeps <br> in its model and splits at draw time; here they are
// newlines already.
func TestMermaidLineBreaks(t *testing.T) {
	d := mustParse(t, "\nsequenceDiagram\n"+
		"participant 1 as multiline<br>text\n"+
		"participant 2 as multiline<br/>text\n"+
		"participant 3 as multiline<br />text\n"+
		"participant 4 as multiline<br \t/>text\n"+
		"1->>2: multiline<br>text\n"+
		"note right of 2: multiline<br>text\n"+
		"2->>3: multiline<br/>text\n"+
		"note right of 3: multiline<BR/>text\n"+
		"3->>4: multiline</br>text\n")
	for _, p := range d.Participants {
		if p.Label() != "multiline\ntext" {
			t.Errorf("participant %s label = %q", p.ID, p.Label())
		}
	}
	for _, s := range flatten(d.Items) {
		if s.text != "multiline\ntext" {
			t.Errorf("%s text = %q", s.kind, s.text)
		}
	}
}

func TestMermaidWrap(t *testing.T) {
	d := mustParse(t, `
sequenceDiagram
participant 1
participant 2
participant 3
1->>2: single-line text
note right of 2: single-line text
2->>3:nowrap: single-line text
note right of 3:nowrap: single-line text
1->>2:wrap: single-line text
note right of 2:wrap: multiline<br/>text
`)
	want := []struct {
		text string
		wrap bool
	}{
		{"single-line text", false}, {"single-line text", false},
		{"single-line text", false}, {"single-line text", false},
		{"single-line text", true}, {"multiline\ntext", true},
	}
	for i, it := range d.Items {
		text, wrap := "", false
		if it.Message != nil {
			text, wrap = it.Message.Label, it.Message.Wrap
		} else {
			text, wrap = it.Note.Text, it.Note.Wrap
		}
		if text != want[i].text || wrap != want[i].wrap {
			t.Errorf("item %d = %q wrap=%v, want %q wrap=%v", i, text, wrap, want[i].text, want[i].wrap)
		}
	}
}

func TestMermaidWrapDirective(t *testing.T) {
	d := mustParse(t, `
%%{init: { 'theme': 'dark' } }%%
sequenceDiagram
%%{wrap}%%
Alice->>Bob: Hello Bob, how are you? If you are not available right now, I can leave you a message.
Note left of Alice: Bob thinks
Bob->>Alice:nowrap: Fine!`)
	if d.Config.Theme != "dark" {
		t.Errorf("theme = %q", d.Config.Theme)
	}
	if !d.Items[0].Message.Wrap || !d.Items[1].Note.Wrap || d.Items[2].Message.Wrap {
		t.Errorf("wrap flags = %v %v %v, want true true false",
			d.Items[0].Message.Wrap, d.Items[1].Note.Wrap, d.Items[2].Message.Wrap)
	}
	if a, _ := d.Participant("Alice"); !a.Wrap {
		t.Error("participants should wrap too")
	}
	if d.Items[0].Line != 5 {
		t.Errorf("first message on line %d, want 5", d.Items[0].Line)
	}
}

// The init directive's `config` key becomes the sequence config.
func TestMermaidInitConfigKey(t *testing.T) {
	d := mustParse(t, `
    %%{init: { "theme": "dark", 'config': { "fontFamily": "Menlo", "fontSize": 18, "messageFontWeight": 400, "wrap": true }}}%%
sequenceDiagram
Alice->>Bob: Hello Bob, how are you?
Bob->>Alice: Fine!`)
	c := d.Config
	if c.Theme != "dark" {
		t.Errorf("theme = %q", c.Theme)
	}
	s := c.Sequence
	if s.FontFamily == nil || *s.FontFamily != "Menlo" || s.FontSize == nil || *s.FontSize != 18 ||
		s.MessageFontWeight == nil || *s.MessageFontWeight != "400" || s.Wrap == nil || !*s.Wrap {
		t.Errorf("sequence config = %+v", s)
	}
	if !d.Items[0].Message.Wrap {
		t.Error("sequence.wrap should make messages wrap")
	}
}

func TestMermaidNotes(t *testing.T) {
	d := mustParse(t, "sequenceDiagram\nAlice->Bob: Hello Bob, how are you?\nNote over Bob: Bob thinks")
	if s := flatten(d.Items)[1]; s.from != "Bob" || s.to != "Bob" {
		t.Errorf("note over one = %+v", s)
	}
	d = mustParse(t, "sequenceDiagram\nAlice->Bob: Hello\nNote over Alice,Bob: confusion\nNote over Bob,Alice: resolution")
	s := flatten(d.Items)
	if s[1].from != "Alice" || s[1].to != "Bob" || s[2].from != "Bob" || s[2].to != "Alice" {
		t.Errorf("notes over two = %+v", s)
	}
}

func TestMermaidBlocks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{"loop", "Alice->Bob: Hello\n%% Comment\nNote right of Bob: Bob thinks\nloop Multiple happy responses\nBob-->Alice: I am good thanks!\nend",
			[]string{"solid-no-head", "note", "loop-start", "dashed-no-head", "loop-end"}},
		{"rect", "Alice->Bob: Hello\nrect rgb(200, 255, 200)\nNote right of Bob: Bob thinks\nBob-->Alice: I am good thanks\nend",
			[]string{"solid-no-head", "rect-start", "note", "dashed-no-head", "rect-end"}},
		{"nested rect", "Alice->Bob: Hello\nrect rgb(200, 255, 200)\nrect rgb(0, 0, 0)\nNote right of Bob: Bob thinks\nend\nBob-->Alice: I am good thanks\nend",
			[]string{"solid-no-head", "rect-start", "rect-start", "note", "rect-end", "dashed-no-head", "rect-end"}},
		{"alt", "Alice->Bob: Hello\nNote right of Bob: Bob thinks\nalt isWell\nBob-->Alice: I am good thanks!\nelse isSick\nBob-->Alice: Feel sick...\nelse default\nBob-->Alice: :-)\nend",
			[]string{"solid-no-head", "note", "alt-start", "dashed-no-head", "alt-branch", "dashed-no-head", "alt-branch", "dashed-no-head", "alt-end"}},
		{"critical", "critical Establish a connection to the DB\nService-->DB: connect\noption Network timeout\nService-->Service: Log error\noption Credentials rejected\nService-->Service: Log different error\nend",
			[]string{"critical-start", "dashed-no-head", "critical-branch", "dashed-no-head", "critical-branch", "dashed-no-head", "critical-end"}},
		{"break", "Consumer-->API: Book something\nAPI-->BookingService: Start booking process\nbreak when the booking process fails\nAPI-->Consumer: show failure\nend\nAPI-->BillingService: Start billing process",
			[]string{"dashed-no-head", "dashed-no-head", "break-start", "dashed-no-head", "break-end", "dashed-no-head"}},
		{"par", "par Parallel one\nAlice->>Bob: Hello\nBob-->>Alice: I am good!\nand Parallel two\nAlice->>Bob: Are you OK?\nBob-->>Alice: Fine!\nand Parallel three\nAlice->>Bob: What do you think?\nBob-->>Alice: It's good!\nend",
			[]string{"par-start", "solid", "dashed", "par-branch", "solid", "dashed", "par-branch", "solid", "dashed", "par-end"}},
		{"par_over", "par_over Parallel overlap\nAlice ->> Bob: Message\nNote left of Alice: Alice note\nNote right of Bob: Bob note\nend",
			[]string{"par_over-start", "solid", "note", "note", "par_over-end"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var kinds []string
			for _, s := range flatten(mustParse(t, "sequenceDiagram\n"+tc.src).Items) {
				kinds = append(kinds, s.kind)
			}
			if !reflect.DeepEqual(kinds, tc.want) {
				t.Errorf("signals = %v\nwant      %v", kinds, tc.want)
			}
		})
	}
}

func TestMermaidRectColor(t *testing.T) {
	d := mustParse(t, "sequenceDiagram\nAlice->Bob: Hello\nrect rgb(200, 255, 200)\nBob-->Alice: Fine\nend")
	if s := flatten(d.Items)[1]; s.text != "rgb(200, 255, 200)" {
		t.Errorf("rect colour = %q", s.text)
	}
}

func TestMermaidSpecialCharacters(t *testing.T) {
	cases := []struct {
		src   string
		index int
		want  string
	}{
		{"Alice->Bob: -:<>,;# comment", 0, "-:<>,"},
		{"Alice->Bob: Hello\nNote right of Bob: -:<>,;# comment", 1, "-:<>,"},
		{"Alice->Bob: Hello\nloop -:<>,;# comment\nBob-->Alice: Fine\nend", 1, "-:<>,"},
		{"Alice->Bob: Hello\nopt -:<>,;# comment\nBob-->Alice: Fine\nend", 1, "-:<>,"},
		{"Alice->Bob: Hello\nalt -:<>,;# comment\nBob-->Alice: Fine\nelse ,<>:-#; comment\nBob-->Alice: Fine\nend", 3, ",<>:-"},
		{"Alice->Bob: Hello\npar -:<>,;# comment\nBob-->Alice: Fine\nand ,<>:-#; comment\nBob-->Alice: Fine\nend", 3, ",<>:-"},
	}
	for _, tc := range cases {
		s := flatten(mustParse(t, "sequenceDiagram\n"+tc.src).Items)
		if s[tc.index].text != tc.want {
			t.Errorf("%q: signal %d text = %q, want %q", tc.src, tc.index, s[tc.index].text, tc.want)
		}
	}
}

func TestMermaidNoLabelBlocks(t *testing.T) {
	cases := []string{
		"loop\nBob-->Alice: I am good thanks!\nend",
		"opt # comment\nBob-->Alice: I am good thanks!\nend",
		"alt;Bob-->Alice: I am good thanks!\nelse # comment\nBob-->Alice: I am good thanks!\nend",
		"par;Bob-->Alice: I am good thanks!\nand # comment\nBob-->Alice: I am good thanks!\nend",
	}
	for _, src := range cases {
		s := flatten(mustParse(t, "sequenceDiagram\nAlice->Bob: Hello\n"+src).Items)
		if s[1].text != "" || s[2].text != "I am good thanks!" {
			t.Errorf("%q: %+v", src, s)
		}
		if strings.HasPrefix(src, "alt") || strings.HasPrefix(src, "par") {
			if s[3].text != "" || s[4].text != "I am good thanks!" {
				t.Errorf("%q branch: %+v", src, s)
			}
		}
	}
}

func TestMermaidLinks(t *testing.T) {
	d := mustParse(t, `
sequenceDiagram
participant a as Alice
participant b as Bob
participant c as Charlie
links a: { "Repo": "https://repo.contoso.com/", "Dashboard": "https://dashboard.contoso.com/" }
links b: { "Dashboard": "https://dashboard.contoso.com/" }
links a: { "On-Call": "https://oncall.contoso.com/?svc=alice" }
link a: Endpoint @ https://alice.contoso.com
link a: Swagger @ https://swagger.contoso.com
link a: Tests @ https://tests.contoso.com/?svc=alice@contoso.com
`)
	a, _ := d.Participant("a")
	b, _ := d.Participant("b")
	c, _ := d.Participant("c")
	wantA := []Link{
		{"Repo", "https://repo.contoso.com/"},
		{"Dashboard", "https://dashboard.contoso.com/"},
		{"On-Call", "https://oncall.contoso.com/?svc=alice"},
		{"Endpoint", "https://alice.contoso.com"},
		{"Swagger", "https://swagger.contoso.com"},
		{"Tests", "https://tests.contoso.com/?svc=alice@contoso.com"},
	}
	if !reflect.DeepEqual(a.Links, wantA) {
		t.Errorf("a links = %+v", a.Links)
	}
	if !reflect.DeepEqual(b.Links, []Link{{"Dashboard", "https://dashboard.contoso.com/"}}) {
		t.Errorf("b links = %+v", b.Links)
	}
	if c.Links != nil {
		t.Errorf("c links = %+v", c.Links)
	}
}

func TestMermaidProperties(t *testing.T) {
	d := mustParse(t, `
sequenceDiagram
participant a as Alice
participant b as Bob
participant c as Charlie
properties a: {"class": "internal-service-actor", "icon": "@clock"}
properties b: {"class": "external-service-actor", "icon": "@computer"}
`)
	a, _ := d.Participant("a")
	b, _ := d.Participant("b")
	c, _ := d.Participant("c")
	if a.Properties["class"] != "internal-service-actor" || a.Properties["icon"] != "@clock" ||
		b.Properties["class"] != "external-service-actor" || c.Properties != nil {
		t.Errorf("properties = %v / %v / %v", a.Properties, b.Properties, c.Properties)
	}
}

func TestMermaidBoxes(t *testing.T) {
	cases := []struct {
		line, label, fill string
	}{
		{"box green Group 1", "Group 1", "green"},
		{"box Group 1", "Group 1", "transparent"},
		{"box aqua", "", "aqua"},
	}
	for _, tc := range cases {
		d := mustParse(t, "sequenceDiagram\n"+tc.line+"\nparticipant a as Alice\nparticipant b as Bob\nend\nparticipant c as Charlie")
		bx := d.Boxes[0]
		if bx.Label != tc.label || bx.Fill != tc.fill || !reflect.DeepEqual(bx.Members, []string{"a", "b"}) {
			t.Errorf("%s: box = %+v", tc.line, bx)
		}
	}
}

func TestMermaidCreateAndDestroy(t *testing.T) {
	d := mustParse(t, `
  sequenceDiagram
  participant a as Alice
  a ->>b: Hello Bob?
  create participant c
  b-->>c: Hello c!
  c ->> b: Hello b?
  create actor d as Donald
  a ->> d: Hello Donald?
  `)
	c, _ := d.Participant("c")
	dd, _ := d.Participant("d")
	if !c.Created || c.Label() != "c" || c.Kind != ParticipantKindParticipant {
		t.Errorf("c = %+v", c)
	}
	if !dd.Created || dd.Label() != "Donald" || dd.Kind != ParticipantKindActor {
		t.Errorf("d = %+v", dd)
	}

	d = mustParse(t, `
  sequenceDiagram
  participant a as Alice
  a ->>b: Hello Bob?
  destroy a
  b-->>a: Hello Alice!
  b ->> c: Where is Alice?
  destroy c
  b ->> c: Where are you?
  `)
	for _, id := range []string{"a", "c"} {
		if p, _ := d.Participant(id); !p.Destroyed {
			t.Errorf("%s should be destroyed", id)
		}
	}

	d = mustParse(t, `
  sequenceDiagram
  a ->>b: Hello Bob?
  create participant c
  b ->>c: Hello c!
  c ->> b: Hello b?
  destroy c
  b ->> c : Bye c !
  `)
	if c, _ := d.Participant("c"); !c.Created || !c.Destroyed {
		t.Errorf("c = %+v", c)
	}
}

func TestMermaidTrailingColon(t *testing.T) {
	d := mustParse(t, "\nsequenceDiagram\nAlice->>Bob:\nBob->>Alice:Got it!\n")
	if m := d.Items[0].Message; m.Label != "" || m.From != "Alice" || m.To != "Bob" {
		t.Errorf("message = %+v", m)
	}
}

func TestMermaidParticipantTypes(t *testing.T) {
	d := mustParse(t, `
  sequenceDiagram
      participant Alice@{ "type" : "database" }
      participant Bob@{ "type" : "database" }
      participant Carl@{ type: "database" }
      participant David@{ "type" : 'database' }
      participant Eve@{ type: 'database' }
      participant Favela@{ "type" : "database"    }
      Bob->>+Alice: Hi Alice
      Alice->>+Bob: Hi Bob
    `)
	for _, p := range d.Participants {
		if p.Kind != ParticipantKindDatabase || p.Label() != p.ID {
			t.Errorf("%s = %+v", p.ID, p)
		}
	}
	if s := flatten(d.Items); len(s) != 4 || s[2].text != "Hi Bob" {
		t.Errorf("signals = %+v", s)
	}

	for _, typ := range []string{"boundary", "control", "entity", "database", "collections", "queue"} {
		d := mustParse(t, "sequenceDiagram\nparticipant X@{ \"type\" : \""+typ+"\" }\nX->X: test")
		if got := d.Participants[0].Kind.String(); got != typ {
			t.Errorf("type %s parsed as %s", typ, got)
		}
	}

	d = mustParse(t, `
    sequenceDiagram
        participant lead
        participant dsa@{ "type" : "queue" }
        API->>+Database: getUserb
        Database-->>-API: userb
        dsa --> Database: hello
`)
	if dsa, _ := d.Participant("dsa"); dsa.Kind != ParticipantKindQueue {
		t.Errorf("dsa = %+v", dsa)
	}
	if api, _ := d.Participant("API"); api.Kind != ParticipantKindParticipant {
		t.Errorf("API = %+v", api)
	}
}

func TestMermaidParticipantConfigErrors(t *testing.T) {
	for _, src := range []string{
		"participant D@{ \"type: \"entity\" }",
		"participant C@{ \"type\" \"control\" }\nC ->> C: action",
		"participant E@{ \"type\": \"entity\"\nE ->> E: process",
		"participant SAS MyServiceWithMoreThan20Chars <br> service decription",
		"participant X_AutoPublishable asAAAAAAAAAAAAA:AAAAAAAAAAAAA",
	} {
		mustFail(t, "sequenceDiagram\n"+src)
	}
}

func TestMermaidParticipantAliases(t *testing.T) {
	cases := []struct {
		decl  string
		kind  ParticipantKind
		label string
	}{
		{`participant Alice@{ "type" : "boundary" } as Public API`, ParticipantKindBoundary, "Public API"},
		{`actor A@{ "type" : "database" } AS Database Server`, ParticipantKindDatabase, "Database Server"},
		{`participant API@{ "type" : "boundary", "alias": "Public API" }`, ParticipantKindBoundary, "Public API"},
		{`actor U@{ "type" : "actor", "alias": "End User" }`, ParticipantKindActor, "End User"},
		{`participant API@{ "type" : "boundary", "alias": "Internal Name" } as External Name`, ParticipantKindBoundary, "External Name"},
		{`participant API@{ "alias": "Public API" }`, ParticipantKindParticipant, "Public API"},
		{`participant B@{ "type" : "control" } as Service B`, ParticipantKindControl, "Service B"},
	}
	for _, tc := range cases {
		d := mustParse(t, "sequenceDiagram\n"+tc.decl)
		p := d.Participants[0]
		if p.Kind != tc.kind || p.Label() != tc.label {
			t.Errorf("%s: kind %v label %q, want %v %q", tc.decl, p.Kind, p.Label(), tc.kind, tc.label)
		}
	}
}
