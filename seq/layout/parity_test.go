package layout

import (
	"encoding/xml"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"dyamark/seq/parse"
)

type svgElement struct {
	Tag        string
	Attrs      map[string]string
	Text       string
	Name       string
	DataID     string
	DataFrom   string
	DataTo     string
	Class      string
	X, Y, W, H float32
	X1, Y1     float32
	X2, Y2     float32
}

type svgDoc struct {
	ViewBox     Rect
	TopActors   map[string]svgElement
	BotActors   map[string]svgElement
	Lifelines   map[string]svgElement
	Messages    []svgElement
	MsgTexts    []svgElement
	Notes       []svgElement
	Activations []svgElement
	Frames      []svgElement
}

func parseFloat(s string) float32 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "px")
	f, _ := strconv.ParseFloat(s, 32)
	return float32(f)
}

func parseRefSVG(data []byte) (*svgDoc, error) {
	doc := &svgDoc{
		TopActors: make(map[string]svgElement),
		BotActors: make(map[string]svgElement),
		Lifelines: make(map[string]svgElement),
	}
	d := xml.NewDecoder(strings.NewReader(string(data)))

	var curText *svgElement
	var textBuf strings.Builder

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

			switch elem.Name.Local {
			case "svg":
				if vb, ok := attrs["viewBox"]; ok {
					parts := strings.Fields(vb)
					if len(parts) == 4 {
						doc.ViewBox = Rect{
							X: parseFloat(parts[0]),
							Y: parseFloat(parts[1]),
							W: parseFloat(parts[2]),
							H: parseFloat(parts[3]),
						}
					}
				}

			case "rect":
				el := svgElement{
					Tag:    "rect",
					Attrs:  attrs,
					Class:  cls,
					Name:   name,
					DataID: attrs["data-id"],
					X:      parseFloat(attrs["x"]),
					Y:      parseFloat(attrs["y"]),
					W:      parseFloat(attrs["width"]),
					H:      parseFloat(attrs["height"]),
				}
				if strings.Contains(cls, "actor-top") {
					doc.TopActors[name] = el
				} else if strings.Contains(cls, "actor-bottom") {
					doc.BotActors[name] = el
				} else if strings.Contains(cls, "activation") {
					doc.Activations = append(doc.Activations, el)
				} else if strings.Contains(cls, "note") {
					doc.Notes = append(doc.Notes, el)
				} else if strings.Contains(cls, "labelBox") {
					doc.Frames = append(doc.Frames, el)
				}

			case "line":
				el := svgElement{
					Tag:      "line",
					Attrs:    attrs,
					Class:    cls,
					Name:     name,
					DataID:   attrs["data-id"],
					DataFrom: attrs["data-from"],
					DataTo:   attrs["data-to"],
					X1:       parseFloat(attrs["x1"]),
					Y1:       parseFloat(attrs["y1"]),
					X2:       parseFloat(attrs["x2"]),
					Y2:       parseFloat(attrs["y2"]),
				}
				if strings.Contains(cls, "actor-line") {
					key := attrs["name"]
					if key == "" {
						key = attrs["data-id"]
					}
					doc.Lifelines[key] = el
				} else if strings.Contains(cls, "messageLine") {
					doc.Messages = append(doc.Messages, el)
				}

			case "text":
				el := svgElement{
					Tag:   "text",
					Attrs: attrs,
					Class: cls,
					X:     parseFloat(attrs["x"]),
					Y:     parseFloat(attrs["y"]),
				}
				curText = &el
				textBuf.Reset()
			}

		case xml.CharData:
			if curText != nil {
				textBuf.Write(elem)
			}

		case xml.EndElement:
			if elem.Name.Local == "text" && curText != nil {
				curText.Text = strings.TrimSpace(textBuf.String())
				if strings.Contains(curText.Class, "messageText") {
					doc.MsgTexts = append(doc.MsgTexts, *curText)
				}
				curText = nil
			}
		}
	}

	return doc, nil
}

func approxEq(a, b, tol float32) bool {
	return float32(math.Abs(float64(a-b))) <= tol
}

func TestParityAgainstMermaid(t *testing.T) {
	matches, err := filepath.Glob("../testdata/*.mmd")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) < 30 {
		t.Fatalf("expected at least 30 diagrams in seq/testdata, got %d", len(matches))
	}

	for _, mmdPath := range matches {
		name := strings.TrimSuffix(filepath.Base(mmdPath), ".mmd")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(mmdPath)
			if err != nil {
				t.Fatalf("read mmd: %v", err)
			}
			refPath := strings.TrimSuffix(mmdPath, ".mmd") + ".ref.svg"
			refData, err := os.ReadFile(refPath)
			if err != nil {
				t.Fatalf("read ref svg: %v", err)
			}

			parsedDiagram, err := parse.Parse(string(src))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			laidOut, err := Layout(parsedDiagram, Default())
			if err != nil {
				t.Fatalf("layout failed: %v", err)
			}

			refDoc, err := parseRefSVG(refData)
			if err != nil {
				t.Fatalf("parse ref svg failed: %v", err)
			}

			tol := float32(2.0)

			// 1. ViewBox comparison
			if !approxEq(laidOut.ViewBox.X, refDoc.ViewBox.X, tol) ||
				!approxEq(laidOut.ViewBox.Y, refDoc.ViewBox.Y, tol) ||
				!approxEq(laidOut.ViewBox.W, refDoc.ViewBox.W, tol) ||
				!approxEq(laidOut.ViewBox.H, refDoc.ViewBox.H, tol) {
				t.Errorf("ViewBox mismatch: got %+v, ref %+v", laidOut.ViewBox, refDoc.ViewBox)
			}

			// 2. Top actor boxes
			for _, a := range laidOut.Actors {
				if a.Footer {
					continue
				}
				refActor, ok := refDoc.TopActors[a.ID]
				if !ok {
					t.Errorf("Top Actor %s missing from reference SVG", a.ID)
					continue
				}
				got := a.Rect
				ref := Rect{X: refActor.X, Y: refActor.Y, W: refActor.W, H: refActor.H}
				if !approxEq(got.X, ref.X, tol) || !approxEq(got.Y, ref.Y, tol) ||
					!approxEq(got.W, ref.W, tol) || !approxEq(got.H, ref.H, tol) {
					t.Errorf("Top Actor %s rect mismatch: got %+v, ref %+v", a.ID, got, ref)
				}
			}

			// 3. Lifelines
			for _, l := range laidOut.Lifelines {
				refLine, ok := refDoc.Lifelines[l.ID]
				if !ok {
					t.Errorf("Lifeline %s missing from reference SVG", l.ID)
					continue
				}
				if !approxEq(l.X, refLine.X1, tol) || !approxEq(l.Y1, refLine.Y1, tol) || !approxEq(l.Y2, refLine.Y2, tol) {
					t.Errorf("Lifeline %s mismatch: got (x=%.1f, y1=%.1f, y2=%.1f), ref (x=%.1f, y1=%.1f, y2=%.1f)",
						l.ID, l.X, l.Y1, l.Y2, refLine.X1, refLine.Y1, refLine.Y2)
				}
			}

			// 4. Message lines
			if len(laidOut.Messages) == len(refDoc.Messages) {
				for i := range laidOut.Messages {
					got := laidOut.Messages[i]
					ref := refDoc.Messages[i]
					if len(got.Path) >= 2 {
						gotP1 := got.Path[0]
						gotP2 := got.Path[len(got.Path)-1]
						if !approxEq(gotP1.X, ref.X1, tol) || !approxEq(gotP1.Y, ref.Y1, tol) ||
							!approxEq(gotP2.X, ref.X2, tol) || !approxEq(gotP2.Y, ref.Y2, tol) {
							t.Errorf("Message line %d (%s->%s) mismatch: got (%.1f,%.1f -> %.1f,%.1f), ref (%.1f,%.1f -> %.1f,%.1f)",
								i, got.From, got.To, gotP1.X, gotP1.Y, gotP2.X, gotP2.Y, ref.X1, ref.Y1, ref.X2, ref.Y2)
						}
					}
				}
			}

			// 5. Notes
			if len(laidOut.Notes) == len(refDoc.Notes) {
				for i := range laidOut.Notes {
					got := laidOut.Notes[i].Rect
					ref := Rect{X: refDoc.Notes[i].X, Y: refDoc.Notes[i].Y, W: refDoc.Notes[i].W, H: refDoc.Notes[i].H}
					if !approxEq(got.X, ref.X, tol) || !approxEq(got.Y, ref.Y, tol) ||
						!approxEq(got.W, ref.W, tol) || !approxEq(got.H, ref.H, tol) {
						t.Errorf("Note %d rect mismatch: got %+v, ref %+v", i, got, ref)
					}
				}
			}

			// 6. Activations
			if len(laidOut.Activations) == len(refDoc.Activations) {
				for i := range laidOut.Activations {
					// Match activation by closest bounds
					got := laidOut.Activations[i].Rect
					found := false
					for _, refAct := range refDoc.Activations {
						ref := Rect{X: refAct.X, Y: refAct.Y, W: refAct.W, H: refAct.H}
						if approxEq(got.X, ref.X, tol) && approxEq(got.Y, ref.Y, tol) &&
							approxEq(got.W, ref.W, tol) && approxEq(got.H, ref.H, tol) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Activation %d rect %+v not found in ref activations", i, got)
					}
				}
			}
		})
	}
}
