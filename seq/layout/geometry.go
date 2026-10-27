package layout

import "dyamark/seq/parse"

// Diagram is a laid-out sequence diagram: everything needed to draw it,
// in the same user-space units (CSS px) as Mermaid's SVG.
type Diagram struct {
	// ViewBox is the region to show, margins included, as in Mermaid's
	// SVG viewBox attribute.
	ViewBox Rect
	Look    Look

	Title       *Text
	Boxes       []Box
	Rects       []Background
	Lifelines   []Lifeline
	Actors      []Actor
	Activations []Activation
	Frames      []Frame
	Notes       []Note
	Messages    []Message
}

// Rect is an axis-aligned rectangle.
type Rect struct{ X, Y, W, H float32 }

// Point is a position.
type Point struct{ X, Y float32 }

// Anchor is a text's horizontal alignment around its X, as SVG's
// text-anchor.
type Anchor int8

const (
	AnchorStart Anchor = iota
	AnchorMiddle
	AnchorEnd
)

// Baseline says what a text's Y is measured to, as SVG's
// dominant-baseline.
type Baseline int8

const (
	BaselineAlphabetic Baseline = iota
	BaselineMiddle
	BaselineCentral
)

// TextKind says what a text labels, which decides its style.
type TextKind int8

const (
	TextActor TextKind = iota
	TextMessage
	TextNote
	TextFrameKeyword // "loop", "alt", … in the frame's corner tab
	TextFrameTitle
	TextSectionTitle // an else / and / option label
	TextBoxLabel
	TextTitle
	TextSequenceNumber
)

// Text is one line of text, placed as Mermaid places its <text> element.
type Text struct {
	Kind     TextKind
	X, Y     float32
	Anchor   Anchor
	Baseline Baseline
	// DY shifts the line down by this many px after positioning, as the
	// element's dy attribute.
	DY     float32
	Size   float32
	Weight int
	Text   string
}

// Actor is one participant's head or foot.
type Actor struct {
	ID     string
	Kind   parse.ParticipantKind
	Footer bool
	// Rect is the participant's box. For a stick figure it's the cell the
	// figure and its label sit in.
	Rect Rect
	// Radius rounds Rect's corners.
	Radius float32
	Figure *StickFigure
	Label  []Text
	Line   int
}

// StickFigure is the drawing of an `actor` participant.
type StickFigure struct {
	Head   Point
	Radius float32
	// Lines are the torso, arms and legs, as pairs of end points.
	Lines [][2]Point
}

// Lifeline is the vertical line under a participant.
type Lifeline struct {
	ID     string
	X      float32
	Y1, Y2 float32
}

// Box is a `box` grouping participants.
type Box struct {
	Rect  Rect
	Fill  string
	Label []Text
	Line  int
}

// Background is a `rect` block's fill.
type Background struct {
	Rect Rect
	Fill string
	Line int
}

// Activation is an activation bar on a lifeline.
type Activation struct {
	ID   string
	Rect Rect
	// Depth is how many activations of the same participant were open
	// when this one started.
	Depth int
}

// Frame is a loop, alt, opt, par, critical or break block.
type Frame struct {
	Kind parse.BlockKind
	Rect Rect
	// Tab is the outline of the keyword tab in the top-left corner.
	Tab []Point
	// Dividers are the y of the dashed lines between sections.
	Dividers []float32
	Texts    []Text
	Line     int
}

// Note is a note.
type Note struct {
	Rect Rect
	Text []Text
	Line int
}

// Marker is what's drawn at a message line's end.
type Marker int8

const (
	MarkerNone Marker = iota
	MarkerArrow
	MarkerOpen // the async arrow of -) and --)
	MarkerCross
	MarkerSolidTop
	MarkerSolidBottom
	MarkerStickTop
	MarkerStickBottom
)

// Message is a message line with its label.
type Message struct {
	From, To string
	Arrow    parse.ArrowType
	Dashed   bool
	// Path is the line: two points for a straight message, four for a
	// self message drawn as a cubic curve (start, two controls, end), or
	// a polyline of four for a self message with rightAngles.
	Path   []Point
	Curved bool
	// StartMarker and EndMarker are drawn at Path's first and last point.
	StartMarker, EndMarker Marker
	Text                   []Text
	// Number is the autonumber badge, if shown.
	Number *SequenceNumber
	// Circles are the centres of central-connection circles.
	Circles []Point
	Line    int
}

// SequenceNumber is a message's autonumber badge.
type SequenceNumber struct {
	Center Point
	Radius float32
	Text   Text
}
