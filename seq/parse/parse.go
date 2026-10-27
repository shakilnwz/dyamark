// Package parse reads Mermaid sequenceDiagram source into a Diagram.
//
// It started as a copy of github.com/julianshen/mmgo's sequence parser (see
// UPSTREAM and LICENSE.mmgo) and follows Mermaid 12's sequenceDiagram.jison
// and sequenceDb.ts for behaviour: statements end at a newline or `;`, `#`
// starts a comment, keywords are case-insensitive, and the same statements
// Mermaid rejects are errors here.
package parse

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Error is a parse error at a 1-based source line.
type Error struct {
	Line int
	Msg  string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

// Warning is something Mermaid ignores, reported so the UI can show it.
type Warning struct {
	Line int
	Msg  string
}

// Parse reads a sequence diagram. Line numbers in errors, warnings and the
// model count from the first line of src, frontmatter included.
func Parse(src string) (*Diagram, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")

	fm, body, err := splitFrontmatter(src)
	if err != nil {
		return nil, err
	}
	dirs, body := extractDirectives(body)
	cfg, warnings := mergeConfig(fm, dirs)

	p := &parser{
		diagram: &Diagram{
			Title:    fm.title,
			Config:   cfg,
			Warnings: warnings,
		},
		participantIx: make(map[string]int),
		active:        make(map[string]int),
		wrapDefault:   cfg.WrapByDefault(),
	}
	if err := p.parseBody(body); err != nil {
		return nil, err
	}
	return p.diagram, nil
}

// participantIx maps participant ID to its position in
// diagram.Participants so that explicit declarations can upgrade an
// entry that was implicitly registered by a prior message.
type parser struct {
	diagram       *Diagram
	participantIx map[string]int
	blockStack    []*blockFrame
	boxFrame      *boxFrameState
	wrapDefault   bool
	headerSeen    bool
	line          int

	// active counts activations per participant, to reject deactivating
	// one that isn't active, as Mermaid does.
	active map[string]int
	// pendingCreate / pendingDestroy name a participant whose `create` /
	// `destroy` still waits for its message.
	pendingCreate  string
	pendingDestroy string

	// non-nil while inside an accDescr {…} block.
	accDescrBlock *strings.Builder
}

type boxFrameState struct {
	index int
}

type blockFrame struct {
	block        *Block
	activeBranch *Block
}

func (p *parser) errorf(format string, args ...any) error {
	return &Error{Line: p.line, Msg: fmt.Sprintf(format, args...)}
}

func (p *parser) parseBody(body string) error {
	for i, raw := range strings.Split(body, "\n") {
		p.line = i + 1
		if err := p.parseSourceLine(raw); err != nil {
			return err
		}
	}
	if !p.headerSeen {
		return &Error{Msg: "missing sequenceDiagram header"}
	}
	if len(p.blockStack) > 0 {
		b := p.blockStack[len(p.blockStack)-1].block
		return &Error{Line: b.Line, Msg: fmt.Sprintf("unclosed %v block (missing 'end')", b.Kind)}
	}
	if p.boxFrame != nil {
		return &Error{Line: p.diagram.Boxes[p.boxFrame.index].Line, Msg: "unclosed box (missing 'end')"}
	}
	if p.accDescrBlock != nil {
		return &Error{Msg: "unclosed accDescr block (missing '}')"}
	}
	p.markDestroyed(p.diagram.Items)
	return nil
}

// parseSourceLine splits one source line into statements: Mermaid ends a
// statement at a newline or `;`, and `#` starts a comment.
func (p *parser) parseSourceLine(raw string) error {
	line := raw
	// Inside an accDescr {…} block every line is description text — must
	// short-circuit before any other dispatch, otherwise lines containing
	// "note", "end", etc. would be mis-parsed as diagram statements.
	if p.accDescrBlock != nil {
		text, rest, closed := strings.Cut(line, "}")
		if p.accDescrBlock.Len() > 0 {
			p.accDescrBlock.WriteByte('\n')
		}
		p.accDescrBlock.WriteString(strings.TrimSpace(text))
		if !closed {
			return nil
		}
		p.diagram.AccDescr = strings.TrimSpace(p.accDescrBlock.String())
		p.accDescrBlock = nil
		line = rest
	}
	if t := strings.TrimSpace(line); strings.HasPrefix(t, "%%") {
		return nil
	}
	line = encodeEntities(line)

	for {
		line = strings.TrimLeft(line, " \t")
		if line == "" || line[0] == '#' {
			return nil
		}
		// accTitle and accDescr take the rest of the line, `;` and `#` too.
		if p.headerSeen {
			if v, ok := cutKeywordColon(line, "accTitle"); ok {
				p.diagram.AccTitle = decodeEntities(strings.TrimSpace(v))
				return nil
			}
			if v, ok := cutKeywordColon(line, "accDescr"); ok {
				p.diagram.AccDescr = decodeEntities(strings.TrimSpace(v))
				return nil
			}
			if v, ok := cutKeyword(line, "accDescr"); ok {
				if r, ok := strings.CutPrefix(strings.TrimLeft(v, " \t"), "{"); ok {
					p.accDescrBlock = &strings.Builder{}
					return p.parseSourceLine(r)
				}
			}
		}
		end := strings.IndexAny(line, ";#")
		stmt, next := line, ""
		if end >= 0 {
			stmt, next = line[:end], line[end:]
			if next[0] == ';' {
				next = next[1:]
			}
		}
		if s := strings.TrimSpace(stmt); s != "" {
			if err := p.parseStatement(s); err != nil {
				return err
			}
		}
		line = next
	}
}

func (p *parser) parseStatement(line string) error {
	if !p.headerSeen {
		if _, ok := cutKeyword(line, "sequenceDiagram"); !ok {
			return p.errorf("expected 'sequenceDiagram' header, got %q", decodeEntities(line))
		}
		p.headerSeen = true
		return nil
	}
	if p.boxFrame != nil {
		return p.parseBoxStatement(line)
	}

	if rest, ok := cutKeyword(line, "participant"); ok {
		return p.parseParticipant(rest, ParticipantKindParticipant, false)
	}
	if rest, ok := cutKeyword(line, "actor"); ok {
		return p.parseParticipant(rest, ParticipantKindActor, false)
	}
	if rest, ok := cutKeyword(line, "create"); ok {
		return p.parseCreate(rest)
	}
	if rest, ok := cutKeyword(line, "destroy"); ok {
		return p.parseDestroy(rest)
	}
	if rest, ok := cutKeyword(line, "box"); ok {
		return p.openBox(rest)
	}
	if _, ok := cutKeyword(line, "end"); ok {
		return p.closeBlock()
	}
	if kind, label, ok := matchBlockOpen(line); ok {
		return p.openBlock(kind, label)
	}
	if parents, label, ok := matchBranchKeyword(line); ok {
		return p.addBranch(parents, label)
	}
	if rest, ok := cutKeyword(line, "note"); ok {
		return p.parseNote(rest)
	}
	if rest, ok := cutKeyword(line, "links"); ok {
		return p.parseLinks(rest)
	}
	if rest, ok := cutKeyword(line, "link"); ok {
		return p.parseLink(rest)
	}
	if rest, ok := cutKeyword(line, "properties"); ok {
		return p.parseProperties(rest)
	}
	if rest, ok := cutKeyword(line, "details"); ok {
		// Mermaid reads details from an element in the host page; there is
		// no page here.
		_, _, err := p.actorText(rest, "details")
		return err
	}
	if rest, ok := cutKeyword(line, "activate"); ok {
		return p.parseActivation(rest, true)
	}
	if rest, ok := cutKeyword(line, "deactivate"); ok {
		return p.parseActivation(rest, false)
	}
	if rest, ok := cutKeywordColon(line, "title"); ok {
		p.diagram.Title = displayText(strings.TrimSpace(rest))
		return nil
	}
	if rest, ok := cutKeyword(line, "title"); ok {
		p.diagram.Title = displayText(rest)
		return nil
	}
	if rest, ok := cutKeyword(line, "autonumber"); ok {
		return p.parseAutonumber(rest)
	}
	if m, hasText, ok := parseMessage(line, p.wrapDefault); ok {
		if !hasText {
			return p.errorf("message needs ':' after its target, as in 'A->>B: text' or 'A->>B:'")
		}
		return p.addMessage(m)
	}
	return p.errorf("unrecognized statement: %q", decodeEntities(line))
}

// cutKeyword returns the whitespace-trimmed remainder after kw when line
// starts with kw, in any case, followed by end-of-string or whitespace. The
// word-boundary check keeps an actor named `Endpoint` from matching `end`.
func cutKeyword(line, kw string) (string, bool) {
	if len(line) < len(kw) || !strings.EqualFold(line[:len(kw)], kw) {
		return "", false
	}
	if len(line) == len(kw) {
		return "", true
	}
	c := line[len(kw)]
	if c != ' ' && c != '\t' {
		return "", false
	}
	return strings.TrimSpace(line[len(kw)+1:]), true
}

// cutKeywordColon matches `kw:` with optional space before the colon, as
// in `title: X` and `accTitle : X`.
func cutKeywordColon(line, kw string) (string, bool) {
	if len(line) < len(kw) || !strings.EqualFold(line[:len(kw)], kw) {
		return "", false
	}
	rest := strings.TrimLeft(line[len(kw):], " \t")
	return strings.CutPrefix(rest, ":")
}

func (p *parser) parseBoxStatement(line string) error {
	if _, ok := cutKeyword(line, "end"); ok {
		p.boxFrame = nil
		return nil
	}
	if rest, ok := cutKeyword(line, "participant"); ok {
		return p.parseParticipant(rest, ParticipantKindParticipant, false)
	}
	if rest, ok := cutKeyword(line, "actor"); ok {
		return p.parseParticipant(rest, ParticipantKindActor, false)
	}
	if rest, ok := cutKeyword(line, "destroy"); ok {
		return p.parseDestroy(rest)
	}
	if _, ok := cutKeyword(line, "box"); ok {
		return p.errorf("nested boxes are not allowed")
	}
	return p.errorf("only participant and actor declarations can go inside a box, got %q", decodeEntities(line))
}

// participantDecl is the shape of a participant declaration:
// `ID`, `ID as Alias`, `ID@{ config }` or `ID@{ config } as Alias`.
var (
	participantWithConfig = regexp.MustCompile(`^([^<>:,;@\s]+)@\{([^}]*)\}(.*)$`)
	participantWithAlias  = regexp.MustCompile(`(?i)^([^<>:,;@\s]+)\s+as(?:\s+(.*))?$`)
	aliasSuffix           = regexp.MustCompile(`(?i)^\s+as(?:\s+(.*))?$`)
)

type participantDecl struct {
	id       string
	alias    string
	hasAlias bool
	wrap     bool
	kind     ParticipantKind
	hasKind  bool
}

func (p *parser) parseParticipantDecl(rest string) (participantDecl, error) {
	var d participantDecl
	// aliasSrc is an `as` alias, which wins over one in the config.
	var aliasSrc string
	external := false
	if m := participantWithConfig.FindStringSubmatch(rest); m != nil {
		d.id = m[1]
		if err := p.applyParticipantConfig(&d, m[2]); err != nil {
			return d, err
		}
		if after := m[3]; strings.TrimSpace(after) != "" {
			am := aliasSuffix.FindStringSubmatch(after)
			if am == nil {
				return d, p.errorf("unexpected %q after participant config", decodeEntities(strings.TrimSpace(after)))
			}
			aliasSrc, external = am[1], true
		}
	} else if strings.Contains(rest, "@{") {
		return d, p.errorf("participant config is missing its closing '}'")
	} else if m := participantWithAlias.FindStringSubmatch(rest); m != nil {
		d.id, aliasSrc, external = m[1], m[2], true
	} else {
		d.id = strings.TrimSpace(rest)
		if strings.ContainsAny(d.id, "<>:,@") {
			return d, p.errorf("invalid participant name %q", decodeEntities(d.id))
		}
	}
	if d.id == "" {
		return d, p.errorf("participant declaration missing ID")
	}
	d.id = decodeEntities(d.id)
	switch {
	case external:
		d.alias, d.wrap = parseText(aliasSrc, p.wrapDefault)
		d.hasAlias = true
	case !d.hasAlias:
		d.wrap = p.wrapDefault
	}
	return d, nil
}

// applyParticipantConfig reads `@{ "type": "database", "alias": "DB" }`.
// Mermaid loads it as YAML with the JSON schema, so both quoted and bare
// keys work.
func (p *parser) applyParticipantConfig(d *participantDecl, src string) error {
	var meta map[string]any
	if err := yaml.Unmarshal([]byte("{"+src+"}"), &meta); err != nil {
		return p.errorf("participant config: %v", err)
	}
	if t, ok := meta["type"].(string); ok {
		if k, ok := participantKindByName(t); ok {
			d.kind, d.hasKind = k, true
		} else {
			p.warnf("unknown participant type %q, drawn as a participant", t)
			d.kind, d.hasKind = ParticipantKindParticipant, true
		}
	}
	if a, ok := meta["alias"].(string); ok {
		d.alias, d.wrap = parseText(a, p.wrapDefault)
		d.hasAlias = true
	}
	return nil
}

func (p *parser) warnf(format string, args ...any) {
	p.diagram.Warnings = append(p.diagram.Warnings, Warning{Line: p.line, Msg: fmt.Sprintf(format, args...)})
}

func (p *parser) parseParticipant(rest string, kind ParticipantKind, create bool) error {
	if rest == "" {
		return p.errorf("participant declaration missing ID")
	}
	d, err := p.parseParticipantDecl(rest)
	if err != nil {
		return err
	}
	if !d.hasKind {
		d.kind = kind
	}
	if create {
		if _, exists := p.participantIx[d.id]; exists {
			return p.errorf("participant %q already exists; Mermaid doesn't allow re-creating one, even after destroy. Use 'as' aliases instead", d.id)
		}
	}
	return p.declareParticipant(d)
}

// declareParticipant follows Mermaid's addActor: a repeated declaration
// with no alias or config changes nothing, while one with either replaces
// the label and kind. Box membership is kept from the first box.
func (p *parser) declareParticipant(d participantDecl) error {
	idx, exists := p.participantIx[d.id]
	if !exists {
		p.registerParticipant(d.id)
		idx = p.participantIx[d.id]
	}
	pt := &p.diagram.Participants[idx]

	if p.boxFrame != nil {
		switch pt.BoxIndex {
		case -1:
			pt.BoxIndex = p.boxFrame.index
			box := &p.diagram.Boxes[p.boxFrame.index]
			box.Members = append(box.Members, d.id)
		case p.boxFrame.index:
		default:
			return p.errorf("participant %q is already in box %q; a participant can only be in one box",
				d.id, p.diagram.Boxes[pt.BoxIndex].Label)
		}
	}

	if exists && !d.hasAlias && !d.hasKind {
		return nil
	}
	pt.Kind = d.kind
	pt.Alias = d.alias
	pt.Wrap = d.wrap
	return nil
}

func (p *parser) registerParticipant(id string) {
	p.participantIx[id] = len(p.diagram.Participants)
	p.diagram.Participants = append(p.diagram.Participants, Participant{
		ID: id, Kind: ParticipantKindParticipant, BoxIndex: -1,
		Wrap: p.wrapDefault, Line: p.line,
	})
}

func (p *parser) ensureParticipant(id string) {
	if _, ok := p.participantIx[id]; ok {
		return
	}
	p.registerParticipant(id)
}

func (p *parser) parseCreate(rest string) error {
	var kind ParticipantKind
	var idRest string
	if r, ok := cutKeyword(rest, "participant"); ok {
		kind, idRest = ParticipantKindParticipant, r
	} else if r, ok := cutKeyword(rest, "actor"); ok {
		kind, idRest = ParticipantKindActor, r
	} else {
		return p.errorf("create expects 'participant' or 'actor', got %q", decodeEntities(rest))
	}
	if err := p.parseParticipant(idRest, kind, true); err != nil {
		return err
	}
	id := p.diagram.Participants[len(p.diagram.Participants)-1].ID
	p.diagram.Participants[p.participantIx[id]].Created = true
	p.pendingCreate = id
	p.appendItem(Item{Create: &id})
	return nil
}

func (p *parser) parseDestroy(rest string) error {
	id := decodeEntities(strings.TrimSpace(rest))
	if id == "" {
		return p.errorf("destroy missing participant ID")
	}
	p.pendingDestroy = id
	p.appendItem(Item{Destroy: &id})
	return nil
}

// markDestroyed flags destroyed participants once all are known; a
// `destroy` may name one that its following message introduces.
func (p *parser) markDestroyed(items []Item) {
	for _, it := range items {
		switch {
		case it.Destroy != nil:
			if idx, ok := p.participantIx[*it.Destroy]; ok {
				p.diagram.Participants[idx].Destroyed = true
			}
		case it.Block != nil:
			p.markDestroyed(it.Block.Items)
			for _, br := range it.Block.Branches {
				p.markDestroyed(br.Items)
			}
		}
	}
}

func (p *parser) parseActivation(rest string, activate bool) error {
	id := decodeEntities(strings.TrimSpace(rest))
	if id == "" {
		kw := "activate"
		if !activate {
			kw = "deactivate"
		}
		return p.errorf("%s requires a participant ID", kw)
	}
	p.ensureParticipant(id)
	if activate {
		p.active[id]++
	} else if err := p.deactivate(id); err != nil {
		return err
	}
	p.appendItem(Item{Activation: &Activation{Participant: id, Activate: activate}})
	return nil
}

func (p *parser) deactivate(id string) error {
	if p.active[id] < 1 {
		return p.errorf("trying to deactivate %q, which isn't active", id)
	}
	p.active[id]--
	return nil
}

// autonumberArg is Mermaid's NUM token: up to two decimal places.
var autonumberArg = regexp.MustCompile(`^(?:[0-9]+(?:\.[0-9]{1,2})?|\.[0-9]{1,2})$`)

func (p *parser) parseAutonumber(rest string) error {
	an := AutoNumber{Visible: true}
	fields := strings.Fields(rest)
	switch {
	case len(fields) == 1 && strings.EqualFold(fields[0], "off"):
		an.Visible = false
	case len(fields) > 2:
		return p.errorf("autonumber accepts at most 2 arguments (start step), got %d", len(fields))
	case len(fields) > 0:
		for _, f := range fields {
			if !autonumberArg.MatchString(f) {
				return p.errorf("autonumber: %q isn't a number with at most two decimals", f)
			}
		}
		an.Start, _ = strconv.ParseFloat(fields[0], 64)
		// `autonumber 10` resets the step to 1, as Mermaid's grammar does.
		an.Step = 1
		if len(fields) == 2 {
			an.Step, _ = strconv.ParseFloat(fields[1], 64)
		}
	}
	p.appendItem(Item{AutoNumber: &an})
	return nil
}

var notePlacement = regexp.MustCompile(`(?i)^(left\s+of|right\s+of|over)\s+(.*)$`)

func (p *parser) parseNote(rest string) error {
	m := notePlacement.FindStringSubmatch(rest)
	if m == nil {
		return p.errorf("note position must be 'left of', 'right of', or 'over': %q", decodeEntities(rest))
	}
	var pos NotePosition
	switch strings.ToLower(strings.Fields(m[1])[0]) {
	case "left":
		pos = NotePositionLeft
	case "right":
		pos = NotePositionRight
	default:
		pos = NotePositionOver
	}
	who, textSrc, ok := strings.Cut(m[2], ":")
	if !ok {
		return p.errorf("note missing ':' before text: %q", decodeEntities(rest))
	}
	parts := splitParticipantList(who)
	if pos != NotePositionOver && len(parts) != 1 {
		return p.errorf("'left of'/'right of' note takes exactly one participant, got %d", len(parts))
	}
	if len(parts) == 0 || len(parts) > 2 {
		return p.errorf("note expects 1 or 2 participants, got %d", len(parts))
	}
	for _, id := range parts {
		p.ensureParticipant(id)
	}
	text, wrap := parseText(textSrc, p.wrapDefault)
	p.appendItem(Item{Note: &Note{
		Participants: parts,
		Text:         text,
		Wrap:         wrap,
		Position:     pos,
	}})
	return nil
}

// actorText reads `<actor>: <text>`, the shape of links, link, properties
// and details statements, registering the actor.
func (p *parser) actorText(rest, kw string) (id, text string, err error) {
	who, text, ok := strings.Cut(rest, ":")
	id = decodeEntities(strings.TrimSpace(who))
	if !ok || id == "" {
		return "", "", p.errorf("%s expects '<participant>: <text>'", kw)
	}
	p.ensureParticipant(id)
	return id, decodeEntities(strings.TrimSpace(text)), nil
}

func (p *parser) parseLinks(rest string) error {
	id, text, err := p.actorText(rest, "links")
	if err != nil {
		return err
	}
	links, err := orderedStringMap(text)
	if err != nil {
		p.warnf("ignoring links for %q: %v", id, err)
		return nil
	}
	p.addLinks(id, links)
	return nil
}

func (p *parser) parseLink(rest string) error {
	id, text, err := p.actorText(rest, "link")
	if err != nil {
		return err
	}
	label, url, ok := strings.Cut(text, "@")
	if !ok {
		p.warnf("ignoring link for %q: expected 'label @ url'", id)
		return nil
	}
	p.addLinks(id, []Link{{Label: strings.TrimSpace(label), URL: strings.TrimSpace(url)}})
	return nil
}

// addLinks adds links in order; a label given again replaces its URL in
// place, like assigning to a JavaScript object.
func (p *parser) addLinks(id string, links []Link) {
	pt := &p.diagram.Participants[p.participantIx[id]]
outer:
	for _, l := range links {
		for i := range pt.Links {
			if pt.Links[i].Label == l.Label {
				pt.Links[i].URL = l.URL
				continue outer
			}
		}
		pt.Links = append(pt.Links, l)
	}
}

func (p *parser) parseProperties(rest string) error {
	id, text, err := p.actorText(rest, "properties")
	if err != nil {
		return err
	}
	var props map[string]any
	if err := json.Unmarshal([]byte(text), &props); err != nil {
		p.warnf("ignoring properties for %q: %v", id, err)
		return nil
	}
	pt := &p.diagram.Participants[p.participantIx[id]]
	if pt.Properties == nil {
		pt.Properties = map[string]any{}
	}
	for k, v := range props {
		pt.Properties[k] = v
	}
	return nil
}

// orderedStringMap decodes a JSON object of strings, keeping key order.
func orderedStringMap(text string) ([]Link, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, fmt.Errorf("expected a JSON object")
	}
	var out []Link
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var v string
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		out = append(out, Link{Label: k.(string), URL: v})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return out, nil
}

// appendItem adds item to the innermost open block, stamping the current
// line unless the item has one.
func (p *parser) appendItem(item Item) {
	if item.Line == 0 {
		item.Line = p.line
	}
	if len(p.blockStack) > 0 {
		top := p.blockStack[len(p.blockStack)-1]
		top.activeBranch.Items = append(top.activeBranch.Items, item)
	} else {
		p.diagram.Items = append(p.diagram.Items, item)
	}
}

// addMessage checks the message against a pending create or destroy, as
// Mermaid's addMessage does, then records it and its activation.
func (p *parser) addMessage(m Message) error {
	if p.pendingCreate != "" {
		if m.To != p.pendingCreate {
			return p.errorf("the created participant %q has no creating message: the message after 'create' must be sent to it", p.pendingCreate)
		}
		p.pendingCreate = ""
	} else if p.pendingDestroy != "" {
		if m.From != p.pendingDestroy && m.To != p.pendingDestroy {
			return p.errorf("the destroyed participant %q has no destroying message: the message after 'destroy' must involve it", p.pendingDestroy)
		}
		p.pendingDestroy = ""
	}
	p.ensureParticipant(m.From)
	p.ensureParticipant(m.To)
	switch m.Lifeline {
	case LifelineEffectActivate:
		p.active[m.To]++
	case LifelineEffectDeactivate:
		if err := p.deactivate(m.From); err != nil {
			return err
		}
	}
	p.appendItem(Item{Message: &m})
	return nil
}

var blockOpeners = []struct {
	kw   string
	kind BlockKind
}{
	{"alt", BlockKindAlt},
	{"opt", BlockKindOpt},
	{"loop", BlockKindLoop},
	{"par_over", BlockKindParOver},
	{"par", BlockKindPar},
	{"critical", BlockKindCritical},
	{"break", BlockKindBreak},
	{"rect", BlockKindRect},
}

func matchBlockOpen(line string) (BlockKind, string, bool) {
	for _, bo := range blockOpeners {
		if rest, ok := cutKeyword(line, bo.kw); ok {
			return bo.kind, rest, true
		}
	}
	return 0, "", false
}

func (p *parser) openBlock(kind BlockKind, label string) error {
	b := &Block{Kind: kind, Line: p.line}
	if kind == BlockKindRect {
		// Mermaid gives rect no label: the whole line is its colour.
		b.Fill, _ = parseText(label, false)
	} else {
		b.Label, b.Wrap = parseText(label, p.wrapDefault)
	}
	frame := &blockFrame{block: b, activeBranch: b}
	p.blockStack = append(p.blockStack, frame)
	return nil
}

func (p *parser) closeBlock() error {
	if len(p.blockStack) == 0 {
		return p.errorf("'end' without a matching block opener")
	}
	top := p.blockStack[len(p.blockStack)-1]
	p.blockStack = p.blockStack[:len(p.blockStack)-1]
	top.block.EndLine = p.line
	p.appendItem(Item{Block: top.block, Line: top.block.Line})
	return nil
}

func (p *parser) openBox(rest string) error {
	fill, label, wrap := parseBoxData(rest, p.wrapDefault)
	p.diagram.Boxes = append(p.diagram.Boxes, Box{
		Label: label,
		Fill:  fill,
		Wrap:  wrap,
		Line:  p.line,
	})
	p.boxFrame = &boxFrameState{index: len(p.diagram.Boxes) - 1}
	return nil
}

// Branch keywords: else → alt, and → par / par_over, option → critical.
var branchKeywords = []struct {
	kw      string
	parents []BlockKind
}{
	{"else", []BlockKind{BlockKindAlt}},
	{"and", []BlockKind{BlockKindPar, BlockKindParOver}},
	{"option", []BlockKind{BlockKindCritical}},
}

func matchBranchKeyword(line string) ([]BlockKind, string, bool) {
	for _, bk := range branchKeywords {
		if rest, ok := cutKeyword(line, bk.kw); ok {
			return bk.parents, rest, true
		}
	}
	return nil, "", false
}

func (p *parser) addBranch(parents []BlockKind, label string) error {
	if len(p.blockStack) == 0 {
		return p.errorf("branch keyword outside of any block")
	}
	top := p.blockStack[len(p.blockStack)-1]
	valid := false
	for _, k := range parents {
		valid = valid || top.block.Kind == k
	}
	if !valid {
		return p.errorf("branch keyword not valid inside %v block", top.block.Kind)
	}
	branch := Block{Kind: top.block.Kind, Line: p.line}
	branch.Label, branch.Wrap = parseText(label, p.wrapDefault)
	top.block.Branches = append(top.block.Branches, branch)
	top.activeBranch = &top.block.Branches[len(top.block.Branches)-1]
	return nil
}

func splitParticipantList(s string) []string {
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if t := strings.TrimSpace(r); t != "" {
			out = append(out, decodeEntities(t))
		}
	}
	return out
}

// arrowTokens lists the message arrow literals with their ArrowType.
// When two start at the same position the longer one wins, so `-->>`
// beats `-->`, `--x` beats `-x`, etc.
var arrowTokens = []struct {
	lit string
	typ ArrowType
}{
	{"<<-->>", ArrowTypeDashedBi},
	{"<<->>", ArrowTypeSolidBi},
	{"-->>", ArrowTypeDashed},
	{"-->", ArrowTypeDashedNoHead},
	{"--x", ArrowTypeDashedCross},
	{"--)", ArrowTypeDashedOpen},
	{"->>", ArrowTypeSolid},
	{"->", ArrowTypeSolidNoHead},
	{"-x", ArrowTypeSolidCross},
	{"-)", ArrowTypeSolidOpen},
	{`--|\`, ArrowTypeSolidTopDashed},
	{`--|/`, ArrowTypeSolidBottomDashed},
	{`--\\`, ArrowTypeStickTopDashed},
	{`--//`, ArrowTypeStickBottomDashed},
	{`-|\`, ArrowTypeSolidTop},
	{`-|/`, ArrowTypeSolidBottom},
	{`-\\`, ArrowTypeStickTop},
	{`-//`, ArrowTypeStickBottom},
	{`/|--`, ArrowTypeSolidTopReverseDashed},
	{`\|--`, ArrowTypeSolidBottomReverseDashed},
	{`//--`, ArrowTypeStickTopReverseDashed},
	{`\\--`, ArrowTypeStickBottomReverseDashed},
	{`/|-`, ArrowTypeSolidTopReverse},
	{`\|-`, ArrowTypeSolidBottomReverse},
	{`//-`, ArrowTypeStickTopReverse},
	{`\\-`, ArrowTypeStickBottomReverse},
}

// parseMessage finds the LEFTMOST arrow token in line and splits
// From/To around it. Leftmost-wins is correctness-critical when a
// message label contains an arrow-like substring — e.g.,
// `A->>B: send --> response` must split on the leading `->>`, not the
// `-->` inside the label. hasText reports the `:` Mermaid requires before
// the (possibly empty) text.
func parseMessage(line string, wrapDefault bool) (m Message, hasText, ok bool) {
	head, textSrc, hasText := strings.Cut(line, ":")
	bestIdx := -1
	var best struct {
		lit string
		typ ArrowType
	}
	for _, tok := range arrowTokens {
		idx := strings.Index(head, tok.lit)
		if idx <= 0 {
			continue
		}
		if bestIdx == -1 || idx < bestIdx || (idx == bestIdx && len(tok.lit) > len(best.lit)) {
			bestIdx, best = idx, tok
		}
	}
	if bestIdx == -1 {
		return Message{}, false, false
	}
	from := strings.TrimSpace(head[:bestIdx])
	to := strings.TrimSpace(head[bestIdx+len(best.lit):])

	m.ArrowType = best.typ
	sourceCentral := false
	if r, ok := strings.CutSuffix(from, "()"); ok {
		from, sourceCentral = strings.TrimSpace(r), true
	}
	switch {
	case strings.HasPrefix(to, "()"):
		to = strings.TrimSpace(to[2:])
		m.Central = CentralTarget
		if sourceCentral {
			m.Central = CentralBoth
		}
	case sourceCentral:
		m.Central = CentralSource
	case strings.HasPrefix(to, "+"):
		m.Lifeline, to = LifelineEffectActivate, strings.TrimSpace(to[1:])
	case strings.HasPrefix(to, "-"):
		m.Lifeline, to = LifelineEffectDeactivate, strings.TrimSpace(to[1:])
	}
	if from == "" || to == "" || hasArrow(to) {
		return Message{}, false, false
	}
	m.From, m.To = decodeEntities(from), decodeEntities(to)
	m.Label, m.Wrap = parseText(textSrc, wrapDefault)
	return m, hasText, true
}

// hasArrow reports whether s contains an arrow token. A target that does
// means two arrows in one statement, as in `A-xavier->>B`, which Mermaid
// rejects.
func hasArrow(s string) bool {
	for _, tok := range arrowTokens {
		if strings.Contains(s, tok.lit) {
			return true
		}
	}
	return false
}
