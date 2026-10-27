package layout

import (
	"math"
	"strconv"
	"strings"

	"dyamark/seq/parse"
)

// Constants from Mermaid 12.0.0 actorBands.ts
const (
	GlyphBandHeight  = 44.0
	GlyphLabelGap    = 6.0
	LabelLifelineGap = 6.0
)

type actorLayoutData struct {
	id          string
	name        string
	kind        parse.ParticipantKind
	alias       string
	x           float64
	y           float64
	stopy       float64
	width       float64
	height      float64
	margin      float64
	textHeight  float64
	boxIndex    int
	prevActor   string
	nextActor   string
	line        int
	activations []activationEntry
}

type activationEntry struct {
	startx float64
	starty float64
	stopx  float64
	stopy  float64
	depth  int
}

type boundsTracker struct {
	startx      *float64
	stopx       *float64
	starty      *float64
	stopy       *float64
	verticalPos float64
	boxMargin   float64
}

func (b *boundsTracker) bumpVerticalPos(bump float64) {
	b.verticalPos += bump
	if b.stopy == nil || b.verticalPos > *b.stopy {
		val := b.verticalPos
		b.stopy = &val
	}
}

func (b *boundsTracker) insert(x1, y1, x2, y2 float64) {
	minX := math.Min(x1, x2)
	maxX := math.Max(x1, x2)
	minY := math.Min(y1, y2)
	maxY := math.Max(y1, y2)

	if b.startx == nil || minX < *b.startx {
		val := minX
		b.startx = &val
	}
	if b.stopx == nil || maxX > *b.stopx {
		val := maxX
		b.stopx = &val
	}
	if b.starty == nil || minY < *b.starty {
		val := minY
		b.starty = &val
	}
	if b.stopy == nil || maxY > *b.stopy {
		val := maxY
		b.stopy = &val
	}
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
	line       int
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

		a.x = prevWidth + prevMargin
		a.y = bounds.verticalPos

		bounds.insert(a.x, bounds.verticalPos, a.x+a.width, a.y+a.height)

		prevWidth += a.width + prevMargin
		if boxIdx >= 0 {
			boxModels[boxIdx].w = prevWidth + boxModels[boxIdx].margin - boxModels[boxIdx].x
		}
		prevMargin = a.margin
		prevBoxIdx = boxIdx
	}

	bounds.bumpVerticalPos(rowMaxHeight)

	// Helper to get activation bounds for an actor
	getActivationBounds := func(actID string) (float64, float64) {
		act := actors[actID]
		if act == nil {
			return 0, 0
		}
		mid := act.x + act.width/2
		left := mid - 1
		right := mid + 1
		for _, e := range act.activations {
			if e.stopy == 0 { // currently active
				left = math.Min(left, e.startx)
				right = math.Max(right, e.stopx)
			}
		}
		return left, right
	}

	getActiveCount := func(actID string) int {
		act := actors[actID]
		if act == nil {
			return 0
		}
		cnt := 0
		for _, e := range act.activations {
			if e.stopy == 0 {
				cnt++
			}
		}
		return cnt
	}

	// 2. Loop bounds precalculation
	type loopWidthModel struct {
		from  float64
		to    float64
		width float64
	}
	loopWidths := make(map[string]*loopWidthModel)
	var loopStack []*loopWidthModel

	for _, ev := range events {
		switch ev.kind {
		case evBlockStart:
			lwm := &loopWidthModel{from: math.MaxFloat64, to: -math.MaxFloat64}
			loopStack = append(loopStack, lwm)
			loopWidths[ev.label] = lwm
		case evBlockEnd:
			if len(loopStack) > 0 {
				loopStack = loopStack[:len(loopStack)-1]
			}
		case evMessage:
			msg := ev.msg
			fa, ta := actors[msg.From], actors[msg.To]
			if fa != nil && ta != nil && len(loopStack) > 0 {
				fromLeft, fromRight := getActivationBounds(msg.From)
				toLeft, toRight := getActivationBounds(msg.To)
				isArrowToRight := fromLeft <= toLeft
				startx := fromLeft
				if isArrowToRight {
					startx = fromRight
				}
				stopx := toRight
				if isArrowToRight {
					stopx = toLeft
				}
				boundedWidth := math.Abs(startx - stopx)
				msgW := math.Max(boundedWidth+2*cfg.WrapPadding, cfg.Width)
				for _, lwm := range loopStack {
					lwm.from = math.Min(lwm.from, math.Min(startx, stopx))
					lwm.to = math.Max(lwm.to, math.Max(startx, stopx))
					lwm.width = math.Max(lwm.width, msgW) - cfg.LabelBoxWidth
				}
			}
		case evNote:
			n := ev.note
			if len(n.Participants) > 0 && len(loopStack) > 0 {
				fa := actors[n.Participants[0]]
				if fa != nil {
					for _, lwm := range loopStack {
						lwm.from = math.Min(lwm.from, fa.x)
						lwm.to = math.Max(lwm.to, fa.x+cfg.Width)
						lwm.width = math.Max(lwm.width, math.Abs(lwm.to-lwm.from)) - cfg.LabelBoxWidth
					}
				}
			}
		}
	}

	// 3. Process events
	type frameModel struct {
		kind     parse.BlockKind
		starty   float64
		stopy    float64
		minX     float64
		maxX     float64
		title    string
		dividers []float64
		titles   []string
		line     int
	}
	var openFrames []*frameModel
	var finishedFrames []*frameModel

	type rectBlockModel struct {
		starty float64
		stopy  float64
		fill   string
		line   int
	}
	var openRects []*rectBlockModel
	var finishedRects []*rectBlockModel

	sequenceIndex := 1.0
	sequenceIndexStep := 1.0
	sequenceNumbersVisible := cfg.ShowSequenceNumbers

	for _, ev := range events {
		switch ev.kind {
		case evAutoNumber:
			if ev.autoNumber.Start > 0 {
				sequenceIndex = ev.autoNumber.Start
			}
			if ev.autoNumber.Step > 0 {
				sequenceIndexStep = ev.autoNumber.Step
			}
			sequenceNumbersVisible = ev.autoNumber.Visible

		case evActivation:
			act := actors[ev.activation.Participant]
			if act == nil {
				continue
			}
			if ev.activation.Activate {
				stacked := getActiveCount(act.id)
				mid := act.x + act.width/2
				x := mid + float64(stacked-1)*cfg.ActivationWidth/2
				if stacked == 0 {
					x = mid - cfg.ActivationWidth/2
				}
				act.activations = append(act.activations, activationEntry{
					startx: x,
					starty: bounds.verticalPos + 2,
					stopx:  x + cfg.ActivationWidth,
					depth:  stacked,
				})
			} else {
				// Deactivate
				for i := len(act.activations) - 1; i >= 0; i-- {
					if act.activations[i].stopy == 0 {
						if act.activations[i].starty+18 > bounds.verticalPos {
							act.activations[i].starty = bounds.verticalPos - 6
							bounds.verticalPos += 12
						}
						act.activations[i].stopy = bounds.verticalPos
						bounds.insert(act.activations[i].startx, bounds.verticalPos-10, act.activations[i].stopx, bounds.verticalPos)
						out.Activations = append(out.Activations, Activation{
							ID: act.id,
							Rect: Rect{
								X: float32(act.activations[i].startx),
								Y: float32(act.activations[i].starty),
								W: float32(act.activations[i].stopx - act.activations[i].startx),
								H: float32(act.activations[i].stopy - act.activations[i].starty),
							},
							Depth: act.activations[i].depth,
						})
						break
					}
				}
			}

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

			bounds.bumpVerticalPos(cfg.BoxMargin)
			starty := bounds.verticalPos

			noteLabel := n.Text
			if n.Wrap {
				noteLabel = m.WrapLabel(noteLabel, cfg.Width-2*cfg.NoteMargin, cfg.NoteFont())
			}
			noteDims := m.MeasureText(noteLabel, cfg.NoteFont())
			noteW := math.Max(cfg.Width, noteDims.Width+2*cfg.NoteMargin)
			var startx float64

			switch n.Position {
			case parse.NotePositionRight:
				noteW = math.Max((fromAct.width+toAct.width)/2, noteDims.Width+2*cfg.NoteMargin)
				startx = fromAct.x + (fromAct.width+cfg.ActorMargin)/2
			case parse.NotePositionLeft:
				noteW = math.Max((fromAct.width+toAct.width)/2, noteDims.Width+2*cfg.NoteMargin)
				startx = fromAct.x - noteW + (fromAct.width-cfg.ActorMargin)/2
			case parse.NotePositionOver:
				if fromID == toID {
					noteW = math.Max(fromAct.width, noteDims.Width+2*cfg.NoteMargin)
					startx = fromAct.x + fromAct.width/2 - noteW/2
				} else {
					fromCenter := fromAct.x + fromAct.width/2
					toCenter := toAct.x + toAct.width/2
					noteW = math.Abs(fromCenter-toCenter) + cfg.ActorMargin
					if fromAct.x < toAct.x {
						startx = fromCenter - cfg.ActorMargin/2
					} else {
						startx = toCenter - cfg.ActorMargin/2
					}
				}
			}

			noteH := noteDims.Height + 2*cfg.NoteMargin
			stopy := starty + noteH
			bounds.bumpVerticalPos(noteH)
			bounds.insert(startx, starty, startx+noteW, stopy)

			// Texts in note
			lines := splitLineBreaks(noteLabel)
			var texts []Text
			curY := starty + cfg.NoteMargin
			for _, line := range lines {
				texts = append(texts, Text{
					Kind:     TextNote,
					X:        float32(startx + noteW/2),
					Y:        float32(curY + noteDims.LineHeight/2),
					Anchor:   AnchorMiddle,
					Baseline: BaselineMiddle,
					Size:     float32(cfg.FontSize),
					Weight:   cfg.NoteFontWeight,
					Text:     line,
				})
				curY += noteDims.LineHeight
			}

			out.Notes = append(out.Notes, Note{
				Rect: Rect{
					X: float32(startx),
					Y: float32(starty),
					W: float32(noteW),
					H: float32(noteH),
				},
				Text: texts,
				Line: ev.line,
			})

		case evBlockStart:
			bounds.bumpVerticalPos(cfg.BoxMargin)
			fm := &frameModel{
				kind:   ev.blockKind,
				starty: bounds.verticalPos,
				title:  ev.label,
				line:   ev.line,
				minX:   math.MaxFloat64,
				maxX:   -math.MaxFloat64,
			}
			openFrames = append(openFrames, fm)
			totalOffset := 0.0
			if ev.label != "" {
				labelToMeasure := "[" + ev.label + "]"
				if lwm, ok := loopWidths[ev.label]; ok && lwm.width > 0 {
					labelToMeasure = m.WrapLabel(labelToMeasure, lwm.width-2*cfg.WrapPadding, cfg.MessageFont())
				}
				textDims := m.MeasureText(labelToMeasure, cfg.MessageFont())
				totalOffset = math.Max(textDims.Height, cfg.LabelBoxHeight)
			}
			bounds.bumpVerticalPos(cfg.BoxMargin + cfg.BoxTextMargin + totalOffset)

		case evBlockBranch:
			if len(openFrames) > 0 {
				top := openFrames[len(openFrames)-1]
				top.dividers = append(top.dividers, bounds.verticalPos)
				top.titles = append(top.titles, ev.label)
				bounds.bumpVerticalPos(cfg.BoxMargin + cfg.BoxTextMargin)
				totalOffset := 0.0
				if ev.label != "" {
					textDims := m.MeasureText(ev.label, cfg.MessageFont())
					totalOffset = math.Max(textDims.Height, cfg.LabelBoxHeight)
				}
				bounds.bumpVerticalPos(cfg.BoxMargin + totalOffset)
			}

		case evBlockEnd:
			if len(openFrames) > 0 {
				top := openFrames[len(openFrames)-1]
				openFrames = openFrames[:len(openFrames)-1]
				top.stopy = bounds.verticalPos + cfg.BoxMargin
				bounds.bumpVerticalPos(cfg.BoxMargin)
				finishedFrames = append(finishedFrames, top)
			}

		case evRectStart:
			bounds.bumpVerticalPos(cfg.BoxMargin)
			openRects = append(openRects, &rectBlockModel{
				starty: bounds.verticalPos,
				fill:   ev.fill,
				line:   ev.line,
			})
			bounds.bumpVerticalPos(cfg.BoxMargin)

		case evRectEnd:
			if len(openRects) > 0 {
				top := openRects[len(openRects)-1]
				openRects = openRects[:len(openRects)-1]
				top.stopy = bounds.verticalPos + cfg.BoxMargin
				bounds.bumpVerticalPos(cfg.BoxMargin)
				finishedRects = append(finishedRects, top)
			}

		case evMessage:
			msg := ev.msg
			fromAct := actors[msg.From]
			toAct := actors[msg.To]
			if fromAct == nil || toAct == nil {
				continue
			}

			bounds.bumpVerticalPos(10)
			msgStarty := bounds.verticalPos

			fromLeft, fromRight := getActivationBounds(msg.From)
			toLeft, toRight := getActivationBounds(msg.To)
			isArrowToRight := fromLeft <= toLeft

			startx := fromLeft
			if isArrowToRight {
				startx = fromRight
			}
			stopx := toRight
			if isArrowToRight {
				stopx = toLeft
			}

			if cfg.Look == LookNeo {
				offset := 3.0
				if msg.ArrowType != parse.ArrowTypeSolidNoHead {
					if isArrowToRight {
						stopx -= offset
					} else {
						stopx += offset
					}
				}
				if msg.ArrowType.IsBidirectional() {
					if isArrowToRight {
						startx += offset
					} else {
						startx -= offset
					}
				}
			}

			isArrowToActivation := math.Abs(toLeft-toRight) > 2
			if msg.Lifeline == parse.LifelineEffectActivate && !isArrowToActivation {
				adjust := cfg.ActivationWidth/2 - 1
				if isArrowToRight {
					stopx -= adjust
				} else {
					stopx += adjust
				}
			}

			isStickArrow := msg.ArrowType == parse.ArrowTypeStickTop ||
				msg.ArrowType == parse.ArrowTypeStickBottom ||
				msg.ArrowType == parse.ArrowTypeStickTopDashed ||
				msg.ArrowType == parse.ArrowTypeStickBottomDashed ||
				msg.ArrowType == parse.ArrowTypeStickTopReverse ||
				msg.ArrowType == parse.ArrowTypeStickBottomReverse ||
				msg.ArrowType == parse.ArrowTypeStickTopReverseDashed ||
				msg.ArrowType == parse.ArrowTypeStickBottomReverseDashed

			if msg.ArrowType.HasArrowHead() && !isStickArrow && msg.From != msg.To {
				if isArrowToRight {
					stopx -= 3.0
				} else {
					stopx += 3.0
				}
				if msg.ArrowType.IsBidirectional() {
					if isArrowToRight {
						startx += 3.0
					} else {
						startx -= 3.0
					}
				}
			}

			if sequenceNumbersVisible {
				startx += 6.0
			}

			boundedWidth := math.Abs(startx - stopx)
			msgLabel := msg.Label
			if msg.Wrap {
				msgLabel = m.WrapLabel(msgLabel, math.Max(boundedWidth+2*cfg.WrapPadding, cfg.Width), cfg.MessageFont())
			}
			textDims := m.MeasureText(msgLabel, cfg.MessageFont())

			lines := splitLineBreaks(msgLabel)
			lineH := textDims.Height
			if len(lines) > 0 {
				lineH = textDims.Height / float64(len(lines))
			}
			bounds.bumpVerticalPos(lineH)

			totalOffset := textDims.Height - 10
			var lineStartY float64

			var path []Point
			curved := false

			if msg.From == msg.To {
				lineStartY = bounds.verticalPos + totalOffset
				if !cfg.RightAngles {
					totalOffset += cfg.BoxMargin
					lineStartY = bounds.verticalPos + totalOffset
				}
				totalOffset += 30
				dx := math.Max(textDims.Width/2, cfg.Width/2)
				bounds.insert(startx-dx, bounds.verticalPos-10+totalOffset, startx+dx, bounds.verticalPos+30+totalOffset)

				curved = !cfg.RightAngles
				if curved {
					path = []Point{
						{X: float32(startx), Y: float32(lineStartY)},
						{X: float32(startx + 50), Y: float32(lineStartY - 10)},
						{X: float32(startx + 50), Y: float32(lineStartY + 30)},
						{X: float32(startx), Y: float32(lineStartY + 20)},
					}
				} else {
					path = []Point{
						{X: float32(startx), Y: float32(lineStartY)},
						{X: float32(startx + 50), Y: float32(lineStartY)},
						{X: float32(startx + 50), Y: float32(lineStartY + 20)},
						{X: float32(startx), Y: float32(lineStartY + 20)},
					}
				}
			} else {
				totalOffset += cfg.BoxMargin
				lineStartY = bounds.verticalPos + totalOffset
				bounds.insert(startx, lineStartY-10, stopx, lineStartY)

				path = []Point{
					{X: float32(startx), Y: float32(lineStartY)},
					{X: float32(stopx), Y: float32(lineStartY)},
				}
			}

			bounds.bumpVerticalPos(totalOffset)

			// Text placement
			textMinX := math.Min(startx, stopx)
			textW := math.Abs(stopx - startx)
			textMidX := math.Round(textMinX + textW/2)
			if msg.From == msg.To {
				textMidX = math.Round(startx + 25)
			}

			var msgTexts []Text
			curY := msgStarty + 10
			for _, line := range lines {
				msgTexts = append(msgTexts, Text{
					Kind:     TextMessage,
					X:        float32(textMidX),
					Y:        float32(curY),
					Anchor:   AnchorMiddle,
					Baseline: BaselineMiddle,
					DY:       16, // 1em SVG dy equivalent
					Size:     float32(cfg.FontSize),
					Weight:   cfg.MessageFontWeight,
					Text:     line,
				})
				curY += lineH
			}

			// Autonumber badge
			var seqNum *SequenceNumber
			if sequenceNumbersVisible {
				numVal := math.Round(sequenceIndex*100) / 100
				numStr := strings.TrimRight(strings.TrimRight(strings.ReplaceAll(string(rune(int(numVal)))+"", "\x00", ""), "0"), ".")
				if numVal == math.Floor(numVal) {
					numStr = strings.TrimSpace(strings.ReplaceAll(string(rune(int(numVal)))+"", "\x00", ""))
					numStr = strings.TrimLeft(strings.TrimRight(string([]byte{byte('0' + int(numVal)%10)}), " "), " ")
					// Standard formatting
					numStr = mathRoundStr(numVal)
				} else {
					numStr = mathRoundStr(numVal)
				}

				badgeX := float32(math.Min(startx, stopx) + 10)
				badgeY := float32(lineStartY - 10)
				seqNum = &SequenceNumber{
					Center: Point{X: badgeX, Y: badgeY},
					Radius: 6,
					Text: Text{
						Kind:     TextSequenceNumber,
						X:        badgeX,
						Y:        badgeY,
						Anchor:   AnchorMiddle,
						Baseline: BaselineCentral,
						Size:     10,
						Text:     numStr,
					},
				}
				sequenceIndex = math.Round((sequenceIndex+sequenceIndexStep)*100) / 100
			}

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
		rect := Rect{
			X: float32(a.x),
			Y: float32(a.y),
			W: float32(a.width),
			H: float32(a.height),
		}
		var fig *StickFigure
		if a.kind == parse.ParticipantKindActor {
			headCenter := Point{X: float32(a.x + a.width/2), Y: float32(a.y + 15)}
			fig = &StickFigure{
				Head:   headCenter,
				Radius: 10,
				Lines: [][2]Point{
					{headCenter, {X: headCenter.X, Y: headCenter.Y + 20}},
					{{X: headCenter.X - 15, Y: headCenter.Y + 10}, {X: headCenter.X + 15, Y: headCenter.Y + 10}},
					{{X: headCenter.X, Y: headCenter.Y + 20}, {X: headCenter.X - 12, Y: headCenter.Y + 35}},
					{{X: headCenter.X, Y: headCenter.Y + 20}, {X: headCenter.X + 12, Y: headCenter.Y + 35}},
				},
			}
		}

		lines := splitLineBreaks(a.alias)
		var texts []Text
		curY := a.y + a.height/2 - float64(len(lines))*a.textHeight/2
		for _, line := range lines {
			texts = append(texts, Text{
				Kind:     TextActor,
				X:        float32(a.x + a.width/2),
				Y:        float32(curY + a.textHeight/2),
				Anchor:   AnchorMiddle,
				Baseline: BaselineCentral,
				Size:     float32(cfg.FontSize),
				Weight:   cfg.ActorFontWeight,
				Text:     line,
			})
			curY += a.textHeight
		}

		radius := float32(3)
		if cfg.Look == LookNeo {
			radius = 6
		}

		out.Actors = append(out.Actors, Actor{
			ID:     a.id,
			Kind:   a.kind,
			Footer: false,
			Rect:   rect,
			Radius: radius,
			Figure: fig,
			Label:  texts,
			Line:   a.line,
		})
	}

	// 5. Footer actors and lifelines
	if cfg.MirrorActors {
		bounds.bumpVerticalPos(cfg.BoxMargin * 2)
		for _, key := range actorKeys {
			a := actors[key]
			a.stopy = bounds.verticalPos

			rect := Rect{
				X: float32(a.x),
				Y: float32(a.stopy),
				W: float32(a.width),
				H: float32(a.height),
			}
			var fig *StickFigure
			if a.kind == parse.ParticipantKindActor {
				headCenter := Point{X: float32(a.x + a.width/2), Y: float32(a.stopy + 15)}
				fig = &StickFigure{
					Head:   headCenter,
					Radius: 10,
				}
			}

			lines := splitLineBreaks(a.alias)
			var texts []Text
			curY := a.stopy + a.height/2 - float64(len(lines))*a.textHeight/2
			for _, line := range lines {
				texts = append(texts, Text{
					Kind:     TextActor,
					X:        float32(a.x + a.width/2),
					Y:        float32(curY + a.textHeight/2),
					Anchor:   AnchorMiddle,
					Baseline: BaselineCentral,
					Size:     float32(cfg.FontSize),
					Weight:   cfg.ActorFontWeight,
					Text:     line,
				})
				curY += a.textHeight
			}

			radius := float32(3)
			if cfg.Look == LookNeo {
				radius = 6
			}

			out.Actors = append(out.Actors, Actor{
				ID:     a.id,
				Kind:   a.kind,
				Footer: true,
				Rect:   rect,
				Radius: radius,
				Figure: fig,
				Label:  texts,
				Line:   a.line,
			})

			out.Lifelines = append(out.Lifelines, Lifeline{
				ID: a.id,
				X:  float32(a.x + a.width/2),
				Y1: float32(a.y + a.height),
				Y2: float32(a.stopy),
			})
		}
		bounds.bumpVerticalPos(rowMaxHeight + cfg.BoxMargin)
	} else {
		for _, key := range actorKeys {
			a := actors[key]
			out.Lifelines = append(out.Lifelines, Lifeline{
				ID: a.id,
				X:  float32(a.x + a.width/2),
				Y1: float32(a.y + a.height),
				Y2: float32(bounds.verticalPos + a.height/2),
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
			lblTexts = append(lblTexts, Text{
				Kind:     TextBoxLabel,
				X:        float32(x + w/2),
				Y:        float32(y + 15),
				Anchor:   AnchorMiddle,
				Baseline: BaselineMiddle,
				Size:     float32(cfg.FontSize),
				Weight:   cfg.MessageFontWeight,
				Text:     bm.label,
			})
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

	// 7. Finalize Frames
	for _, fm := range finishedFrames {
		// Frame covers all actors by default or enclosed items
		frameX := bounds.startx
		frameStopX := bounds.stopx
		if frameX == nil || frameStopX == nil {
			continue
		}
		x := *frameX + cfg.BoxMargin
		w := (*frameStopX - *frameX) - 2*cfg.BoxMargin
		y := fm.starty
		h := fm.stopy - fm.starty

		tabW := cfg.LabelBoxWidth
		tabH := cfg.LabelBoxHeight
		tab := []Point{
			{X: float32(x), Y: float32(y)},
			{X: float32(x + tabW), Y: float32(y)},
			{X: float32(x + tabW), Y: float32(y + tabH - 6)},
			{X: float32(x + tabW - 6), Y: float32(y + tabH)},
			{X: float32(x), Y: float32(y + tabH)},
		}

		var frameTexts []Text
		keyword := fm.kind.String()
		frameTexts = append(frameTexts, Text{
			Kind:     TextFrameKeyword,
			X:        float32(x + tabW/2 - 2),
			Y:        float32(y + tabH/2),
			Anchor:   AnchorMiddle,
			Baseline: BaselineMiddle,
			Size:     float32(cfg.FontSize - 2),
			Weight:   700,
			Text:     keyword,
		})

		if fm.title != "" {
			frameTexts = append(frameTexts, Text{
				Kind:     TextFrameTitle,
				X:        float32(x + tabW + 10),
				Y:        float32(y + tabH/2),
				Anchor:   AnchorStart,
				Baseline: BaselineMiddle,
				Size:     float32(cfg.FontSize - 2),
				Weight:   400,
				Text:     fm.title,
			})
		}

		var dividers []float32
		for i, divY := range fm.dividers {
			dividers = append(dividers, float32(divY))
			if i < len(fm.titles) && fm.titles[i] != "" {
				frameTexts = append(frameTexts, Text{
					Kind:     TextSectionTitle,
					X:        float32(x + 10),
					Y:        float32(divY + 12),
					Anchor:   AnchorStart,
					Baseline: BaselineMiddle,
					Size:     float32(cfg.FontSize - 2),
					Weight:   400,
					Text:     fm.titles[i],
				})
			}
		}

		out.Frames = append(out.Frames, Frame{
			Kind:     fm.kind,
			Rect:     Rect{X: float32(x), Y: float32(y), W: float32(w), H: float32(h)},
			Tab:      tab,
			Dividers: dividers,
			Texts:    frameTexts,
			Line:     fm.line,
		})
	}

	// 8. Finalize Rect Backgrounds
	for _, rm := range finishedRects {
		frameX := bounds.startx
		frameStopX := bounds.stopx
		if frameX == nil || frameStopX == nil {
			continue
		}
		x := *frameX + cfg.BoxMargin
		w := (*frameStopX - *frameX) - 2*cfg.BoxMargin
		out.Rects = append(out.Rects, Background{
			Rect: Rect{X: float32(x), Y: float32(rm.starty), W: float32(w), H: float32(rm.stopy - rm.starty)},
			Fill: rm.fill,
			Line: rm.line,
		})
	}

	// 9. Diagram Title
	if d.Title != "" {
		startX := 0.0
		stopX := 0.0
		if bounds.startx != nil {
			startX = *bounds.startx
		}
		if bounds.stopx != nil {
			stopX = *bounds.stopx
		}
		titleX := (stopX - startX) / 2
		out.Title = &Text{
			Kind:     TextTitle,
			X:        float32(titleX),
			Y:        -25,
			Anchor:   AnchorMiddle,
			Baseline: BaselineMiddle,
			Size:     float32(cfg.FontSize + 2),
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
	for _, it := range items {
		switch {
		case it.Message != nil:
			events = append(events, layoutEvent{
				kind: evMessage,
				msg:  it.Message,
				line: it.Line,
			})
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
					label:     b.Label,
					line:      b.Line,
					blockKind: b.Kind,
				})
				events = append(events, flattenItems(b.Items)...)
				for _, br := range b.Branches {
					events = append(events, layoutEvent{
						kind:      evBlockBranch,
						label:     br.Label,
						line:      br.Line,
						blockKind: b.Kind,
					})
					events = append(events, flattenItems(br.Items)...)
				}
				events = append(events, layoutEvent{
					kind:      evBlockEnd,
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
