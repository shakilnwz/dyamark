package parse

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Entity codes such as #59; or #quot; are swapped for these markers before
// a line is split, so their `#` and `;` aren't read as a comment or a
// statement separator, as Mermaid's encodeEntities does. They are private-use
// runes, which diagram text won't contain.
const (
	entityOpen  = ""
	entityClose = ""
)

var entityCode = regexp.MustCompile(`#\w+;`)

func encodeEntities(s string) string {
	if !strings.Contains(s, "#") {
		return s
	}
	return entityCode.ReplaceAllStringFunc(s, func(m string) string {
		return entityOpen + m[1:len(m)-1] + entityClose
	})
}

var encodedEntity = regexp.MustCompile(entityOpen + `([^` + entityClose + `]*)` + entityClose)

// decodeEntities turns encoded entity codes into the characters they name:
// #59; is ";", #9829; is "♥", #quot; is `"`. Unknown names stay as written.
func decodeEntities(s string) string {
	if !strings.Contains(s, entityOpen) {
		return s
	}
	return encodedEntity.ReplaceAllStringFunc(s, func(m string) string {
		name := m[len(entityOpen) : len(m)-len(entityClose)]
		if n, err := strconv.ParseUint(strings.TrimPrefix(name, "+"), 10, 32); err == nil {
			return string(rune(n))
		}
		if dec := html.UnescapeString("&" + name + ";"); dec != "&"+name+";" {
			return dec
		}
		return "#" + name + ";"
	})
}

// lineBreak is Mermaid's lineBreakRegex: <br>, <br/>, <br />, </br>.
var lineBreak = regexp.MustCompile(`(?i)</?br\s*/?>`)

// displayText turns label source into the text to draw: entity codes
// decoded and <br> tags as newlines.
func displayText(s string) string {
	return decodeEntities(lineBreak.ReplaceAllString(s, "\n"))
}

// parseText reads a label the way Mermaid's parseMessage does: an optional
// `wrap:` or `nowrap:` prefix, then the text. Without a prefix, the wrap
// default applies.
func parseText(s string, wrapDefault bool) (text string, wrap bool) {
	s = strings.TrimSpace(s)
	wrap = wrapDefault
	t := strings.TrimPrefix(s, ":")
	if r, ok := strings.CutPrefix(t, "wrap:"); ok {
		s, wrap = r, true
	} else if r, ok := strings.CutPrefix(t, "nowrap:"); ok {
		s, wrap = r, false
	}
	return displayText(strings.TrimSpace(s)), wrap
}

// boxColorPattern is Mermaid's parseBoxData split: a colour function or a
// word, then the label. `#` starts a comment, so hex colours never get here.
var boxColorPattern = regexp.MustCompile(`^((?:rgba?|hsla?)\s*\(.*\)|\w*)(.*)$`)

var colorFunction = regexp.MustCompile(`(?i)^(rgba?|hsla?)\([-+0-9.,%\s/a-z]*\)$`)

// parseBoxData splits a box line into its fill and label. Mermaid keeps the
// first word as the colour only when the browser accepts it as one;
// otherwise the fill is transparent and the whole line is the label.
func parseBoxData(s string, wrapDefault bool) (fill, label string, wrap bool) {
	s = strings.TrimSpace(s)
	m := boxColorPattern.FindStringSubmatch(s)
	fill = strings.TrimSpace(m[1])
	rest := m[2]
	if fill == "" {
		fill = "transparent"
	}
	if !isCSSColor(fill) {
		fill, rest = "transparent", s
	}
	label, wrap = parseText(rest, wrapDefault)
	return fill, label, wrap
}

func isCSSColor(s string) bool {
	if colorFunction.MatchString(s) {
		return true
	}
	_, ok := cssColorNames[strings.ToLower(s)]
	return ok
}

// cssColorNames is every keyword a browser accepts as a colour: the CSS
// named colours plus transparent, currentcolor and the global keywords.
var cssColorNames = func() map[string]struct{} {
	names := `aliceblue antiquewhite aqua aquamarine azure beige bisque black
blanchedalmond blue blueviolet brown burlywood cadetblue chartreuse chocolate
coral cornflowerblue cornsilk crimson cyan darkblue darkcyan darkgoldenrod
darkgray darkgreen darkgrey darkkhaki darkmagenta darkolivegreen darkorange
darkorchid darkred darksalmon darkseagreen darkslateblue darkslategray
darkslategrey darkturquoise darkviolet deeppink deepskyblue dimgray dimgrey
dodgerblue firebrick floralwhite forestgreen fuchsia gainsboro ghostwhite gold
goldenrod gray green greenyellow grey honeydew hotpink indianred indigo ivory
khaki lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral
lightcyan lightgoldenrodyellow lightgray lightgreen lightgrey lightpink
lightsalmon lightseagreen lightskyblue lightslategray lightslategrey
lightsteelblue lightyellow lime limegreen linen magenta maroon
mediumaquamarine mediumblue mediumorchid mediumpurple mediumseagreen
mediumslateblue mediumspringgreen mediumturquoise mediumvioletred midnightblue
mintcream mistyrose moccasin navajowhite navy oldlace olive olivedrab orange
orangered orchid palegoldenrod palegreen paleturquoise palevioletred
papayawhip peachpuff peru pink plum powderblue purple rebeccapurple red
rosybrown royalblue saddlebrown salmon sandybrown seagreen seashell sienna
silver skyblue slateblue slategray slategrey snow springgreen steelblue tan
teal thistle tomato turquoise violet wheat white whitesmoke yellow
yellowgreen transparent currentcolor inherit initial unset revert`
	m := map[string]struct{}{}
	for _, n := range strings.Fields(names) {
		m[n] = struct{}{}
	}
	return m
}()
