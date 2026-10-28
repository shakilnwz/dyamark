package layout

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"dyamark/seq/parse"
)

// The parity test lays out every seq/testdata/*.mmd and compares the
// geometry, element by element and in both directions, with the SVG that
// mmdc drew for it (*.ref.svg, made by bin/golden).
//
// Each kind of element (top actors, messages, frames, …) is collected from
// both sides into the same shape: a list of items with a label and a few
// numbers. A kind matches when both lists have the same length and every
// number is within parityTol.

const parityTol = 2.0

// knownDiffs lists, per diagram, the kinds that don't match Mermaid yet,
// each a known layout gap. They're logged, not failed. A listed kind that
// starts matching fails the test, so the list only ever shrinks.
//
// The gaps, as of 2026-10-28:
//   - message-text: every message label is 5px lower than Mermaid's, and a
//     self message's label is at startx+25 instead of startx.
//   - message: self-message control points are at startx+50, not +60.
//   - note-text: note lines are 2.5px higher than Mermaid's.
//   - frame, frame-tab, frame-text, divider, rect: frames and rect
//     backgrounds span the whole diagram instead of the participants
//     inside them, the frame title is left-aligned instead of centred,
//     and dividers sit 15px high.
//   - seqnum: the autonumber badge is at min(startx, stopx)+10, lineY-10
//     instead of startx, lineY.
//   - title: centred on the diagram instead of Mermaid's start-anchored
//     x = width/2 - 2*diagramMarginX.
//   - box-label: 3.5px higher than Mermaid's.
//   - actor-label: lines of a multi-line name are 34px apart, not 16px.
//   - frame-text: section labels aren't put in [brackets], else / and /
//     option labels aren't wrapped to the frame's width.
//   - seqnum: the counter doesn't advance while autonumber is off.
//   - 33: a deactivate pushes everything after it 12px down.
//   - 35: create and destroy aren't laid out.
//   - 38: a word wider than the wrap width isn't broken.
//   - 39: a wrapped note is as wide as the actors, not max(width, text).
//
// The kinds after a gap in the same diagram (lifeline, actor-bottom,
// viewbox) move with it, so they're listed too.
var knownDiffs = map[string][]string{
	"01-basic":                   {"message-text"},
	"02-aliases":                 {"message-text"},
	"03-arrows-solid-open":       {"message-text"},
	"04-arrows-cross-point":      {"message-text"},
	"05-arrows-bidirectional":    {"message-text"},
	"06-arrow-half-solid":        {"message-text"},
	"07-arrow-half-stick":        {"message-text"},
	"08-activations-inline":      {"message-text"},
	"09-activations-explicit":    {"message", "message-text"},
	"10-activations-stacked":     {"message", "message-text"},
	"11-self-message":            {"message", "message-text"},
	"12-self-message-dotted":     {"message", "message-text"},
	"13-note-right":              {"message-text", "note-text"},
	"14-note-left":               {"message-text", "note-text"},
	"15-note-over-single":        {"message-text", "note-text"},
	"16-note-over-two":           {"message-text", "note-text"},
	"17-loop":                    {"frame", "frame-tab", "frame-text", "message-text"},
	"18-alt-else":                {"divider", "frame", "frame-tab", "frame-text", "message-text"},
	"19-opt":                     {"frame", "frame-tab", "frame-text", "message-text"},
	"20-par-and":                 {"divider", "frame", "frame-tab", "frame-text", "message-text"},
	"21-critical-option":         {"divider", "frame", "frame-tab", "frame-text", "message-text"},
	"22-break":                   {"frame", "frame-tab", "frame-text", "message-text"},
	"23-rect":                    {"message-text", "rect"},
	"24-boxes":                   {"box-label", "message-text"},
	"25-autonumber":              {"message-text", "seqnum"},
	"26-autonumber-custom":       {"message-text", "seqnum"},
	"27-title":                   {"message-text", "title"},
	"28-multiline-message":       {"message-text"},
	"29-multiline-actor":         {"actor-label", "message-text"},
	"30-multiline-note":          {"message-text", "note-text"},
	"31-nested-blocks":           {"divider", "frame", "frame-tab", "frame-text", "message-text"},
	"32-complex-auth":            {"divider", "frame", "frame-tab", "frame-text", "message-text", "note-text", "seqnum"},
	"33-deactivate-then-message": {"actor-bottom", "actor-label", "lifeline", "message", "message-text", "viewbox"},
	"34-autonumber-off":          {"message-text", "seqnum"},
	"35-create-destroy":          {"actor-bottom", "actor-label", "actor-top", "lifeline", "message", "message-text", "viewbox"},
	"36-frames-partial-width":    {"frame", "frame-tab", "frame-text", "message-text"},
	"37-else-long-label":         {"actor-bottom", "actor-label", "divider", "frame", "frame-tab", "frame-text", "lifeline", "message", "message-text", "viewbox"},
	"38-wrap-long-word":          {"actor-bottom", "actor-label", "actor-top", "lifeline", "message", "message-text", "viewbox"},
	"39-wrap-note":               {"message-text", "note", "note-text"},
}

// item is one element: a label for the report and the numbers compared.
type item struct {
	label string
	v     []float32
}

// geometry holds a diagram's items by kind.
type geometry map[string][]item

func (g geometry) add(kind, label string, v ...float32) {
	g[kind] = append(g[kind], item{label: label, v: v})
}

// Kinds whose order differs between Mermaid's SVG and our layout are sorted
// by position on both sides before comparing.
var sortedKinds = map[string]bool{
	"activation": true,
	"frame":      true,
	"frame-tab":  true,
	"divider":    true,
	"frame-text": true,
	"rect":       true,
}

// Kinds keyed by participant are sorted by label.
var labelledKinds = map[string]bool{
	"actor-top":    true,
	"actor-bottom": true,
	"actor-label":  true,
	"lifeline":     true,
}

func (g geometry) normalise() {
	for kind, items := range g {
		switch {
		case labelledKinds[kind]:
			sort.SliceStable(items, func(i, j int) bool { return items[i].label < items[j].label })
		case sortedKinds[kind]:
			sort.SliceStable(items, func(i, j int) bool {
				a, b := items[i].v, items[j].v
				for k := 0; k < len(a) && k < len(b); k++ {
					if a[k] != b[k] {
						return a[k] < b[k]
					}
				}
				return len(a) < len(b)
			})
		}
	}
}

// layoutGeometry collects our layout's items.
func layoutGeometry(d *Diagram) geometry {
	g := geometry{}
	vb := d.ViewBox
	g.add("viewbox", "", vb.X, vb.Y, vb.W, vb.H)

	if d.Title != nil {
		g.add("title", d.Title.Text, textPos(*d.Title)...)
	}
	for _, a := range d.Actors {
		kind := "actor-top"
		if a.Footer {
			kind = "actor-bottom"
		}
		g.add(kind, a.ID, rectV(a.Rect)...)
		for i, t := range a.Label {
			g.add("actor-label", fmt.Sprintf("%s/%s/%d", kind, a.ID, i), textPos(t)...)
		}
	}
	for _, l := range d.Lifelines {
		g.add("lifeline", l.ID, l.X, l.Y1, l.Y2)
	}
	for _, m := range d.Messages {
		label := m.From + "->" + m.To
		var v []float32
		for _, p := range m.Path {
			v = append(v, p.X, p.Y)
		}
		g.add("message", label, v...)
		for _, t := range m.Text {
			g.add("message-text", t.Text, textPos(t)...)
		}
		if m.Number != nil {
			n := m.Number
			g.add("seqnum", n.Text.Text, append([]float32{n.Center.X, n.Center.Y}, textPos(n.Text)...)...)
		}
	}
	for _, n := range d.Notes {
		g.add("note", "", rectV(n.Rect)...)
		for _, t := range n.Text {
			g.add("note-text", t.Text, textPos(t)...)
		}
	}
	for _, a := range d.Activations {
		g.add("activation", a.ID, rectV(a.Rect)...)
	}
	for _, f := range d.Frames {
		g.add("frame", f.Kind.String(), rectV(f.Rect)...)
		var tab []float32
		for _, p := range f.Tab {
			tab = append(tab, p.X, p.Y)
		}
		g.add("frame-tab", f.Kind.String(), tab...)
		for _, y := range f.Dividers {
			g.add("divider", "", f.Rect.X, y, f.Rect.X+f.Rect.W, y)
		}
		for _, t := range f.Texts {
			g.add("frame-text", t.Text, textPos(t)...)
		}
	}
	for _, r := range d.Rects {
		g.add("rect", "rect", rectV(r.Rect)...)
	}
	for _, b := range d.Boxes {
		g.add("rect", "box", rectV(b.Rect)...)
		for _, t := range b.Label {
			g.add("box-label", t.Text, textPos(t)...)
		}
	}
	g.normalise()
	return g
}

func rectV(r Rect) []float32 { return []float32{r.X, r.Y, r.W, r.H} }

// textPos is where a text line ends up: its position plus its dy.
func textPos(t Text) []float32 { return []float32{t.X, t.Y + t.DY} }

var numberRE = regexp.MustCompile(`-?[0-9]*\.?[0-9]+(?:e-?[0-9]+)?`)

// refGeometry collects the items from an mmdc SVG.
func refGeometry(data []byte) (geometry, error) {
	g := geometry{}
	d := xml.NewDecoder(strings.NewReader(string(data)))

	// The text being read, if any.
	var (
		inText   bool
		textKind string
		textX    float32
		textY    float32
		textDY   float32
		tspanDY  float32
		textBuf  strings.Builder
	)
	// The participant the next actor label belongs to.
	lastActor := ""

	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			attrs := make(map[string]string)
			for _, a := range elem.Attr {
				attrs[a.Name.Local] = a.Value
			}
			cls := attrs["class"]
			name := attrs["name"]
			if name == "" {
				name = attrs["data-id"]
			}
			num := func(k string) float32 { return parseFloat(attrs[k]) }

			switch elem.Name.Local {
			case "svg":
				if parts := strings.Fields(attrs["viewBox"]); len(parts) == 4 {
					g.add("viewbox", "", parseFloat(parts[0]), parseFloat(parts[1]), parseFloat(parts[2]), parseFloat(parts[3]))
				}

			case "rect":
				v := []float32{num("x"), num("y"), num("width"), num("height")}
				switch {
				case strings.Contains(cls, "actor-top"):
					g.add("actor-top", name, v...)
					lastActor = "actor-top/" + name
				case strings.Contains(cls, "actor-bottom"):
					g.add("actor-bottom", name, v...)
					lastActor = "actor-bottom/" + name
				case strings.HasPrefix(cls, "activation"):
					g.add("activation", "", v...)
				case cls == "note":
					g.add("note", "", v...)
				case cls == "rect":
					g.add("rect", "", v...)
				}

			case "line":
				v := []float32{num("x1"), num("y1"), num("x2"), num("y2")}
				switch {
				case strings.Contains(cls, "actor-line"):
					g.add("lifeline", name, v[0], v[1], v[3])
				case strings.HasPrefix(cls, "messageLine"):
					g.add("message", attrs["data-from"]+"->"+attrs["data-to"], v...)
				case cls == "loopLine":
					if strings.Contains(attrs["style"], "dasharray") {
						g.add("divider", "", v...)
					} else {
						g.add("loopline", "", v...)
					}
				case strings.Contains(attrs["marker-start"], "sequencenumber"):
					g.add("seqnum-anchor", "", v[0], v[1])
				}

			case "path":
				if strings.HasPrefix(cls, "messageLine") {
					var v []float32
					for _, n := range numberRE.FindAllString(attrs["d"], -1) {
						v = append(v, parseFloat(n))
					}
					g.add("message", attrs["data-from"]+"->"+attrs["data-to"], v...)
				}

			case "polygon":
				if cls == "labelBox" {
					var v []float32
					for _, n := range numberRE.FindAllString(attrs["points"], -1) {
						v = append(v, parseFloat(n))
					}
					g.add("frame-tab", "", v...)
				}

			case "text":
				inText = true
				textBuf.Reset()
				textX, textY = num("x"), num("y")
				textDY = parseLength(attrs["dy"])
				tspanDY = 0
				switch {
				case strings.Contains(cls, "actor-box"):
					textKind = "actor-label"
				case cls == "messageText":
					textKind = "message-text"
				case cls == "noteText":
					textKind = "note-text"
				case cls == "labelText", cls == "loopText", cls == "sectionTitle":
					textKind = "frame-text"
				case cls == "sequenceNumber":
					textKind = "seqnum-text"
				case cls == "text":
					textKind = "box-label"
				case cls == "":
					textKind = "title"
				default:
					textKind = ""
				}

			case "tspan":
				if inText && textBuf.Len() == 0 {
					tspanDY = parseLength(attrs["dy"])
				}
			}

		case xml.CharData:
			if inText {
				textBuf.Write(elem)
			}

		case xml.EndElement:
			if elem.Name.Local == "text" && inText {
				inText = false
				label := strings.TrimSpace(textBuf.String())
				if textKind == "actor-label" {
					label = lastActor
				}
				if textKind != "" {
					g.add(textKind, label, textX, textY+textDY+tspanDY)
				}
			}
		}
	}

	// Frames are drawn as four solid loopLines: top, right, bottom, left.
	lines := g["loopline"]
	delete(g, "loopline")
	for i := 0; i+3 < len(lines); i += 4 {
		top, bottom := lines[i].v, lines[i+2].v
		g.add("frame", "", top[0], top[1], top[2]-top[0], bottom[1]-top[1])
	}
	if len(lines)%4 != 0 {
		return nil, fmt.Errorf("%d loopLines don't make whole frames", len(lines))
	}

	// Sequence numbers are an anchor line and a text, in message order.
	anchors, texts := g["seqnum-anchor"], g["seqnum-text"]
	delete(g, "seqnum-anchor")
	delete(g, "seqnum-text")
	if len(anchors) != len(texts) {
		return nil, fmt.Errorf("%d sequence number anchors but %d texts", len(anchors), len(texts))
	}
	for i := range anchors {
		g.add("seqnum", texts[i].label, append(anchors[i].v, texts[i].v...)...)
	}

	// Actor labels are matched to the participant they follow.
	for i := range g["actor-label"] {
		g["actor-label"][i].label += "/" + strconv.Itoa(countBefore(g["actor-label"], i))
	}

	g.normalise()
	return g, nil
}

// countBefore counts the items before i with the same label, which makes
// the labels of a multi-line actor name distinct.
func countBefore(items []item, i int) int {
	n := 0
	for j := 0; j < i; j++ {
		if items[j].label == items[i].label {
			n++
		}
	}
	return n
}

func parseFloat(s string) float32 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "px")
	f, _ := strconv.ParseFloat(s, 32)
	return float32(f)
}

// parseLength reads a dy attribute, where 1em is the 16px font size.
func parseLength(s string) float32 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "em") {
		return parseFloat(strings.TrimSuffix(s, "em")) * 16
	}
	return parseFloat(s)
}

// compareKind returns what differs between got and ref for one kind, or
// nothing when they match.
func compareKind(got, ref []item) []string {
	var diffs []string
	if len(got) != len(ref) {
		diffs = append(diffs, fmt.Sprintf("count: got %d, ref %d", len(got), len(ref)))
	}
	for i := 0; i < len(got) && i < len(ref); i++ {
		g, r := got[i], ref[i]
		// Labels are compared where both sides have one: message ends,
		// participant names, and the words of every text.
		if g.label != "" && r.label != "" && g.label != r.label {
			diffs = append(diffs, fmt.Sprintf("[%d] label: got %q, ref %q", i, g.label, r.label))
		} else if !closeEnough(g.v, r.v) {
			label := g.label
			if label == "" {
				label = r.label
			}
			diffs = append(diffs, fmt.Sprintf("[%d] %s: got %v, ref %v", i, label, round(g.v), round(r.v)))
		}
	}
	return diffs
}

func closeEnough(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(float64(a[i]-b[i])) > parityTol {
			return false
		}
	}
	return true
}

func round(v []float32) []float32 {
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(math.Round(float64(x)*10) / 10)
	}
	return out
}

func TestParityAgainstMermaid(t *testing.T) {
	matches, err := filepath.Glob("../testdata/*.mmd")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) < 30 {
		t.Fatalf("expected at least 30 diagrams in seq/testdata, got %d", len(matches))
	}

	seen := map[string]bool{}
	for _, mmdPath := range matches {
		name := strings.TrimSuffix(filepath.Base(mmdPath), ".mmd")
		seen[name] = true
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(mmdPath)
			if err != nil {
				t.Fatalf("read mmd: %v", err)
			}
			refData, err := os.ReadFile(strings.TrimSuffix(mmdPath, ".mmd") + ".ref.svg")
			if err != nil {
				t.Fatalf("read ref svg: %v", err)
			}
			parsed, err := parse.Parse(string(src))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			laidOut, err := Layout(parsed, Default())
			if err != nil {
				t.Fatalf("layout failed: %v", err)
			}
			ref, err := refGeometry(refData)
			if err != nil {
				t.Fatalf("reading ref svg: %v", err)
			}
			got := layoutGeometry(laidOut)

			known := map[string]bool{}
			for _, k := range knownDiffs[name] {
				known[k] = true
			}
			kinds := map[string]bool{}
			for k := range got {
				kinds[k] = true
			}
			for k := range ref {
				kinds[k] = true
			}
			for k := range known {
				kinds[k] = true
			}
			var sorted []string
			for k := range kinds {
				sorted = append(sorted, k)
			}
			sort.Strings(sorted)

			for _, kind := range sorted {
				diffs := compareKind(got[kind], ref[kind])
				switch {
				case len(diffs) > 0 && known[kind]:
					t.Logf("known difference in %s:\n  %s", kind, strings.Join(diffs, "\n  "))
				case len(diffs) > 0:
					t.Errorf("%s doesn't match Mermaid:\n  %s", kind, strings.Join(diffs, "\n  "))
				case known[kind]:
					t.Errorf("%s matches Mermaid now; remove it from knownDiffs[%q]", kind, name)
				}
			}
		})
	}
	for name := range knownDiffs {
		if !seen[name] {
			t.Errorf("knownDiffs names %q, which isn't in seq/testdata", name)
		}
	}
}
