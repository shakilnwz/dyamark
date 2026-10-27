package parse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Config is a diagram's own configuration, merged the way Mermaid merges
// it: frontmatter `config:` first, then %%{init}%% directives on top.
// Unset fields are nil or empty, so the layout can fall back to its
// defaults.
type Config struct {
	Theme          string
	ThemeVariables map[string]string
	FontFamily     string
	FontSize       *float64
	// Wrap is the top-level wrap setting that %%{wrap}%% turns on. When
	// set, it wins over Sequence.Wrap.
	Wrap     *bool
	Sequence SequenceConfig
	// Raw is the merged configuration as written, keys and all.
	Raw map[string]any
}

// SequenceConfig is Mermaid's `sequence` configuration block.
type SequenceConfig struct {
	HideUnusedParticipants *bool    `json:"hideUnusedParticipants"`
	ActivationWidth        *float64 `json:"activationWidth"`
	DiagramMarginX         *float64 `json:"diagramMarginX"`
	DiagramMarginY         *float64 `json:"diagramMarginY"`
	ActorMargin            *float64 `json:"actorMargin"`
	Width                  *float64 `json:"width"`
	Height                 *float64 `json:"height"`
	BoxMargin              *float64 `json:"boxMargin"`
	BoxTextMargin          *float64 `json:"boxTextMargin"`
	NoteMargin             *float64 `json:"noteMargin"`
	MessageMargin          *float64 `json:"messageMargin"`
	MessageAlign           *string  `json:"messageAlign"`
	MirrorActors           *bool    `json:"mirrorActors"`
	BottomMarginAdj        *float64 `json:"bottomMarginAdj"`
	RightAngles            *bool    `json:"rightAngles"`
	ShowSequenceNumbers    *bool    `json:"showSequenceNumbers"`
	ActorFontSize          *float64 `json:"actorFontSize"`
	ActorFontFamily        *string  `json:"actorFontFamily"`
	ActorFontWeight        *string  `json:"actorFontWeight"`
	NoteFontSize           *float64 `json:"noteFontSize"`
	NoteFontFamily         *string  `json:"noteFontFamily"`
	NoteFontWeight         *string  `json:"noteFontWeight"`
	NoteAlign              *string  `json:"noteAlign"`
	MessageFontSize        *float64 `json:"messageFontSize"`
	MessageFontFamily      *string  `json:"messageFontFamily"`
	MessageFontWeight      *string  `json:"messageFontWeight"`
	Wrap                   *bool    `json:"wrap"`
	WrapPadding            *float64 `json:"wrapPadding"`
	LabelBoxWidth          *float64 `json:"labelBoxWidth"`
	LabelBoxHeight         *float64 `json:"labelBoxHeight"`
	// FontFamily and FontSize arrive here when an init directive uses the
	// diagram-neutral `config` key.
	FontFamily *string  `json:"fontFamily"`
	FontSize   *float64 `json:"fontSize"`
}

// WrapByDefault is whether labels wrap when they don't say wrap: or nowrap:.
func (c Config) WrapByDefault() bool {
	if c.Wrap != nil {
		return *c.Wrap
	}
	if c.Sequence.Wrap != nil {
		return *c.Sequence.Wrap
	}
	return false
}

// frontmatter is the YAML block Mermaid accepts before the diagram.
type frontmatter struct {
	title  string
	config map[string]any
}

// splitFrontmatter cuts a leading `---` YAML block off src, replacing it with
// as many empty lines, so line numbers still match the source. Mermaid
// requires the closing `---` at the same indent as the opening one.
func splitFrontmatter(src string) (fm frontmatter, rest string, err error) {
	lines := strings.SplitAfter(src, "\n")
	open := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if strings.TrimSpace(l) == "---" {
			open = i
		}
		break
	}
	if open < 0 {
		return fm, src, nil
	}
	indent := lines[open][:len(lines[open])-len(strings.TrimLeft(lines[open], " \t"))]
	closing := -1
	for i := open + 1; i < len(lines); i++ {
		l := strings.TrimRight(lines[i], " \t\r\n")
		if l == indent+"---" {
			closing = i
			break
		}
	}
	if closing < 0 {
		return fm, src, nil
	}

	var body strings.Builder
	for _, l := range lines[open+1 : closing] {
		body.WriteString(strings.TrimPrefix(l, indent))
	}
	blank := strings.Repeat("\n", closing+1)
	rest = blank + strings.Join(lines[closing+1:], "")

	var meta map[string]any
	if err := yaml.Unmarshal([]byte(body.String()), &meta); err != nil {
		return fm, rest, &Error{Line: open + 1, Msg: "frontmatter: " + strings.TrimPrefix(err.Error(), "yaml: ")}
	}
	if t, ok := meta["title"]; ok && t != nil {
		fm.title = fmt.Sprint(t)
	}
	if c, ok := meta["config"].(map[string]any); ok {
		fm.config = c
	}
	return fm, rest, nil
}

// directive is one %%{type: args}%% block.
type directive struct {
	typ  string
	args string
	line int
}

// extractDirectives finds %%{…}%% directives, which may span lines, and
// blanks them out of src, keeping the newlines inside them so line numbers
// still match.
func extractDirectives(src string) ([]directive, string) {
	var dirs []directive
	var out strings.Builder
	for {
		start := strings.Index(src, "%%{")
		if start < 0 {
			out.WriteString(src)
			break
		}
		out.WriteString(src[:start])
		inner := src[start+3:]
		end := strings.Index(inner, "}%%")
		var raw string
		if end < 0 {
			raw, src = inner, ""
		} else {
			raw, src = inner[:end], inner[end+3:]
		}
		line := strings.Count(out.String(), "\n") + 1
		out.WriteString(strings.Repeat("\n", strings.Count(raw, "\n")))
		if d, ok := parseDirective(raw); ok {
			d.line = line
			dirs = append(dirs, d)
		}
	}
	return dirs, out.String()
}

// parseDirective splits the inside of a directive into its type and
// arguments, as Mermaid's directive regex does: `word` or `word: args`.
func parseDirective(raw string) (directive, bool) {
	s := strings.TrimSpace(raw)
	n := 0
	for n < len(s) && isWordByte(s[n]) {
		n++
	}
	if n == 0 {
		return directive{}, false
	}
	d := directive{typ: s[:n]}
	rest := strings.TrimSpace(s[n:])
	if r, ok := strings.CutPrefix(rest, ":"); ok {
		d.args = strings.TrimSpace(r)
	}
	return d, true
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// mergeConfig builds the diagram's Config from its frontmatter and
// directives, following Mermaid's preprocessDiagram.
func mergeConfig(fm frontmatter, dirs []directive) (Config, []Warning) {
	var warnings []Warning

	// Mermaid parses every init directive with JSON.parse after turning
	// single quotes into double ones. If any fails, all are dropped.
	var inits []map[string]any
	initsOK := true
	wrap := false
	for _, d := range dirs {
		switch strings.ToLower(d.typ) {
		case "init", "initialize":
			var m map[string]any
			dec := json.NewDecoder(strings.NewReader(strings.ReplaceAll(d.args, "'", `"`)))
			dec.UseNumber()
			if err := dec.Decode(&m); err != nil {
				warnings = append(warnings, Warning{Line: d.line, Msg: "ignoring init directives: " + err.Error()})
				initsOK = false
				continue
			}
			inits = append(inits, m)
		case "wrap":
			wrap = true
		}
	}

	merged := map[string]any{}
	deepMerge(merged, fm.config)
	if initsOK {
		directive := map[string]any{}
		for _, m := range inits {
			deepMerge(directive, m)
		}
		// The diagram-neutral `config` key belongs to the diagram type.
		if c, ok := directive["config"]; ok {
			directive["sequence"] = c
			delete(directive, "config")
		}
		deepMerge(merged, directive)
	}
	if wrap {
		merged["wrap"] = true
	}

	cfg, err := typedConfig(merged)
	if err != nil {
		warnings = append(warnings, Warning{Msg: "config: " + err.Error()})
	}
	return cfg, warnings
}

// deepMerge copies src into dst, merging nested maps instead of replacing them.
func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				deepMerge(dm, sm)
				continue
			}
			cp := map[string]any{}
			deepMerge(cp, sm)
			dst[k] = cp
			continue
		}
		dst[k] = v
	}
}

func typedConfig(raw map[string]any) (Config, error) {
	cfg := Config{Raw: raw}
	if s, ok := raw["theme"].(string); ok {
		cfg.Theme = s
	}
	if s, ok := raw["fontFamily"].(string); ok {
		cfg.FontFamily = s
	}
	if f, ok := number(raw["fontSize"]); ok {
		cfg.FontSize = &f
	}
	if b, ok := raw["wrap"].(bool); ok {
		cfg.Wrap = &b
	}
	if tv, ok := raw["themeVariables"].(map[string]any); ok {
		cfg.ThemeVariables = make(map[string]string, len(tv))
		for k, v := range tv {
			cfg.ThemeVariables[k] = fmt.Sprint(v)
		}
	}

	seq, ok := raw["sequence"].(map[string]any)
	if !ok {
		return cfg, nil
	}
	// Mermaid accepts sizes as numbers or strings like "16px", and weights
	// as numbers or strings. Normalise before decoding.
	norm := make(map[string]any, len(seq))
	for k, v := range seq {
		switch {
		case strings.HasSuffix(k, "FontSize") || k == "fontSize":
			if f, ok := number(v); ok {
				v = f
			}
		case strings.HasSuffix(k, "FontWeight"):
			if _, ok := v.(string); !ok {
				v = fmt.Sprint(v)
			}
		}
		norm[k] = v
	}
	data, err := json.Marshal(norm)
	if err != nil {
		return cfg, err
	}
	// A wrongly typed value leaves only that field unset.
	err = json.NewDecoder(bytes.NewReader(data)).Decode(&cfg.Sequence)
	return cfg, err
}

// number reads a number from JSON or YAML, or from a string the way
// JavaScript's parseFloat does ("16px" is 16).
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		s := strings.TrimSpace(n)
		end := 0
		for end < len(s) && (s[end] >= '0' && s[end] <= '9' || s[end] == '.' || end == 0 && (s[end] == '-' || s[end] == '+')) {
			end++
		}
		f, err := strconv.ParseFloat(s[:end], 64)
		return f, err == nil
	}
	return 0, false
}
