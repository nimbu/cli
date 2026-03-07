package apps

import (
	"regexp"
	"strings"
)

func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var builder strings.Builder
	builder.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					builder.WriteString("(?:.*/)?")
					i += 2
				} else {
					builder.WriteString(".*")
					i++
				}
			} else {
				builder.WriteString("[^/]*")
			}
		case '?':
			builder.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			builder.WriteByte('\\')
			builder.WriteByte(pattern[i])
		default:
			builder.WriteByte(pattern[i])
		}
	}
	builder.WriteString("$")
	return regexp.Compile(builder.String())
}
