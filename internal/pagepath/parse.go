package pagepath

import (
	"strconv"
	"strings"
)

// Selector kinds.
const (
	SelIndex = iota
	SelID
	SelSlug
)

// Path is a parsed human or raw page path.
type Path struct {
	Raw      string
	Field    string
	Segments []Segment
}

// Segment is one dotted path component, optionally with a selector.
type Segment struct {
	Name     string
	Selector *Selector
}

// Selector picks a repeatable by index, id, or slug.
type Selector struct {
	Kind  int
	Index int
	ID    string
	Slug  string
}

var pageFields = map[string]struct{}{
	"title":              {},
	"slug":               {},
	"seo_title":          {},
	"seo_description":    {},
	"seo_keywords":       {},
	"published":          {},
	"og_image":           {},
	"security_mechanism": {},
	"template":           {},
}

var pageFieldOrder = []string{
	"title",
	"slug",
	"seo_title",
	"seo_description",
	"seo_keywords",
	"published",
	"og_image",
	"security_mechanism",
	"template",
}

// Parse parses a human path or a slash-prefixed raw API path.
func Parse(input string) (Path, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Path{}, &ParseError{Input: "", Message: "empty path"}
	}
	if strings.HasPrefix(trimmed, "/") {
		return Path{Raw: trimmed}, nil
	}

	segs, err := parseSegments(trimmed)
	if err != nil {
		return Path{}, err
	}
	if len(segs) == 1 && segs[0].Selector == nil && isPageField(segs[0].Name) {
		return Path{Field: segs[0].Name, Raw: "/" + segs[0].Name}, nil
	}
	if segs[0].Selector != nil && isPageField(segs[0].Name) {
		return Path{}, parseErrorf(trimmed, "selector not allowed on page field %q", segs[0].Name)
	}
	return Path{Segments: segs}, nil
}

func isPageField(name string) bool {
	_, ok := pageFields[name]
	return ok
}

// IsPageField reports whether name is a top-level page field. A single-segment
// human path with this name always parses as the page field, never as an
// editable, so callers that build paths must fall back to the raw path when an
// editable name collides with one.
func IsPageField(name string) bool {
	return isPageField(name)
}

func parseSegments(input string) ([]Segment, error) {
	var segs []Segment
	i := 0
	for i < len(input) {
		seg, next, err := parseSegment(input, i)
		if err != nil {
			return nil, err
		}
		segs = append(segs, seg)
		i = next
		if i == len(input) {
			break
		}
		if input[i] != '.' {
			return nil, &ParseError{Input: input, Message: "expected '.'"}
		}
		i++
		if i == len(input) {
			return nil, &ParseError{Input: input, Message: "trailing '.'"}
		}
	}
	if len(segs) == 0 {
		return nil, &ParseError{Input: input, Message: "empty path"}
	}
	return segs, nil
}

func parseSegment(input string, start int) (Segment, int, error) {
	var name string
	i := start
	if i < len(input) && input[i] == '"' {
		n, next, err := parseQuoted(input, i)
		if err != nil {
			return Segment{}, 0, err
		}
		name = n
		i = next
	} else {
		j := i
		for j < len(input) && input[j] != '.' && input[j] != '[' {
			j++
		}
		name = input[i:j]
		i = j
	}
	if name == "" {
		return Segment{}, 0, &ParseError{Input: input, Message: "empty name"}
	}
	if i < len(input) && input[i] == '[' {
		sel, next, err := parseSelector(input, i)
		if err != nil {
			return Segment{}, 0, err
		}
		return Segment{Name: name, Selector: &sel}, next, nil
	}
	return Segment{Name: name}, i, nil
}

func parseQuoted(input string, start int) (string, int, error) {
	var b strings.Builder
	i := start + 1
	for i < len(input) {
		if input[i] == '\\' {
			if i+1 >= len(input) {
				return "", 0, &ParseError{Input: input, Message: "unbalanced quotes"}
			}
			b.WriteByte(input[i+1])
			i += 2
			continue
		}
		if input[i] == '"' {
			return b.String(), i + 1, nil
		}
		b.WriteByte(input[i])
		i++
	}
	return "", 0, &ParseError{Input: input, Message: "unbalanced quotes"}
}

func parseSelector(input string, start int) (Selector, int, error) {
	end := start + 1
	for end < len(input) && input[end] != ']' {
		end++
	}
	if end >= len(input) {
		return Selector{}, 0, &ParseError{Input: input, Message: "unbalanced brackets"}
	}
	inner := strings.TrimSpace(input[start+1 : end])
	sel, err := parseSelectorInner(input, inner)
	if err != nil {
		return Selector{}, 0, err
	}
	return sel, end + 1, nil
}

func parseSelectorInner(input, inner string) (Selector, error) {
	if inner == "" {
		return Selector{}, &ParseError{Input: input, Message: "invalid selector"}
	}
	if strings.HasPrefix(inner, "id=") {
		id := inner[len("id="):]
		if id == "" {
			return Selector{}, &ParseError{Input: input, Message: "invalid selector"}
		}
		return Selector{Kind: SelID, ID: id}, nil
	}
	if strings.HasPrefix(inner, "slug=") {
		slug := inner[len("slug="):]
		if slug == "" {
			return Selector{}, &ParseError{Input: input, Message: "invalid selector"}
		}
		return Selector{Kind: SelSlug, Slug: slug}, nil
	}
	idx, err := strconv.Atoi(inner)
	if err != nil {
		return Selector{}, &ParseError{Input: input, Message: "invalid selector"}
	}
	if idx < 0 {
		return Selector{}, &ParseError{Input: input, Message: "negative index"}
	}
	return Selector{Kind: SelIndex, Index: idx}, nil
}

// String renders the canonical human form, quoting names that need it.
func (p Path) String() string {
	if p.Field != "" {
		return p.Field
	}
	if p.Raw != "" && len(p.Segments) == 0 {
		return p.Raw
	}
	var b strings.Builder
	for i, seg := range p.Segments {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(quoteName(seg.Name))
		if seg.Selector == nil {
			continue
		}
		b.WriteByte('[')
		switch seg.Selector.Kind {
		case SelID:
			b.WriteString("id=")
			b.WriteString(seg.Selector.ID)
		case SelSlug:
			b.WriteString("slug=")
			b.WriteString(seg.Selector.Slug)
		default:
			b.WriteString(strconv.Itoa(seg.Selector.Index))
		}
		b.WriteByte(']')
	}
	return b.String()
}

func quoteName(name string) string {
	if !strings.ContainsAny(name, ".[\"") {
		return name
	}
	escaped := strings.ReplaceAll(name, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
