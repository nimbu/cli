package themesync

import (
	"regexp"
	"strings"
)

type globMatcher struct {
	pattern string
	re      *regexp.Regexp
}

func compileMatchers(patterns []string) ([]globMatcher, error) {
	matchers := make([]globMatcher, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := globToRegexp(pattern)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, globMatcher{pattern: pattern, re: re})
	}
	return matchers, nil
}

func matchesAny(matchers []globMatcher, value string) bool {
	normalized := normalizePath(value)
	for _, matcher := range matchers {
		if matcher.re.MatchString(normalized) {
			return true
		}
	}
	return false
}

func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var builder strings.Builder
	builder.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				builder.WriteString(".*")
				i++
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
