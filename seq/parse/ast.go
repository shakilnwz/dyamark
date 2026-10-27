// Copied from github.com/julianshen/mmgo pkg/diagram/sequence.go and
// extended; see UPSTREAM. MIT licence in LICENSE.mmgo.

package parse

// ParticipantKind is how a participant is drawn: a box, a stick figure, or
// one of Mermaid's typed shapes (`participant A@{ "type": "database" }`).
type ParticipantKind int8

const (
	ParticipantKindUnknown ParticipantKind = iota
	ParticipantKindParticipant
	ParticipantKindActor
	ParticipantKindBoundary
	ParticipantKindCollections
	ParticipantKindControl
	ParticipantKindDatabase
	ParticipantKindEntity
	ParticipantKindQueue
)

var participantKindNames = []string{
	"unknown", "participant", "actor", "boundary", "collections",
	"control", "database", "entity", "queue",
}

func (p ParticipantKind) String() string { return enumString(p, participantKindNames) }

// participantKindByName maps a `type` from the participant config to a kind.
func participantKindByName(name string) (ParticipantKind, bool) {
	for i, n := range participantKindNames {
		if i > 0 && n == name {
			return ParticipantKind(i), true
		}
	}
	return ParticipantKindUnknown, false
}

// Box groups participants under a coloured background and a label.
type Box struct {
	Label string
	// Fill is the CSS colour as written, or "transparent".
	Fill    string
	Wrap    bool
	Members []string
	Line    int
}

// Participant is a column in a sequence diagram.
type Participant struct {
	ID    string
	Alias string
	Kind  ParticipantKind
	// Wrap is whether the alias wraps to the actor width.
	Wrap bool
	// BoxIndex is the index into Diagram.Boxes, or -1.
	BoxIndex int
	// Created and Destroyed mark participants with `create` / `destroy`
	// statements; the Create and Destroy items say where.
	Created   bool
	Destroyed bool
	// Links are the actor menu entries from `link` and `links`, in order.
	Links      []Link
	Properties map[string]any
	// Line is where the participant was declared, or first mentioned.
	Line int
}

// Label is the text shown in the participant's box.
func (p Participant) Label() string {
	if p.Alias != "" {
		return p.Alias
	}
	return p.ID
}

// Link is one actor menu entry.
type Link struct {
	Label string
	URL   string
}

// ArrowType describes the visual style of a sequence message arrow. The
// comments give the matching Mermaid LINETYPE.
type ArrowType int8

const (
	ArrowTypeUnknown      ArrowType = iota
	ArrowTypeSolid                  // ->>    SOLID: solid line, filled arrowhead
	ArrowTypeSolidNoHead            // ->     SOLID_OPEN: solid line, no arrowhead
	ArrowTypeDashed                 // -->>   DOTTED
	ArrowTypeDashedNoHead           // -->    DOTTED_OPEN
	ArrowTypeSolidCross             // -x     SOLID_CROSS
	ArrowTypeDashedCross            // --x    DOTTED_CROSS
	ArrowTypeSolidOpen              // -)     SOLID_POINT: open (async) arrowhead
	ArrowTypeDashedOpen             // --)    DOTTED_POINT
	ArrowTypeSolidBi                // <<->>  BIDIRECTIONAL_SOLID
	ArrowTypeDashedBi               // <<-->> BIDIRECTIONAL_DOTTED

	// Half arrows: one barb only, filled ("solid") or a single line
	// ("stick"), on the top or bottom side.
	ArrowTypeSolidTop                 // -|\   SOLID_TOP
	ArrowTypeSolidBottom              // -|/   SOLID_BOTTOM
	ArrowTypeStickTop                 // -\\   STICK_TOP
	ArrowTypeStickBottom              // -//   STICK_BOTTOM
	ArrowTypeSolidTopDashed           // --|\  SOLID_TOP_DOTTED
	ArrowTypeSolidBottomDashed        // --|/  SOLID_BOTTOM_DOTTED
	ArrowTypeStickTopDashed           // --\\  STICK_TOP_DOTTED
	ArrowTypeStickBottomDashed        // --//  STICK_BOTTOM_DOTTED
	ArrowTypeSolidTopReverse          // /|-   SOLID_ARROW_TOP_REVERSE
	ArrowTypeSolidBottomReverse       // \|-   SOLID_ARROW_BOTTOM_REVERSE
	ArrowTypeStickTopReverse          // //-   STICK_ARROW_TOP_REVERSE
	ArrowTypeStickBottomReverse       // \\-   STICK_ARROW_BOTTOM_REVERSE
	ArrowTypeSolidTopReverseDashed    // /|--  SOLID_ARROW_TOP_REVERSE_DOTTED
	ArrowTypeSolidBottomReverseDashed // \|--  SOLID_ARROW_BOTTOM_REVERSE_DOTTED
	ArrowTypeStickTopReverseDashed    // //--  STICK_ARROW_TOP_REVERSE_DOTTED
	ArrowTypeStickBottomReverseDashed // \\--  STICK_ARROW_BOTTOM_REVERSE_DOTTED
)

var arrowTypeNames = []string{
	"unknown",
	"solid",
	"solid-no-head",
	"dashed",
	"dashed-no-head",
	"solid-cross",
	"dashed-cross",
	"solid-open",
	"dashed-open",
	"solid-bi",
	"dashed-bi",
	"solid-top",
	"solid-bottom",
	"stick-top",
	"stick-bottom",
	"solid-top-dashed",
	"solid-bottom-dashed",
	"stick-top-dashed",
	"stick-bottom-dashed",
	"solid-top-reverse",
	"solid-bottom-reverse",
	"stick-top-reverse",
	"stick-bottom-reverse",
	"solid-top-reverse-dashed",
	"solid-bottom-reverse-dashed",
	"stick-top-reverse-dashed",
	"stick-bottom-reverse-dashed",
}

func (a ArrowType) String() string { return enumString(a, arrowTypeNames) }

// mermaidLineTypes is Mermaid's LINETYPE number for each ArrowType.
var mermaidLineTypes = []int{
	-1, 0, 5, 1, 6, 3, 4, 24, 25, 33, 34,
	41, 42, 43, 44, 51, 52, 53, 54,
	45, 46, 47, 48, 55, 56, 57, 58,
}

// MermaidLineType is the LINETYPE value Mermaid's renderer switches on,
// or -1 for ArrowTypeUnknown.
func (a ArrowType) MermaidLineType() int {
	if int(a) < 0 || int(a) >= len(mermaidLineTypes) {
		return -1
	}
	return mermaidLineTypes[a]
}

func (a ArrowType) HasArrowHead() bool {
	return a != ArrowTypeSolidNoHead && a != ArrowTypeDashedNoHead
}

func (a ArrowType) IsBidirectional() bool {
	return a == ArrowTypeSolidBi || a == ArrowTypeDashedBi
}

func (a ArrowType) IsDashed() bool {
	switch a {
	case ArrowTypeDashed, ArrowTypeDashedNoHead,
		ArrowTypeDashedCross, ArrowTypeDashedOpen,
		ArrowTypeDashedBi,
		ArrowTypeSolidTopDashed, ArrowTypeSolidBottomDashed,
		ArrowTypeStickTopDashed, ArrowTypeStickBottomDashed,
		ArrowTypeSolidTopReverseDashed, ArrowTypeSolidBottomReverseDashed,
		ArrowTypeStickTopReverseDashed, ArrowTypeStickBottomReverseDashed:
		return true
	default:
		return false
	}
}

// LifelineEffect describes the effect of a message on the receiver's lifeline
// activation bar. Modeled as a single enum (rather than two bools) so that
// contradictory states are unrepresentable.
type LifelineEffect int8

const (
	LifelineEffectNone       LifelineEffect = iota // no change
	LifelineEffectActivate                         // + suffix: activate receiver
	LifelineEffectDeactivate                       // - suffix: deactivate sender
)

var lifelineEffectNames = []string{"none", "activate", "deactivate"}

func (l LifelineEffect) String() string { return enumString(l, lifelineEffectNames) }

// CentralConnection is Mermaid's `()` marker, which joins a message to the
// middle of an activation bar instead of its edge.
type CentralConnection int8

const (
	CentralNone   CentralConnection = iota
	CentralTarget                   // A ->>() B: joins B's bar, and activates B
	CentralSource                   // A ()->> B: joins A's bar
	CentralBoth                     // A ()->>() B: both, and activates B
)

var centralConnectionNames = []string{"none", "target", "source", "both"}

func (c CentralConnection) String() string { return enumString(c, centralConnectionNames) }

// Message is a single arrow/interaction in a sequence diagram.
type Message struct {
	From      string
	To        string
	Label     string
	Wrap      bool
	ArrowType ArrowType
	Lifeline  LifelineEffect
	Central   CentralConnection
}

// Item is one source-ordered element in a sequence diagram body.
// Exactly one pointer is set.
//
// A tagged struct is used instead of an interface because the types are a
// closed set, and a value-typed container avoids interface allocation
// overhead during parser/renderer traversal.
type Item struct {
	Message    *Message
	Block      *Block
	Note       *Note
	Activation *Activation
	AutoNumber *AutoNumber
	// Create names the participant a `create` statement introduces; the
	// next message must be sent to it.
	Create *string
	// Destroy names the participant a `destroy` statement removes; the
	// next message must involve it.
	Destroy *string
	// Line is the 1-based source line of the statement.
	Line int
}

// Activation represents a standalone `activate <id>` or `deactivate <id>`
// statement. The inline `+`/`-` arrow shorthand is represented on Message.Lifeline
// instead; this struct exists for the source-ordered standalone form.
type Activation struct {
	Participant string
	Activate    bool
}

// AutoNumber is an `autonumber` statement. It applies to the messages after
// it, like Mermaid's.
type AutoNumber struct {
	// Start and Step are zero when the statement leaves them unchanged.
	Start   float64
	Step    float64
	Visible bool
}

// BlockKind identifies the structural block type.
type BlockKind int8

const (
	BlockKindUnknown  BlockKind = iota
	BlockKindAlt                // alt/else alternative branches
	BlockKindOpt                // opt conditional (no else)
	BlockKindLoop               // loop iteration
	BlockKindPar                // par parallel branches (uses 'and' for extras)
	BlockKindCritical           // critical section (uses 'option' for extras)
	BlockKindBreak              // break out of loop
	BlockKindRect               // rectangular visual grouping
	BlockKindParOver            // par_over: par whose branches overlap
)

var blockKindNames = []string{"unknown", "alt", "opt", "loop", "par", "critical", "break", "rect", "par_over"}

func (b BlockKind) String() string { return enumString(b, blockKindNames) }

// Block is a nested structural element in a sequence diagram.
//
// Items holds the body in source order (messages, notes, nested blocks).
// Branches holds alternative branches for multi-branch kinds:
//   - BlockKindAlt:      else branches
//   - BlockKindPar, BlockKindParOver: 'and' branches
//   - BlockKindCritical: 'option' branches
//
// Branches is always empty for single-branch kinds (opt, loop, break, rect).
// Each branch is itself a Block with Kind matching the parent.
type Block struct {
	Label string
	Wrap  bool
	Items []Item
	// Branches is always empty for single-branch kinds.
	Branches []Block
	Kind     BlockKind
	// Fill is a rect's CSS colour as written; Mermaid gives rect no label.
	Fill string
	// Line is the opening (or branch keyword) line; EndLine the `end` line,
	// set on the block itself, not its branches.
	Line    int
	EndLine int
}

// NotePosition describes where a note is drawn relative to participants.
type NotePosition int8

const (
	NotePositionUnknown NotePosition = iota
	NotePositionLeft
	NotePositionRight
	NotePositionOver
)

var notePositionNames = []string{"unknown", "left", "right", "over"}

func (n NotePosition) String() string { return enumString(n, notePositionNames) }

// Note is an annotation on one or more participants.
//
// Cardinality of Participants depends on Position:
//   - NotePositionLeft, NotePositionRight: exactly 1 participant
//   - NotePositionOver: 1 or 2 participants (spans range if 2)
type Note struct {
	Participants []string
	Text         string
	Wrap         bool
	Position     NotePosition
}

// Diagram is a parsed sequence diagram.
type Diagram struct {
	Participants []Participant
	Items        []Item
	Boxes        []Box
	Title        string
	// AccTitle / AccDescr carry Mermaid's accessibility metadata.
	AccTitle string
	AccDescr string
	// Config is the diagram's own configuration, from frontmatter and
	// %%{init}%% directives.
	Config Config
	// Warnings are problems Mermaid ignores silently, such as a directive
	// that isn't valid JSON.
	Warnings []Warning
}

// Participant returns the participant with the given ID.
func (d *Diagram) Participant(id string) (*Participant, bool) {
	for i := range d.Participants {
		if d.Participants[i].ID == id {
			return &d.Participants[i], true
		}
	}
	return nil, false
}

func enumString[T ~int8](v T, names []string) string {
	i := int(v)
	if i < 0 || i >= len(names) {
		return "unknown"
	}
	return names[i]
}
