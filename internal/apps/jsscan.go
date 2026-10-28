package apps

import (
	"regexp"
	"strings"
)

var (
	// specifierContext matches scanned code that ends where a module
	// specifier string belongs.
	specifierContext = regexp.MustCompile(`(?:\brequire\s*\(|\bimport|\bfrom)\s*$`)
	regexKeywords    = map[string]bool{
		"await": true, "case": true, "delete": true, "do": true, "else": true, "in": true, "instanceof": true,
		"new": true, "of": true, "return": true, "throw": true, "typeof": true, "void": true, "yield": true,
	}
)

// codeSkeleton reduces source to what dependency scanning should see:
// comments and regex literals are dropped and string literals are emptied
// unless they sit where a module specifier belongs (require(, import, from),
// so require-like text in strings, comments or regexes never creates an edge.
func codeSkeleton(source string) string {
	var b strings.Builder
	b.Grow(len(source))
	for i := 0; i < len(source); {
		c := source[i]
		var next byte
		if i+1 < len(source) {
			next = source[i+1]
		}
		switch {
		case c == '"' || c == '\'' || c == '`':
			var end int
			var exprs []string
			if c == '`' {
				end, exprs = scanTemplate(source, i)
			} else {
				end = scanQuoted(source, i)
			}
			out := b.String()
			if specifierContext.MatchString(out[max(0, len(out)-64):]) {
				b.WriteString(source[i:end])
			} else {
				b.WriteByte(c)
				b.WriteByte(c)
			}
			// ${...} interpolations are code and can require modules.
			for _, expr := range exprs {
				b.WriteString(" (" + codeSkeleton(expr) + ")")
			}
			i = end
		case c == '/' && next == '/':
			end := strings.IndexByte(source[i:], '\n')
			if end < 0 {
				return b.String()
			}
			i += end
		case c == '/' && next == '*':
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				return b.String()
			}
			b.WriteByte(' ')
			i += 2 + end + 2
		case c == '/' && regexAllowed(b.String()):
			end, ok := scanRegex(source, i)
			if !ok {
				b.WriteByte(c)
				i++
				continue
			}
			b.WriteByte('0') // stands in for the regex operand
			i = end
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// scanQuoted returns the index just past the single- or double-quoted
// literal starting at start. Unterminated literals end at the newline.
func scanQuoted(source string, start int) int {
	quote := source[start]
	j := start + 1
	for j < len(source) && source[j] != quote {
		switch source[j] {
		case '\\':
			j++
		case '\n':
			return j
		}
		j++
	}
	return min(j+1, len(source))
}

// scanTemplate returns the index just past the template literal starting at
// start, plus the source of its ${...} interpolations.
func scanTemplate(source string, start int) (int, []string) {
	var exprs []string
	for j := start + 1; j < len(source); {
		switch {
		case source[j] == '\\':
			j += 2
		case source[j] == '`':
			return j + 1, exprs
		case source[j] == '$' && j+1 < len(source) && source[j+1] == '{':
			end := scanBraces(source, j+1)
			exprs = append(exprs, strings.TrimSuffix(source[j+2:end], "}"))
			j = end
		default:
			j++
		}
	}
	return len(source), exprs
}

// scanBraces returns the index just past the '}' matching the '{' at start,
// skipping braces inside literals, comments and regexes.
func scanBraces(source string, start int) int {
	depth := 0
	for j := start; j < len(source); {
		switch source[j] {
		case '{':
			depth++
			j++
		case '}':
			depth--
			j++
			if depth == 0 {
				return j
			}
		case '"', '\'':
			j = scanQuoted(source, j)
		case '`':
			j, _ = scanTemplate(source, j)
		case '/':
			j = skipSlash(source, start+1, j)
		default:
			j++
		}
	}
	return len(source)
}

// skipSlash returns the index after the comment, regex literal or division
// operator at j; code starts at from.
func skipSlash(source string, from, j int) int {
	if j+1 < len(source) && source[j+1] == '/' {
		if end := strings.IndexByte(source[j:], '\n'); end >= 0 {
			return j + end
		}
		return len(source)
	}
	if j+1 < len(source) && source[j+1] == '*' {
		if end := strings.Index(source[j+2:], "*/"); end >= 0 {
			return j + 2 + end + 2
		}
		return len(source)
	}
	if regexAllowed(source[from:j]) {
		if end, ok := scanRegex(source, j); ok {
			return end
		}
	}
	return j + 1
}

// scanRegex returns the index just past the regex literal (and its flags)
// starting at start, or false when no literal closes on the same line.
func scanRegex(source string, start int) (int, bool) {
	inClass := false
	for j := start + 1; j < len(source); j++ {
		switch source[j] {
		case '\\':
			j++
		case '\n':
			return 0, false
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if inClass {
				continue
			}
			j++
			for j < len(source) && isIdentByte(source[j]) {
				j++
			}
			return j, true
		}
	}
	return 0, false
}

// regexAllowed reports whether a '/' after out starts a regex literal rather
// than a division: it does unless it follows an operand (identifier, number,
// literal, closing bracket) other than a keyword such as return or typeof.
func regexAllowed(out string) bool {
	end := trimSpaceEnd(out)
	if end == 0 {
		return true
	}
	switch last := out[end-1]; {
	case last == ')':
		return closesControlFlow(out[:end])
	case last == ']' || last == '"' || last == '\'' || last == '`':
		return false
	case (last == '+' || last == '-') && end > 1 && out[end-2] == last:
		return false // postfix ++/-- ends an operand
	case isIdentByte(last):
		start := end
		for start > 0 && isIdentByte(out[start-1]) {
			start--
		}
		return regexKeywords[out[start:end]]
	}
	return true
}

// closesControlFlow reports whether out ends with the ")" of an if, while,
// for or with head, after which a statement (possibly a regex) starts.
func closesControlFlow(out string) bool {
	depth := 0
	for i := len(out) - 1; i >= 0; i-- {
		switch out[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth > 0 {
				continue
			}
			end := trimSpaceEnd(out[:i])
			start := end
			for start > 0 && isIdentByte(out[start-1]) {
				start--
			}
			switch out[start:end] {
			case "if", "while", "for", "with":
				return true
			}
			return false
		}
	}
	return false
}

func trimSpaceEnd(s string) int {
	end := len(s)
	for end > 0 && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return end
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}
