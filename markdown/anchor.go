package markdown

import (
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// Anchor returns the index of the heading block a link's fragment names,
// the part after "#", or -1 when no heading has it. Fragments are named
// the way GitHub names them: the heading's text in lower case, without
// punctuation, with a hyphen for each space, and "-1", "-2", … after the
// second and later headings of the same name.
func (d *ParsedDocument) Anchor(fragment string) int {
	if s, err := url.PathUnescape(fragment); err == nil {
		fragment = s
	}
	if fragment == "" {
		return -1
	}
	seen := map[string]int{}
	folded := -1
	for i, b := range d.Blocks {
		h, ok := b.(*HeadingBlock)
		if !ok {
			continue
		}
		s := uniqueSlug(slug(h.Text), seen)
		if s == fragment {
			return i
		}
		if folded < 0 && s == strings.ToLower(fragment) {
			folded = i
		}
	}
	return folded
}

// slug returns a heading's anchor as GitHub makes it, before duplicates
// are numbered.
func slug(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// uniqueSlug numbers s if an earlier heading took it, and notes the
// result as taken. seen counts, for each slug, the numbers given after it.
func uniqueSlug(s string, seen map[string]int) string {
	u := s
	for {
		if _, taken := seen[u]; !taken {
			break
		}
		seen[s]++
		u = s + "-" + strconv.Itoa(seen[s])
	}
	seen[u] = 0
	return u
}
