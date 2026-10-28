package layout

import (
	"math"
	"strconv"

	"dyamark/seq/parse"
)

// Constants from Mermaid 12.0.0 actorBands.ts
const (
	GlyphBandHeight  = 44.0
	GlyphLabelGap    = 6.0
	LabelLifelineGap = 6.0
)

type actorLayoutData struct {
	id    string
	name  string
	kind  parse.ParticipantKind
	alias string
	x     float64
	y     float64
	// starty is where the head is drawn, which a create message moves
	// down; stopy is where the foot is drawn.
	starty     float64
	stopy      float64
	created    bool
	destroyed  bool
	width      float64
	height     float64
	margin     float64
	textHeight float64
	boxIndex   int
	prevActor  string
	nextActor  string
	line       int
}

type boundsTracker struct {
	startx      *float64
	stopx       *float64
	starty      *float64
	stopy       *float64
	verticalPos float64
	boxMargin   float64
	// loops are the frames and rect blocks open at the current position,
	// outermost first. Each grows to hold whatever is inserted while it's
	// open, plus a boxMargin per level of nesting, as Mermaid's
	// bounds.sequenceItems do.
	loops []*loopBounds
}

// loopBounds is the extent of an open frame or rect block.
type loopBounds struct {
	startx, stopx, starty, stopy *float64
}

func (b *boundsTracker) bumpVerticalPos(bump float64) {
	b.verticalPos += bump
	if b.stopy == nil || b.verticalPos > *b.stopy {
		val := b.verticalPos
		b.stopy = &val
	}
}

func setMin(p **float64, v float64) {
	if *p == nil || v < **p {
		*p = &v
	}
}

func setMax(p **float64, v float64) {
	if *p == nil || v > **p {
		*p = &v
	}
}

func (b *boundsTracker) insert(x1, y1, x2, y2 float64) {
	minX := math.Min(x1, x2)
	maxX := math.Max(x1, x2)
	minY := math.Min(y1, y2)
	maxY := math.Max(y1, y2)

	setMin(&b.startx, minX)
	setMax(&b.stopx, maxX)
	setMin(&b.starty, minY)
	setMax(&b.stopy, maxY)

	// Mermaid's updateBounds: the outermost loop gets the widest margin.
	for i, lp := range b.loops {
		n := float64(len(b.loops)-i) * b.boxMargin
		setMin(&lp.starty, minY-n)
		setMax(&lp.stopy, maxY+n)
		setMin(&lp.startx, minX-n)
		setMax(&lp.stopx, maxX+n)
		setMin(&b.startx, minX-n)
		setMax(&b.stopx, maxX+n)
		setMin(&b.starty, minY-n)
		setMax(&b.stopy, maxY+n)
	}
}

// newLoop opens a loop starting at the current position.
func (b *boundsTracker) newLoop() *loopBounds {
	y := b.verticalPos
	lp := &loopBounds{starty: &y}
	b.loops = append(b.loops, lp)
	return lp
}

// endLoop closes the innermost loop.
func (b *boundsTracker) endLoop() *loopBounds {
	if len(b.loops) == 0 {
		return nil
	}
	lp := b.loops[len(b.loops)-1]
	b.loops = b.loops[:len(b.loops)-1]
	return lp
}

type eventKind int

const (
	evMessage eventKind = iota
	evNote
	evActivation
	evBlockStart
	evBlockBranch
	evBlockEnd
	evRectStart
	evRectEnd
	evAutoNumber
)

type layoutEvent struct {
	kind       eventKind
	msg        *parse.Message
	note       *parse.Note
	activation *parse.Activation
	block      *parse.Block
	blockKind  parse.BlockKind
	branch     *parse.Block
	fill       string
	label      string
	autoNumber *parse.AutoNumber
	// create and destroy name the participant a message creates or
	// destroys, from the `create` / `destroy` statement before it.
	create  string
	destroy string
	line    int
}

// Layout translates a parsed sequence diagram AST into laid-out geometry.
func Layout(d *parse.Diagram, m Measurer) (*Diagram, error) {
	if d == nil {
		return &Diagram{Look: LookNeo}, nil
	}
	return LayoutWithConfig(d, ResolveConfig(d.Config), m)
}

// LayoutWithConfig lays out a parsed diagram with explicit configuration.
func LayoutWithConfig(d *parse.Diagram, cfg Config, m Measurer) (*Diagram, error) {
	if m == nil {
		m = Default()
	}
	if d == nil {
		return &Diagram{Look: cfg.Look}, nil
	}

	out := &Diagram{
		Look: cfg.Look,
	}

	// 1. Build actor models
	actors := make(map[string]*actorLayoutData)
	var actorKeys []string

	for i, p := range d.Participants {
		label := p.Label()
		if p.Wrap {
			label = m.WrapLabel(label, cfg.Width-2*cfg.WrapPadding, cfg.ActorFont())
		}
		actDims := m.MeasureText(label, cfg.ActorFont())

		w := cfg.Width
		if !p.Wrap {
			w = math.Max(cfg.Width, actDims.Width+2*cfg.WrapPadding)
		}

		var h float64
		if cfg.Look == LookNeo {
			stackH := GlyphBandHeight + GlyphLabelGap + actDims.Height + LabelLifelineGap
			h = math.Max(stackH, cfg.Height)
		} else {
			if p.Wrap {
				h = math.Max(actDims.Height, cfg.Height)
			} else {
				h = cfg.Height
			}
		}

		a := &actorLayoutData{
			id:         p.ID,
			name:       p.ID,
			kind:       p.Kind,
			alias:      label,
			width:      w,
			height:     h,
			textHeight: actDims.Height,
			created:    p.Created,
			boxIndex:   p.BoxIndex,
			line:       p.Line,
		}
		if i > 0 {
			a.prevActor = d.Participants[i-1].ID
			actors[d.Participants[i-1].ID].nextActor = p.ID
		}
		actors[p.ID] = a
		actorKeys = append(actorKeys, p.ID)
	}

	// Flatten diagram items into sequential events
	events := flattenItems(d.Items)

	// Calculate max message width per actor
	maxMessageWidth := make(map[string]float64)
	for _, ev := range events {
		switch ev.kind {
		case evNote:
			n := ev.note
			if len(n.Participants) == 0 {
				continue
			}
			fromID := n.Participants[0]
			toID := n.Participants[len(n.Participants)-1]
			fromAct := actors[fromID]
			toAct := actors[toID]
			if fromAct == nil || toAct == nil {
				continue
			}

			noteLabel := n.Text
			if n.Wrap {
				noteLabel = m.WrapLabel(noteLabel, cfg.Width-2*cfg.WrapPadding, cfg.NoteFont())
			}
			noteDims := m.MeasureText(noteLabel, cfg.NoteFont())
			noteW := noteDims.Width + 2*cfg.WrapPadding

			switch n.Position {
			case parse.NotePositionRight:
				if fromAct.nextActor != "" {
					maxMessageWidth[fromID] = math.Max(maxMessageWidth[fromID], noteW)
				}
			case parse.NotePositionLeft:
				if fromAct.prevActor != "" {
					maxMessageWidth[fromAct.prevActor] = math.Max(maxMessageWidth[fromAct.prevActor], noteW)
				}
			case parse.NotePositionOver:
				if fromID == toID {
					if fromAct.prevActor != "" {
						maxMessageWidth[fromAct.prevActor] = math.Max(maxMessageWidth[fromAct.prevActor], noteW/2)
					}
					if fromAct.nextActor != "" {
						maxMessageWidth[fromID] = math.Max(maxMessageWidth[fromID], noteW/2)
					}
				}
			}

		case evMessage:
			msg := ev.msg
			fromAct := actors[msg.From]
			toAct := actors[msg.To]
			if fromAct == nil || toAct == nil {
				continue
			}

			msgLabel := msg.Label
			if msg.Wrap {
				msgLabel = m.WrapLabel(msgLabel, cfg.Width-2*cfg.WrapPadding, cfg.MessageFont())
			}
			msgDims := m.MeasureText(msgLabel, cfg.MessageFont())
			msgW := msgDims.Width + 2*cfg.WrapPadding

			if msg.From == msg.To {
				maxMessageWidth[msg.From] = math.Max(maxMessageWidth[msg.From], msgW/2)
				maxMessageWidth[msg.To] = math.Max(maxMessageWidth[msg.To], msgW/2)
			} else if fromAct.nextActor == toAct.id {
				maxMessageWidth[msg.From] = math.Max(maxMessageWidth[msg.From], msgW)
			} else if fromAct.prevActor == toAct.id {
				maxMessageWidth[msg.To] = math.Max(maxMessageWidth[msg.To], msgW)
			}
		}
	}

	// Calculate actor margins and row maxHeight
	var rowMaxHeight float64
	for _, key := range actorKeys {
		a := actors[key]
		if a.height > rowMaxHeight {
			rowMaxHeight = a.height
		}

		msgW := maxMessageWidth[key]
		if a.nextActor != "" {
			nextA := actors[a.nextActor]
			w := msgW + cfg.ActorMargin - a.width/2 - nextA.width/2
			a.margin = math.Max(w, cfg.ActorMargin)
		} else {
			w := msgW + cfg.ActorMargin - a.width/2
			a.margin = math.Max(w, cfg.ActorMargin)
		}
	}
	cfg.Height = math.Max(rowMaxHeight, cfg.Height)
	// Every actor takes the row's height, so their lifelines start on one
	// line.
	for _, a := range actors {
		a.height = math.Max(a.height, cfg.Height)
	}

	// Box calculations
	type boxData struct {
		x      float64
		y      float64
		w      float64
		h      float64
		margin float64
		label  string
		fill   string
		line   int
	}
	var boxModels []*boxData
	var maxBoxTextHeight float64

	for _, b := range d.Boxes {
		boxDims := m.MeasureText(b.Label, cfg.MessageFont())
		if boxDims.Height > maxBoxTextHeight {
			maxBoxTextHeight = boxDims.Height
		}

		var membersWidth float64
		for _, mID := range b.Members {
			if act, ok := actors[mID]; ok {
				membersWidth += act.width + act.margin
			}
		}
		totalW := membersWidth + cfg.BoxMargin*8 - 2*cfg.BoxTextMargin
		minW := math.Max(totalW, boxDims.Width+2*cfg.WrapPadding)
		bMargin := cfg.BoxTextMargin
		if totalW < minW {
			bMargin += (minW - totalW) / 2
		}

		boxModels = append(boxModels, &boxData{
			margin: bMargin,
			label:  b.Label,
			fill:   b.Fill,
			line:   b.Line,
		})
	}

	bounds := &boundsTracker{boxMargin: cfg.BoxMargin}

	hasBoxes := len(d.Boxes) > 0
	hasBoxTitles := false
	for _, b := range d.Boxes {
		if b.Label != "" {
			hasBoxTitles = true
			break
		}
	}

	if hasBoxes {
		bounds.bumpVerticalPos(cfg.BoxMargin)
		if hasBoxTitles {
			bounds.bumpVerticalPos(maxBoxTextHeight)
		}
	}

	// Horizontal placement of actors
	var prevWidth float64
	var prevMargin float64
	var prevBoxIdx int = -1

	for _, key := range actorKeys {
		a := actors[key]
		boxIdx := a.boxIndex

		if prevBoxIdx >= 0 && prevBoxIdx != boxIdx {
			prevMargin += cfg.BoxMargin + boxModels[prevBoxIdx].margin
		}
		if boxIdx >= 0 && boxIdx != prevBoxIdx {
			boxModels[boxIdx].x = prevWidth + prevMargin
			boxModels[boxIdx].y = 0 // in Mermaid, box.y is 0
			prevMargin += boxModels[boxIdx].margin
		}

		// A created actor is drawn beside the message that creates it, so
		// leave room for half of it.
		if a.created {
			prevMargin += a.width / 2
		}

		a.x = prevWidth + prevMargin
		a.y = bounds.verticalPos
		a.starty = bounds.verticalPos

		bounds.insert(a.x, bounds.verticalPos, a.x+a.width, a.height)

		prevWidth += a.width + prevMargin
		if boxIdx >= 0 {
			boxModels[boxIdx].w = prevWidth + boxModels[boxIdx].margin - boxModels[boxIdx].x
		}
		prevMargin = a.margin
		prevBoxIdx = boxIdx
	}

	bounds.bumpVerticalPos(rowMaxHeight)

	// Activations open at the current position, per participant, in the
	// order they started. Both passes below track them.
	type openActivation struct {
		startx, stopx, starty float64
		depth                 int
	}
	active := map[string][]openActivation{}

	activationBounds := func(id string) (float64, float64) {
		a := actors[id]
		mid := a.x + a.width/2
		left, right := mid-1, mid+1
		for _, e := range active[id] {
			left = math.Min(left, e.startx)
			right = math.Max(right, e.stopx)
		}
		return left, right
	}
	startActivation := func(id string) {
		a := actors[id]
		n := len(active[id])
		x := a.x + a.width/2 + float64(n-1)*cfg.ActivationWidth/2
		active[id] = append(active[id], openActivation{
			startx: x,
			stopx:  x + cfg.ActivationWidth,
			starty: bounds.verticalPos + 2,
			depth:  n,
		})
	}
	endActivation := func(id string) (openActivation, bool) {
		stack := active[id]
		if len(stack) == 0 {
			return openActivation{}, false
		}
		e := stack[len(stack)-1]
		active[id] = stack[:len(stack)-1]
		return e, true
	}

	// msgModel is where a message runs, as Mermaid's buildMessageModel.
	type msgModel struct {
		startx, stopx        float64
		fromBounds, toBounds float64
		width                float64
		label                string
	}
	buildMessageModel := func(msg *parse.Message) msgModel {
		fromLeft, fromRight := activationBounds(msg.From)
		toLeft, toRight := activationBounds(msg.To)
		arrowToRight := fromLeft <= toLeft
		startx, stopx := fromLeft, toRight
		if arrowToRight {
			startx, stopx = fromRight, toLeft
		}
		adjust := func(v float64) float64 {
			if arrowToRight {
				return -v
			}
			return v
		}

		if cfg.Look == LookNeo {
			if msg.ArrowType != parse.ArrowTypeSolidNoHead {
				stopx += adjust(3)
			}
			if msg.ArrowType.IsBidirectional() {
				startx -= adjust(3)
			}
		}

		if msg.From == msg.To {
			stopx = startx
		} else {
			// The first message into an activation isn't drawn to it yet,
			// so it stops at the bar's edge.
			if msg.Lifeline == parse.LifelineEffectActivate && math.Abs(toLeft-toRight) <= 2 {
				stopx += adjust(cfg.ActivationWidth/2 - 1)
			}
			// Shorten the line so its arrowhead ends on the lifeline.
			if !noArrowheadShortening(msg.ArrowType) {
				stopx += adjust(3)
			}
			if startArrowheadShortening(msg.ArrowType) {
				startx -= adjust(3)
			}
		}

		boundedWidth := math.Abs(startx - stopx)
		label := msg.Label
		if msg.Wrap && label != "" {
			label = m.WrapLabel(label, math.Max(boundedWidth+2*cfg.WrapPadding, cfg.Width), cfg.MessageFont())
		}
		width := math.Max(boundedWidth+2*cfg.WrapPadding, cfg.Width)
		if !msg.Wrap {
			width = math.Max(width, m.MeasureText(label, cfg.MessageFont()).Width+2*cfg.WrapPadding)
		}
		return msgModel{
			startx:     startx,
			stopx:      stopx,
			fromBounds: math.Min(math.Min(fromLeft, fromRight), math.Min(toLeft, toRight)),
			toBounds:   math.Max(math.Max(fromLeft, fromRight), math.Max(toLeft, toRight)),
			width:      width,
			label:      label,
		}
	}

	// noteModel is where a note sits, as Mermaid's buildNoteModel.
	type noteModel struct {
		startx, width float64
		label         string
	}
	buildNoteModel := func(n *parse.Note) (noteModel, bool) {
		if len(n.Participants) == 0 {
			return noteModel{}, false
		}
		from := actors[n.Participants[0]]
		to := actors[n.Participants[len(n.Participants)-1]]
		if from == nil || to == nil {
			return noteModel{}, false
		}
		wrap := n.Wrap && n.Text != ""
		measure := func(maxWidth float64) TextDimensions {
			label := n.Text
			if wrap {
				label = m.WrapLabel(label, maxWidth, cfg.NoteFont())
			}
			return m.MeasureText(label, cfg.NoteFont())
		}
		dims := measure(cfg.Width)
		nm := noteModel{startx: from.x}

		switch {
		case n.Position == parse.NotePositionRight:
			if wrap {
				nm.width = math.Max(cfg.Width, dims.Width)
			} else {
				nm.width = math.Max(from.width/2+to.width/2, dims.Width+2*cfg.NoteMargin)
			}
			nm.startx = from.x + (from.width+cfg.ActorMargin)/2
		case n.Position == parse.NotePositionLeft:
			if wrap {
				nm.width = math.Max(cfg.Width, dims.Width+2*cfg.NoteMargin)
			} else {
				nm.width = math.Max(from.width/2+to.width/2, dims.Width+2*cfg.NoteMargin)
			}
			nm.startx = from.x - nm.width + (from.width-cfg.ActorMargin)/2
		case from == to:
			dims = measure(math.Max(cfg.Width, from.width))
			if wrap {
				nm.width = math.Max(cfg.Width, from.width)
			} else {
				nm.width = math.Max(math.Max(from.width, cfg.Width), dims.Width+2*cfg.NoteMargin)
			}
			nm.startx = from.x + (from.width-nm.width)/2
		default:
			nm.width = math.Abs(from.x+from.width/2-(to.x+to.width/2)) + cfg.ActorMargin
			if from.x < to.x {
				nm.startx = from.x + from.width/2 - cfg.ActorMargin/2
			} else {
				nm.startx = to.x + to.width/2 - cfg.ActorMargin/2
			}
		}

		nm.label = n.Text
		if wrap {
			nm.label = m.WrapLabel(n.Text, nm.width-2*cfg.WrapPadding, cfg.NoteFont())
		}
		return nm, true
	}

	// 2. Loop widths, which block labels wrap to. Mermaid's
	// calculateLoopBounds, which also replays the activations.
	type loopWidthModel struct {
		from, to, width float64
	}
	loopWidths := make(map[*parse.Block]*loopWidthModel)
	var loopStack []*loopWidthModel

	for _, ev := range events {
		switch ev.kind {
		case evBlockStart:
			lwm := &loopWidthModel{from: math.MaxFloat64, to: -math.MaxFloat64}
			loopStack = append(loopStack, lwm)
			loopWidths[ev.block] = lwm
		case evBlockEnd:
			if len(loopStack) > 0 {
				loopStack = loopStack[:len(loopStack)-1]
			}
		case evActivation:
			if actors[ev.activation.Participant] == nil {
				continue
			}
			if ev.activation.Activate {
				startActivation(ev.activation.Participant)
			} else {
				endActivation(ev.activation.Participant)
			}
		case evNote:
			nm, ok := buildNoteModel(ev.note)
			if !ok {
				continue
			}
			for _, lwm := range loopStack {
				lwm.from = math.Min(lwm.from, nm.startx)
				lwm.to = math.Max(lwm.to, nm.startx+nm.width)
				lwm.width = math.Max(lwm.width, math.Abs(lwm.from-lwm.to)) - cfg.LabelBoxWidth
			}
		case evMessage:
			msg := ev.msg
			from, to := actors[msg.From], actors[msg.To]
			if from == nil || to == nil {
				continue
			}
			mm := buildMessageModel(msg)
			for _, lwm := range loopStack {
				if msg.From == msg.To {
					lwm.from = math.Min(math.Min(from.x-mm.width/2, from.x-from.width/2), lwm.from)
					lwm.to = math.Max(math.Max(to.x+mm.width/2, to.x+from.width/2), lwm.to)
					lwm.width = math.Max(lwm.width, math.Abs(lwm.to-lwm.from)) - cfg.LabelBoxWidth
				} else {
					lwm.from = math.Min(mm.startx, lwm.from)
					lwm.to = math.Max(mm.stopx, lwm.to)
					lwm.width = math.Max(lwm.width, mm.width) - cfg.LabelBoxWidth
				}
			}
		}
	}
	active = map[string][]openActivation{}

	// blockLabel is a block or section label as Mermaid draws it: in
	// brackets, wrapped to the block's width.
	blockLabel := func(label string, block *parse.Block) string {
		label = "[" + label + "]"
		if lwm, ok := loopWidths[block]; ok {
			label = m.WrapLabel(label, lwm.width-2*cfg.WrapPadding, cfg.MessageFont())
		}
		return label
	}

	// 3. Process events
	type section struct {
		y     float64
		label string
	}
	type frameModel struct {
		kind     parse.BlockKind
		loop     *loopBounds
		label    string
		titled   bool
		sections []section
		line     int
	}
	var openFrames []*frameModel
	var finishedFrames []*frameModel

	type rectBlockModel struct {
		loop *loopBounds
		fill string
		line int
	}
	var openRects []*rectBlockModel
	var finishedRects []*rectBlockModel

	lineHeight := m.MeasureText("x", cfg.MessageFont()).Height

	sequenceIndex := 1.0
	sequenceIndexStep := 1.0
	autonumberOn := false

	for _, ev := range events {
		switch ev.kind {
		case evAutoNumber:
			if ev.autoNumber.Start > 0 {
				sequenceIndex = ev.autoNumber.Start
			}
			if ev.autoNumber.Step > 0 {
				sequenceIndexStep = ev.autoNumber.Step
			}
			autonumberOn = ev.autoNumber.Visible

		case evActivation:
			id := ev.activation.Participant
			if actors[id] == nil {
				continue
			}
			if ev.activation.Activate {
				startActivation(id)
				continue
			}
			e, ok := endActivation(id)
			if !ok {
				continue
			}
			// A bar too short to see is stretched around the current
			// position. Like Mermaid's activeEnd, that moves only the bar,
			// not what follows.
			stopy := bounds.verticalPos
			if e.starty+18 > stopy {
				e.starty = stopy - 6
				stopy += 12
			}
			bounds.insert(e.startx, stopy-10, e.stopx, stopy)
			out.Activations = append(out.Activations, Activation{
				ID: id,
				Rect: Rect{
					X: float32(e.startx),
					Y: float32(e.starty),
					W: float32(e.stopx - e.startx),
					H: float32(stopy - e.starty),
				},
				Depth: e.depth,
			})

		case evNote:
			nm, ok := buildNoteModel(ev.note)
			if !ok {
				continue
			}
			bounds.bumpVerticalPos(cfg.BoxMargin)
			starty := bounds.verticalPos

			noteDims := m.MeasureText(nm.label, cfg.NoteFont())
			noteH := noteDims.Height + 2*cfg.NoteMargin
			bounds.bumpVerticalPos(noteH)
			bounds.insert(nm.startx, starty, nm.startx+nm.width, starty+noteH)

			var texts []Text
			for i, line := range splitLineBreaks(nm.label) {
				texts = append(texts, Text{
					Kind:     TextNote,
					X:        float32(math.Round(nm.startx + nm.width/2)),
					Y:        float32(textLineY(starty, i, noteDims.LineHeight, cfg.NoteMargin)),
					Anchor:   AnchorMiddle,
					Baseline: BaselineMiddle,
					DY:       float32(cfg.FontSize), // dy="1em"
					Size:     float32(cfg.FontSize),
					Weight:   cfg.NoteFontWeight,
					Text:     line,
				})
			}

			out.Notes = append(out.Notes, Note{
				Rect: Rect{
					X: float32(nm.startx),
					Y: float32(starty),
					W: float32(nm.width),
					H: float32(noteH),
				},
				Text: texts,
				Line: ev.line,
			})

		case evBlockStart:
			bounds.bumpVerticalPos(cfg.BoxMargin)
			fm := &frameModel{
				kind: ev.blockKind,
				loop: bounds.newLoop(),
				line: ev.line,
			}
			openFrames = append(openFrames, fm)
			post := cfg.BoxMargin + cfg.BoxTextMargin
			if ev.label != "" {
				fm.label = blockLabel(ev.label, ev.block)
				fm.titled = true
				post += math.Max(m.MeasureText(fm.label, cfg.MessageFont()).Height, cfg.LabelBoxHeight)
			}
			bounds.bumpVerticalPos(post)

		case evBlockBranch:
			bounds.bumpVerticalPos(cfg.BoxMargin + cfg.BoxTextMargin)
			sec := section{y: bounds.verticalPos}
			post := cfg.BoxMargin
			if ev.label != "" {
				sec.label = blockLabel(ev.label, ev.block)
				post += math.Max(m.MeasureText(sec.label, cfg.MessageFont()).Height, cfg.LabelBoxHeight)
			}
			if len(openFrames) > 0 {
				top := openFrames[len(openFrames)-1]
				top.sections = append(top.sections, sec)
			}
			bounds.bumpVerticalPos(post)

		case evBlockEnd:
			if len(openFrames) > 0 {
				top := openFrames[len(openFrames)-1]
				openFrames = openFrames[:len(openFrames)-1]
				bounds.endLoop()
				if top.loop.stopy != nil {
					bounds.bumpVerticalPos(*top.loop.stopy - bounds.verticalPos)
				}
				finishedFrames = append(finishedFrames, top)
			}

		case evRectStart:
			bounds.bumpVerticalPos(cfg.BoxMargin)
			openRects = append(openRects, &rectBlockModel{
				loop: bounds.newLoop(),
				fill: ev.fill,
				line: ev.line,
			})
			bounds.bumpVerticalPos(cfg.BoxMargin)

		case evRectEnd:
			if len(openRects) > 0 {
				top := openRects[len(openRects)-1]
				openRects = openRects[:len(openRects)-1]
				bounds.endLoop()
				if top.loop.stopy != nil {
					bounds.bumpVerticalPos(*top.loop.stopy - bounds.verticalPos)
				}
				finishedRects = append(finishedRects, top)
			}

		case evMessage:
			msg := ev.msg
			fromAct := actors[msg.From]
			toAct := actors[msg.To]
			if fromAct == nil || toAct == nil {
				continue
			}
			mm := buildMessageModel(msg)
			startx, stopx := mm.startx, mm.stopx
			self := msg.From == msg.To

			// Mermaid's boundMessage.
			msgStarty := bounds.verticalPos
			bounds.bumpVerticalPos(10)
			textDims := m.MeasureText(mm.label, cfg.MessageFont())
			lines := splitLineBreaks(mm.label)
			lineH := textDims.Height / float64(len(lines))
			bounds.bumpVerticalPos(lineH)
			msgHeight := lineH

			totalOffset := textDims.Height - 10
			var lineStartY float64
			if self {
				lineStartY = bounds.verticalPos + totalOffset
				if !cfg.RightAngles {
					totalOffset += cfg.BoxMargin
					lineStartY = bounds.verticalPos + totalOffset
				}
				totalOffset += 30
				dx := math.Max(textDims.Width/2, cfg.Width/2)
				bounds.insert(startx-dx, bounds.verticalPos-10+totalOffset, stopx+dx, bounds.verticalPos+30+totalOffset)
			} else {
				totalOffset += cfg.BoxMargin
				lineStartY = bounds.verticalPos + totalOffset
				bounds.insert(startx, lineStartY-10, stopx, lineStartY)
			}
			bounds.bumpVerticalPos(totalOffset)
			msgHeight += totalOffset
			msgStopy := msgStarty + msgHeight
			bounds.insert(mm.fromBounds, msgStarty, mm.toBounds, msgStopy)

			// Mermaid's adjustCreatedDestroyedData: a created actor's head
			// and a destroyed actor's foot sit on the message, which stops
			// at the actor's edge.
			receiverAdjustment := func(a *actorLayoutData, adj float64) {
				if a.x < fromAct.x {
					bounds.insert(stopx-adj, msgStarty, startx, msgStopy+a.height/2+cfg.NoteMargin)
					stopx += adj
				} else {
					bounds.insert(startx, msgStarty, stopx+adj, msgStopy+a.height/2+cfg.NoteMargin)
					stopx -= adj
				}
			}
			senderAdjustment := func(a *actorLayoutData, adj float64) {
				if a.x < toAct.x {
					bounds.insert(startx-adj, msgStarty, stopx, msgStopy+a.height/2+cfg.NoteMargin)
					startx += adj
				} else {
					bounds.insert(stopx, msgStarty, startx+adj, msgStopy+a.height/2+cfg.NoteMargin)
					startx -= adj
				}
			}
			switch {
			case ev.create == msg.To:
				a := toAct
				receiverAdjustment(a, edgeOffset(a)+3)
				a.starty = lineStartY - a.height/2
				bounds.bumpVerticalPos(a.height / 2)
			case ev.destroy == msg.From:
				a := fromAct
				if cfg.MirrorActors {
					senderAdjustment(a, edgeOffset(a))
				}
				a.stopy = lineStartY - a.height/2
				a.destroyed = true
				bounds.bumpVerticalPos(a.height / 2)
			case ev.destroy == msg.To:
				a := toAct
				if cfg.MirrorActors {
					receiverAdjustment(a, edgeOffset(a)+3)
				}
				a.stopy = lineStartY - a.height/2
				a.destroyed = true
				bounds.bumpVerticalPos(a.height / 2)
			}

			showNumber := autonumberOn || cfg.ShowSequenceNumbers
			bidi := msg.ArrowType.IsBidirectional()
			reverse := isReverseArrow(msg.ArrowType)

			// Mermaid's drawMessage.
			var path []Point
			curved := false
			if self {
				lineStartX := startx
				if showNumber && (reverse || bidi) {
					lineStartX += 10
				}
				curved = !cfg.RightAngles
				if curved {
					path = []Point{
						{X: float32(lineStartX), Y: float32(lineStartY)},
						{X: float32(lineStartX + 60), Y: float32(lineStartY - 10)},
						{X: float32(startx + 60), Y: float32(lineStartY + 30)},
						{X: float32(startx), Y: float32(lineStartY + 20)},
					}
				} else {
					right := startx + math.Max(cfg.Width/2, textDims.Width/2)
					path = []Point{
						{X: float32(lineStartX), Y: float32(lineStartY)},
						{X: float32(right), Y: float32(lineStartY)},
						{X: float32(right), Y: float32(lineStartY + 25)},
						{X: float32(startx), Y: float32(lineStartY + 25)},
					}
				}
			} else {
				// The autonumber badge sits on the line's start, so the
				// line starts after it.
				x1, x2 := startx, stopx
				if showNumber {
					switch {
					case bidi && startx < stopx:
						x1 = startx + 2*sequenceNumberRadius
					case bidi:
						x1 = startx - sequenceNumberRadius
					case reverse && stopx > startx:
						x2 = stopx - 2*sequenceNumberRadius
					case reverse:
						x2 = stopx - sequenceNumberRadius
					default:
						x1 = startx + sequenceNumberRadius
					}
				}
				path = []Point{
					{X: float32(x1), Y: float32(lineStartY)},
					{X: float32(x2), Y: float32(lineStartY)},
				}
			}

			textMidX := math.Round(math.Min(startx, stopx) + math.Abs(stopx-startx)/2)
			var msgTexts []Text
			for i, line := range lines {
				msgTexts = append(msgTexts, Text{
					Kind:     TextMessage,
					X:        float32(textMidX),
					Y:        float32(textLineY(msgStarty+10, i, lineH, cfg.WrapPadding)),
					Anchor:   AnchorMiddle,
					Baseline: BaselineMiddle,
					DY:       float32(cfg.FontSize), // dy="1em"
					Size:     float32(cfg.FontSize),
					Weight:   cfg.MessageFontWeight,
					Text:     line,
				})
			}

			var seqNum *SequenceNumber
			if showNumber {
				numStr := mathRoundStr(sequenceIndex)
				leftToRight := startx <= stopx
				var badgeX float64
				switch {
				case self:
					badgeX = mm.fromBounds + 1
				case reverse && leftToRight, !reverse && !leftToRight:
					badgeX = mm.toBounds - 1
				default:
					badgeX = mm.fromBounds + 1
				}
				size := float32(12)
				if len(numStr) > 5 {
					size = 7
				} else if len(numStr) > 3 {
					size = 9
				}
				seqNum = &SequenceNumber{
					Center: Point{X: float32(badgeX), Y: float32(lineStartY)},
					Radius: sequenceNumberRadius,
					Text: Text{
						Kind:     TextSequenceNumber,
						X:        float32(badgeX),
						Y:        float32(lineStartY + 4),
						Anchor:   AnchorMiddle,
						Baseline: BaselineAlphabetic,
						Size:     size,
						Text:     numStr,
					},
				}
			}
			// Every message counts, shown or not.
			sequenceIndex = math.Round((sequenceIndex+sequenceIndexStep)*100) / 100

			// Markers
			endMarker := MarkerNone
			startMarker := MarkerNone
			switch msg.ArrowType {
			case parse.ArrowTypeSolid, parse.ArrowTypeDashed:
				endMarker = MarkerArrow
			case parse.ArrowTypeSolidCross, parse.ArrowTypeDashedCross:
				endMarker = MarkerCross
			case parse.ArrowTypeSolidOpen, parse.ArrowTypeDashedOpen:
				endMarker = MarkerOpen
			case parse.ArrowTypeSolidBi, parse.ArrowTypeDashedBi:
				startMarker = MarkerArrow
				endMarker = MarkerArrow
			case parse.ArrowTypeSolidTop, parse.ArrowTypeSolidTopDashed:
				endMarker = MarkerSolidTop
			case parse.ArrowTypeSolidBottom, parse.ArrowTypeSolidBottomDashed:
				endMarker = MarkerSolidBottom
			case parse.ArrowTypeStickTop, parse.ArrowTypeStickTopDashed:
				endMarker = MarkerStickTop
			case parse.ArrowTypeStickBottom, parse.ArrowTypeStickBottomDashed:
				endMarker = MarkerStickBottom
			}

			out.Messages = append(out.Messages, Message{
				From:        msg.From,
				To:          msg.To,
				Arrow:       msg.ArrowType,
				Dashed:      msg.ArrowType.IsDashed(),
				Path:        path,
				Curved:      curved,
				StartMarker: startMarker,
				EndMarker:   endMarker,
				Text:        msgTexts,
				Number:      seqNum,
				Line:        ev.line,
			})
		}
	}

	// 4. Header actors
	for _, key := range actorKeys {
		a := actors[key]
		out.Actors = append(out.Actors, actorGeometry(a, a.starty, false, cfg))
	}

	// 5. Footer actors and lifelines
	if cfg.MirrorActors {
		bounds.bumpVerticalPos(cfg.BoxMargin * 2)
		for _, key := range actorKeys {
			a := actors[key]
			if !a.destroyed {
				a.stopy = bounds.verticalPos
			}
			out.Actors = append(out.Actors, actorGeometry(a, a.stopy, true, cfg))
			out.Lifelines = append(out.Lifelines, Lifeline{
				ID: a.id,
				X:  float32(a.x + a.width/2),
				Y1: float32(a.starty + a.height),
				Y2: float32(a.stopy),
			})
		}
		bounds.bumpVerticalPos(rowMaxHeight + cfg.BoxMargin)
	} else {
		for _, key := range actorKeys {
			a := actors[key]
			y2 := bounds.verticalPos + a.height/2
			if a.destroyed {
				y2 = a.stopy + a.height/2
			}
			out.Lifelines = append(out.Lifelines, Lifeline{
				ID: a.id,
				X:  float32(a.x + a.width/2),
				Y1: float32(a.starty + a.height),
				Y2: float32(y2),
			})
		}
	}

	// 6. Finalize Boxes
	for _, bm := range boxModels {
		bm.h = bounds.verticalPos - bm.y
		bounds.insert(bm.x, bm.y, bm.x+bm.w, bounds.verticalPos)

		padding := cfg.BoxMargin * 2
		x := bm.x - padding
		y := bm.y - padding*0.25
		w := bm.w + 2*padding
		h := bm.h + padding*0.75

		var lblTexts []Text
		if bm.label != "" {
			lblTexts = centredLines(bm.label, bm.x+bm.w/2, bm.y+cfg.BoxTextMargin+maxBoxTextHeight/2, TextBoxLabel, cfg)
		}

		out.Boxes = append(out.Boxes, Box{
			Rect:  Rect{X: float32(x), Y: float32(y), W: float32(w), H: float32(h)},
			Fill:  bm.fill,
			Label: lblTexts,
			Line:  bm.line,
		})
	}
	if hasBoxes {
		bounds.bumpVerticalPos(cfg.BoxMargin)
	}

	// 7. Finalize Frames, as Mermaid's drawLoop
	// fallbackX spans every actor, for a block with nothing in it.
	fallbackX := func(lp *loopBounds) (float64, float64) {
		if lp.startx != nil && lp.stopx != nil {
			return *lp.startx, *lp.stopx
		}
		var x0, x1 float64
		if bounds.startx != nil && bounds.stopx != nil {
			x0, x1 = *bounds.startx+cfg.BoxMargin, *bounds.stopx-cfg.BoxMargin
		}
		return x0, x1
	}
	loopY := func(lp *loopBounds) (float64, float64) {
		y0 := *lp.starty
		y1 := y0
		if lp.stopy != nil {
			y1 = *lp.stopy
		}
		return y0, y1
	}

	tabW := math.Max(cfg.LabelBoxWidth, 50)
	tabH := cfg.LabelBoxHeight
	if cfg.Look == LookNeo {
		tabH += 15
	}
	for _, fm := range finishedFrames {
		startx, stopx := fallbackX(fm.loop)
		starty, stopy := loopY(fm.loop)

		const cut = 7.0
		tab := []Point{
			{X: float32(startx), Y: float32(starty)},
			{X: float32(startx + tabW), Y: float32(starty)},
			{X: float32(startx + tabW), Y: float32(starty + tabH - cut)},
			{X: float32(startx + tabW - cut*1.2), Y: float32(starty + tabH)},
			{X: float32(startx), Y: float32(starty + tabH)},
		}

		frameTexts := []Text{{
			Kind:     TextFrameKeyword,
			X:        float32(math.Round(startx + tabW/2)),
			Y:        float32(textLineY(starty+tabH/2, 0, 0, cfg.BoxTextMargin)),
			Anchor:   AnchorMiddle,
			Baseline: BaselineMiddle,
			Size:     float32(cfg.FontSize),
			Weight:   700,
			Text:     fm.kind.String(),
		}}
		addLines := func(kind TextKind, label string, x, y float64) {
			for i, line := range splitLineBreaks(label) {
				frameTexts = append(frameTexts, Text{
					Kind:     kind,
					X:        float32(x),
					Y:        float32(textLineY(y, i, lineHeight, cfg.BoxTextMargin)),
					Anchor:   AnchorMiddle,
					Baseline: BaselineAlphabetic,
					Size:     float32(cfg.FontSize),
					Weight:   cfg.MessageFontWeight,
					Text:     line,
				})
			}
		}
		addLines(TextFrameTitle, fm.label, startx+cfg.LabelBoxWidth/2+(stopx-startx)/2, starty+cfg.BoxMargin+cfg.BoxTextMargin)

		var dividers []float32
		for _, sec := range fm.sections {
			dividers = append(dividers, float32(sec.y))
			if sec.label != "" {
				addLines(TextSectionTitle, sec.label, startx+(stopx-startx)/2, sec.y+cfg.BoxMargin+cfg.BoxTextMargin)
			}
		}

		out.Frames = append(out.Frames, Frame{
			Kind:     fm.kind,
			Rect:     Rect{X: float32(startx), Y: float32(starty), W: float32(stopx - startx), H: float32(stopy - starty)},
			Tab:      tab,
			Dividers: dividers,
			Texts:    frameTexts,
			Line:     fm.line,
		})
	}

	// 8. Finalize Rect Backgrounds
	for _, rm := range finishedRects {
		startx, stopx := fallbackX(rm.loop)
		starty, stopy := loopY(rm.loop)
		out.Rects = append(out.Rects, Background{
			Rect: Rect{X: float32(startx), Y: float32(starty), W: float32(stopx - startx), H: float32(stopy - starty)},
			Fill: rm.fill,
			Line: rm.line,
		})
	}

	// 9. Diagram Title, start-anchored where Mermaid puts it.
	if d.Title != "" {
		startX := 0.0
		stopX := 0.0
		if bounds.startx != nil {
			startX = *bounds.startx
		}
		if bounds.stopx != nil {
			stopX = *bounds.stopx
		}
		out.Title = &Text{
			Kind:     TextTitle,
			X:        float32((stopX-startX)/2 - 2*cfg.DiagramMarginX),
			Y:        -25,
			Anchor:   AnchorStart,
			Baseline: BaselineAlphabetic,
			Size:     float32(cfg.FontSize),
			Weight:   700,
			Text:     d.Title,
		}
	}

	// 10. ViewBox calculation
	bStartX := 0.0
	bStopX := 0.0
	bStartY := 0.0
	bStopY := 0.0
	if bounds.startx != nil {
		bStartX = *bounds.startx
	}
	if bounds.stopx != nil {
		bStopX = *bounds.stopx
	}
	if bounds.starty != nil {
		bStartY = *bounds.starty
	}
	if bounds.stopy != nil {
		bStopY = *bounds.stopy
	}

	boxW := bStopX - bStartX
	boxH := bStopY - bStartY

	w := boxW + 2*cfg.DiagramMarginX
	h := boxH + 2*cfg.DiagramMarginY
	if cfg.MirrorActors {
		h = h - cfg.BoxMargin + cfg.BottomMarginAdj
	}

	extraVertForTitle := 0.0
	if d.Title != "" {
		extraVertForTitle = 40.0
	}
	extraHeightForNeo := 0.0
	if len(actors) > 0 && cfg.Look == LookNeo {
		extraHeightForNeo = 30.0
	}

	out.ViewBox = Rect{
		X: float32(bStartX - cfg.DiagramMarginX),
		Y: float32(-(cfg.DiagramMarginY + extraVertForTitle)),
		W: float32(w),
		H: float32(h + extraVertForTitle + extraHeightForNeo),
	}

	return out, nil
}

func flattenItems(items []parse.Item) []layoutEvent {
	var events []layoutEvent
	var create, destroy string
	for _, it := range items {
		switch {
		case it.Create != nil:
			create = *it.Create
		case it.Destroy != nil:
			destroy = *it.Destroy
		case it.Message != nil:
			events = append(events, layoutEvent{
				kind:    evMessage,
				msg:     it.Message,
				create:  create,
				destroy: destroy,
				line:    it.Line,
			})
			create, destroy = "", ""
			if it.Message.Lifeline == parse.LifelineEffectActivate {
				events = append(events, layoutEvent{
					kind: evActivation,
					activation: &parse.Activation{
						Participant: it.Message.To,
						Activate:    true,
					},
					line: it.Line,
				})
			} else if it.Message.Lifeline == parse.LifelineEffectDeactivate {
				events = append(events, layoutEvent{
					kind: evActivation,
					activation: &parse.Activation{
						Participant: it.Message.From,
						Activate:    false,
					},
					line: it.Line,
				})
			}
		case it.Note != nil:
			events = append(events, layoutEvent{
				kind: evNote,
				note: it.Note,
				line: it.Line,
			})
		case it.Activation != nil:
			events = append(events, layoutEvent{
				kind:       evActivation,
				activation: it.Activation,
				line:       it.Line,
			})
		case it.AutoNumber != nil:
			events = append(events, layoutEvent{
				kind:       evAutoNumber,
				autoNumber: it.AutoNumber,
				line:       it.Line,
			})
		case it.Block != nil:
			b := it.Block
			if b.Kind == parse.BlockKindRect {
				events = append(events, layoutEvent{
					kind:      evRectStart,
					fill:      b.Fill,
					line:      b.Line,
					blockKind: b.Kind,
				})
				events = append(events, flattenItems(b.Items)...)
				events = append(events, layoutEvent{
					kind:      evRectEnd,
					line:      b.EndLine,
					blockKind: b.Kind,
				})
			} else {
				events = append(events, layoutEvent{
					kind:      evBlockStart,
					block:     b,
					label:     b.Label,
					line:      b.Line,
					blockKind: b.Kind,
				})
				events = append(events, flattenItems(b.Items)...)
				for _, br := range b.Branches {
					events = append(events, layoutEvent{
						kind:      evBlockBranch,
						block:     b,
						label:     br.Label,
						line:      br.Line,
						blockKind: b.Kind,
					})
					events = append(events, flattenItems(br.Items)...)
				}
				events = append(events, layoutEvent{
					kind:      evBlockEnd,
					block:     b,
					line:      b.EndLine,
					blockKind: b.Kind,
				})
			}
		}
	}
	return events
}

func mathRoundStr(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// sequenceNumberRadius is the autonumber badge's radius.
const sequenceNumberRadius = 6.0

// actorTypeWidth is the width Mermaid gives the icon-shaped participants
// when a message stops at their edge.
const actorTypeWidth = 18 * 2

// edgeOffset is how far a create or destroy message stops short of the
// actor's centre.
func edgeOffset(a *actorLayoutData) float64 {
	switch a.kind {
	case parse.ParticipantKindActor, parse.ParticipantKindControl,
		parse.ParticipantKindEntity, parse.ParticipantKindDatabase:
		return actorTypeWidth / 2
	}
	return a.width / 2
}

// noArrowheadShortening reports the arrows Mermaid doesn't pull back from
// the lifeline: those with no head at their end.
func noArrowheadShortening(t parse.ArrowType) bool {
	switch t {
	case parse.ArrowTypeSolidNoHead, parse.ArrowTypeDashedNoHead,
		parse.ArrowTypeStickTop, parse.ArrowTypeStickBottom,
		parse.ArrowTypeStickTopDashed, parse.ArrowTypeStickBottomDashed:
		return true
	}
	return isReverseArrow(t)
}

// startArrowheadShortening reports the arrows with a head at their start.
func startArrowheadShortening(t parse.ArrowType) bool {
	switch t {
	case parse.ArrowTypeSolidBi, parse.ArrowTypeDashedBi,
		parse.ArrowTypeSolidTopReverse, parse.ArrowTypeSolidBottomReverse,
		parse.ArrowTypeSolidTopReverseDashed, parse.ArrowTypeSolidBottomReverseDashed:
		return true
	}
	return false
}

func isReverseArrow(t parse.ArrowType) bool {
	switch t {
	case parse.ArrowTypeSolidTopReverse, parse.ArrowTypeSolidBottomReverse,
		parse.ArrowTypeStickTopReverse, parse.ArrowTypeStickBottomReverse,
		parse.ArrowTypeSolidTopReverseDashed, parse.ArrowTypeSolidBottomReverseDashed,
		parse.ArrowTypeStickTopReverseDashed, parse.ArrowTypeStickBottomReverseDashed:
		return true
	}
	return false
}

// textLineY is the y of line i of a text block whose first line Mermaid's
// drawText centres at y with the given margin: each line moves down by
// the height of the ones before it.
func textLineY(y float64, i int, lineHeight, margin float64) float64 {
	return math.Round(y + float64(i)*lineHeight + margin/2)
}

// centredLines lays out label as Mermaid's byTspan does: one text per
// line, centred on (x, y), with the lines spread a font size apart.
func centredLines(label string, x, y float64, kind TextKind, cfg Config) []Text {
	lines := splitLineBreaks(label)
	var texts []Text
	for i, line := range lines {
		texts = append(texts, Text{
			Kind:     kind,
			X:        float32(x),
			Y:        float32(y),
			Anchor:   AnchorMiddle,
			Baseline: BaselineCentral,
			DY:       float32(float64(i)*cfg.FontSize - cfg.FontSize*float64(len(lines)-1)/2),
			Size:     float32(cfg.FontSize),
			Weight:   cfg.ActorFontWeight,
			Text:     line,
		})
	}
	return texts
}

// Stick-figure geometry from Mermaid's drawActorTypeActor, in units down
// from the top of its classic box.
const (
	actorGlyphTop    = -5.0
	actorGlyphBottom = 60.0
	actorGlyphHeight = actorGlyphBottom - actorGlyphTop
)

// actorGeometry is a participant's head (or, for footer, foot) at y.
func actorGeometry(a *actorLayoutData, y float64, footer bool, cfg Config) Actor {
	radius := float32(3)
	if cfg.Look == LookNeo {
		radius = 6
	}
	act := Actor{
		ID:     a.id,
		Kind:   a.kind,
		Footer: footer,
		Rect:   Rect{X: float32(a.x), Y: float32(y), W: float32(a.width), H: float32(a.height)},
		Radius: radius,
		Line:   a.line,
	}
	center := a.x + a.width/2
	labelY := y + a.height/2

	if a.kind == parse.ParticipantKindActor {
		// Under neo the figure fills the glyph band above its label;
		// classic draws it at the legacy coordinates.
		scale := 1.0
		glyphBottom := y + actorGlyphBottom
		labelY = y + 35 + a.height/2
		if cfg.Look == LookNeo {
			scale = GlyphBandHeight / actorGlyphHeight
			if footer {
				glyphBottom = y + LabelLifelineGap + GlyphBandHeight
				labelY = glyphBottom + GlyphLabelGap + a.textHeight/2
			} else {
				lifelineStart := y + a.height
				glyphBottom = lifelineStart - LabelLifelineGap - a.textHeight - GlyphLabelGap
				labelY = lifelineStart - LabelLifelineGap - a.textHeight/2
			}
		}
		gy := func(o float64) float32 { return float32(glyphBottom - (actorGlyphBottom-o)*scale) }
		gx := func(o float64) float32 { return float32(center + o*scale) }
		c := float32(center)
		act.Figure = &StickFigure{
			Head:   Point{X: c, Y: gy(10)},
			Radius: float32(15 * scale),
			Lines: [][2]Point{
				{{X: c, Y: gy(25)}, {X: c, Y: gy(45)}},
				{{X: gx(-actorTypeWidth / 2), Y: gy(33)}, {X: gx(actorTypeWidth / 2), Y: gy(33)}},
				{{X: gx(-actorTypeWidth / 2), Y: gy(60)}, {X: c, Y: gy(45)}},
				{{X: c, Y: gy(45)}, {X: gx(actorTypeWidth/2 - 2), Y: gy(60)}},
			},
		}
	}

	act.Label = centredLines(a.alias, center, labelY, TextActor, cfg)
	return act
}
