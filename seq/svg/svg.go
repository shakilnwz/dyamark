package svg

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"math"
	"strings"

	"dyamark/seq/layout"
)

// Render converts a laid-out Diagram into an SVG string.
func Render(diag *layout.Diagram) (string, error) {
	var buf bytes.Buffer
	if err := Write(&buf, diag); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Write renders the diagram as SVG to the given writer.
func Write(w io.Writer, diag *layout.Diagram) error {
	if diag == nil {
		return nil
	}

	vb := diag.ViewBox
	fmt.Fprintf(w, `<svg width="100%%" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="%d %d %d %d" role="graphics-document document">`,
		int(mathRound(float64(vb.X))),
		int(mathRound(float64(vb.Y))),
		int(mathRound(float64(vb.W))),
		int(mathRound(float64(vb.H))),
	)

	// Defs & markers
	w.Write([]byte(`<defs>`))
	w.Write([]byte(`<marker id="arrowhead" refX="7.9" refY="5" markerUnits="userSpaceOnUse" markerWidth="12" markerHeight="12" orient="auto-start-reverse"><path d="M -1 0 L 10 5 L 0 10 z"/></marker>`))
	w.Write([]byte(`<marker id="crosshead" markerWidth="15" markerHeight="8" orient="auto" refX="4" refY="4.5"><path fill="none" stroke="#000000" stroke-width="1pt" d="M 1,2 L 6,7 M 6,2 L 1,7"/></marker>`))
	w.Write([]byte(`<marker id="filled-head" refX="15.5" refY="7" markerWidth="20" markerHeight="28" orient="auto"><path d="M 18,7 L9,13 L14,7 L9,1 Z"/></marker>`))
	w.Write([]byte(`<marker id="sequencenumber" refX="15" refY="15" markerWidth="60" markerHeight="40" orient="auto"><circle cx="15" cy="15" r="6"/></marker>`))
	w.Write([]byte(`<marker id="solidTopArrowHead" refX="7.9" refY="7.25" markerUnits="userSpaceOnUse" markerWidth="12" markerHeight="12" orient="auto-start-reverse"><path d="M 0 0 L 10 8 L 0 8 z"/></marker>`))
	w.Write([]byte(`<marker id="solidBottomArrowHead" refX="7.9" refY="0.75" markerUnits="userSpaceOnUse" markerWidth="12" markerHeight="12" orient="auto-start-reverse"><path d="M 0 0 L 10 0 L 0 8 z"/></marker>`))
	w.Write([]byte(`<marker id="stickTopArrowHead" refX="7.5" refY="7" markerUnits="userSpaceOnUse" markerWidth="12" markerHeight="12" orient="auto-start-reverse"><path d="M 0 0 L 7 7" stroke="black" stroke-width="1.5" fill="none"/></marker>`))
	w.Write([]byte(`<marker id="stickBottomArrowHead" refX="7.5" refY="0" markerUnits="userSpaceOnUse" markerWidth="12" markerHeight="12" orient="auto-start-reverse"><path d="M 0 7 L 7 0" stroke="black" stroke-width="1.5" fill="none"/></marker>`))
	w.Write([]byte(`<filter id="drop-shadow" height="130%" width="130%"><feDropShadow dx="4" dy="4" stdDeviation="0" flood-opacity="0.06" flood-color="#000000"/></filter>`))
	w.Write([]byte(`</defs>`))

	// Default styles
	w.Write([]byte(`<style>` +
		`.actor{stroke:#28253D;fill:#ffffff;stroke-width:2;}` +
		`.actor-line{stroke:#28253D;stroke-width:0.5px;}` +
		`.messageLine0{stroke-width:1.5;stroke-dasharray:none;stroke:#28253D;fill:none;}` +
		`.messageLine1{stroke-width:1.5;stroke-dasharray:3,3;stroke:#28253D;fill:none;}` +
		`.messageText{fill:#28253D;stroke:none;font-family:"Liberation Sans",sans-serif;font-size:16px;}` +
		`.note{stroke:#FACC15;fill:#fff5ad;}` +
		`.noteText{fill:#28253D;stroke:none;font-family:"Liberation Sans",sans-serif;font-size:16px;}` +
		`.activation{fill:#e0e0e0;stroke:#b0b0b0;}` +
		`.labelBox{stroke:#28253D;fill:#ffffff;}` +
		`.loopLine{stroke-width:2px;stroke-dasharray:2,2;stroke:#28253D;}` +
		`.labelText{fill:#28253D;stroke:none;font-family:"Liberation Sans",sans-serif;}` +
		`text{font-family:"Liberation Sans",sans-serif;font-size:16px;}` +
		// After .actor, which would otherwise outline the labels in white.
		`.actor-box{fill:#28253D;stroke:none;stroke-width:0;}` +
		`.sequenceNumberText{fill:#ffffff;font-family:sans-serif;}` +
		`</style>`))

	// 1. Background Rects
	for _, bg := range diag.Rects {
		// Mermaid fills a rect with no colour in the actor background.
		fill := bg.Fill
		if fill == "" {
			fill = "#eaeaea"
		}
		fmt.Fprintf(w, `<rect class="rect" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" />`,
			bg.Rect.X, bg.Rect.Y, bg.Rect.W, bg.Rect.H, html.EscapeString(fill))
	}

	// 2. Boxes
	for _, b := range diag.Boxes {
		fill := b.Fill
		if fill == "" {
			fill = "none"
		}
		fmt.Fprintf(w, `<g class="box"><rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" stroke="#000000" stroke-opacity="0.5" />`,
			b.Rect.X, b.Rect.Y, b.Rect.W, b.Rect.H, html.EscapeString(fill))
		for _, t := range b.Label {
			renderText(w, t, "labelText")
		}
		w.Write([]byte(`</g>`))
	}

	// 3. Frames (Loops, Alt, Opt, Par, Critical, Break)
	for _, f := range diag.Frames {
		fmt.Fprintf(w, `<g class="frame"><rect class="labelBox" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="none" stroke="#28253D" />`,
			f.Rect.X, f.Rect.Y, f.Rect.W, f.Rect.H)
		if len(f.Tab) > 0 {
			var points []string
			for _, p := range f.Tab {
				points = append(points, fmt.Sprintf("%.1f,%.1f", p.X, p.Y))
			}
			fmt.Fprintf(w, `<polygon class="labelBox" points="%s" fill="#ffffff" stroke="#28253D" />`, strings.Join(points, " "))
		}
		for _, divY := range f.Dividers {
			fmt.Fprintf(w, `<line class="loopLine" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" />`,
				f.Rect.X, divY, f.Rect.X+f.Rect.W, divY)
		}
		for _, t := range f.Texts {
			renderText(w, t, "labelText")
		}
		w.Write([]byte(`</g>`))
	}

	// 4. Lifelines
	for _, l := range diag.Lifelines {
		fmt.Fprintf(w, `<line id="actor-line-%s" class="actor-line" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" />`,
			html.EscapeString(l.ID), l.X, l.Y1, l.X, l.Y2)
	}

	// 5. Activations
	for _, act := range diag.Activations {
		fmt.Fprintf(w, `<rect class="activation activation%d" x="%.1f" y="%.1f" width="%.1f" height="%.1f" />`,
			act.Depth, act.Rect.X, act.Rect.Y, act.Rect.W, act.Rect.H)
	}

	// 6. Messages
	for i, m := range diag.Messages {
		lineClass := "messageLine0"
		if m.Dashed {
			lineClass = "messageLine1"
		}
		markerAttr := ""
		if m.EndMarker != layout.MarkerNone {
			markerAttr = fmt.Sprintf(` marker-end="url(#%s)"`, markerName(m.EndMarker))
		}
		if m.StartMarker != layout.MarkerNone {
			markerAttr += fmt.Sprintf(` marker-start="url(#%s)"`, markerName(m.StartMarker))
		}

		if m.Curved && len(m.Path) == 4 {
			fmt.Fprintf(w, `<path class="%s" d="M %.1f,%.1f C %.1f,%.1f %.1f,%.1f %.1f,%.1f"%s />`,
				lineClass,
				m.Path[0].X, m.Path[0].Y,
				m.Path[1].X, m.Path[1].Y,
				m.Path[2].X, m.Path[2].Y,
				m.Path[3].X, m.Path[3].Y,
				markerAttr,
			)
		} else if len(m.Path) == 4 && !m.Curved {
			fmt.Fprintf(w, `<path class="%s" d="M %.1f,%.1f L %.1f,%.1f L %.1f,%.1f L %.1f,%.1f"%s />`,
				lineClass,
				m.Path[0].X, m.Path[0].Y,
				m.Path[1].X, m.Path[1].Y,
				m.Path[2].X, m.Path[2].Y,
				m.Path[3].X, m.Path[3].Y,
				markerAttr,
			)
		} else if len(m.Path) >= 2 {
			fmt.Fprintf(w, `<line id="message-line-%d" class="%s" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"%s />`,
				i, lineClass, m.Path[0].X, m.Path[0].Y, m.Path[len(m.Path)-1].X, m.Path[len(m.Path)-1].Y, markerAttr)
		}

		for _, t := range m.Text {
			renderText(w, t, "messageText")
		}

		if m.Number != nil {
			fmt.Fprintf(w, `<circle class="sequenceNumber" cx="%.1f" cy="%.1f" r="%.1f" fill="#28253D" />`,
				m.Number.Center.X, m.Number.Center.Y, m.Number.Radius)
			renderText(w, m.Number.Text, "sequenceNumberText")
		}
	}

	// 7. Notes
	for _, n := range diag.Notes {
		fmt.Fprintf(w, `<g class="note"><rect class="note" x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="3" ry="3" />`,
			n.Rect.X, n.Rect.Y, n.Rect.W, n.Rect.H)
		for _, t := range n.Text {
			renderText(w, t, "noteText")
		}
		w.Write([]byte(`</g>`))
	}

	// 8. Actors
	for _, a := range diag.Actors {
		actorClass := "actor actor-top"
		if a.Footer {
			actorClass = "actor actor-bottom"
		}
		fmt.Fprintf(w, `<g class="actor">`)
		if a.Figure != nil {
			// Stick figure
			fig := a.Figure
			fmt.Fprintf(w, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="#ffffff" stroke="#28253D" stroke-width="2" />`,
				fig.Head.X, fig.Head.Y, fig.Radius)
			for _, line := range fig.Lines {
				fmt.Fprintf(w, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#28253D" stroke-width="2" />`,
					line[0].X, line[0].Y, line[1].X, line[1].Y)
			}
		} else {
			fmt.Fprintf(w, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f" ry="%.1f" />`,
				actorClass, a.Rect.X, a.Rect.Y, a.Rect.W, a.Rect.H, a.Radius, a.Radius)
		}

		for _, t := range a.Label {
			renderText(w, t, "actor actor-box")
		}
		w.Write([]byte(`</g>`))
	}

	// 9. Title
	if diag.Title != nil {
		renderText(w, *diag.Title, "titleText")
	}

	w.Write([]byte(`</svg>`))
	return nil
}

func renderText(w io.Writer, t layout.Text, defaultClass string) {
	anchor := "middle"
	switch t.Anchor {
	case layout.AnchorStart:
		anchor = "start"
	case layout.AnchorEnd:
		anchor = "end"
	}

	baseline := "central"
	switch t.Baseline {
	case layout.BaselineAlphabetic:
		baseline = "alphabetic"
	case layout.BaselineMiddle:
		baseline = "middle"
	}

	dyAttr := ""
	if t.DY != 0 {
		dyAttr = fmt.Sprintf(` dy="%.1f"`, t.DY)
	}

	fmt.Fprintf(w, `<text class="%s" x="%.1f" y="%.1f" text-anchor="%s" dominant-baseline="%s"%s style="font-size: %.1fpx; font-weight: %d;">%s</text>`,
		defaultClass, t.X, t.Y, anchor, baseline, dyAttr, t.Size, t.Weight, html.EscapeString(t.Text))
}

func markerName(m layout.Marker) string {
	switch m {
	case layout.MarkerArrow:
		return "arrowhead"
	case layout.MarkerCross:
		return "crosshead"
	case layout.MarkerOpen:
		return "filled-head"
	case layout.MarkerSolidTop:
		return "solidTopArrowHead"
	case layout.MarkerSolidBottom:
		return "solidBottomArrowHead"
	case layout.MarkerStickTop:
		return "stickTopArrowHead"
	case layout.MarkerStickBottom:
		return "stickBottomArrowHead"
	default:
		return ""
	}
}

func mathRound(v float64) float64 {
	if v < 0 {
		return math.Ceil(v - 0.5)
	}
	return math.Floor(v + 0.5)
}
