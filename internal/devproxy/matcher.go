package devproxy

import (
	pathpkg "path"
	"strings"
)

type routeRule struct {
	method  string
	pattern string
}

// Matcher decides whether a request should be handled by the simulator proxy.
type Matcher struct {
	exclude []routeRule
	include []routeRule
}

// NewMatcher builds a matcher from route rule strings.
func NewMatcher(include []string, exclude []string) Matcher {
	matcher := Matcher{}
	matcher.include = parseRules(include)
	matcher.exclude = parseRules(exclude)
	return matcher
}

// ShouldProxy returns true when the request should be sent to simulator/render.
func (m Matcher) ShouldProxy(method string, path string, isWebsocket bool) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "GET"
	}

	normalized := normalizePath(path)

	if matchesRules(m.include, method, normalized) {
		return true
	}
	if matchesRules(m.exclude, method, normalized) {
		return false
	}

	return defaultShouldProxy(normalized, isWebsocket)
}

func defaultShouldProxy(path string, isWebsocket bool) bool {
	if isWebsocket {
		return false
	}

	if isStaticPath(path) {
		return false
	}

	if strings.Contains(path, ".hot-update.") {
		return false
	}
	if strings.HasPrefix(path, "/__webpack") || strings.HasPrefix(path, "/sockjs-node/") {
		return false
	}
	if strings.HasPrefix(path, "/__") || path == "/ws" {
		return false
	}
	if strings.Contains(path, "webpack") && (strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".js")) {
		return false
	}

	return true
}

func isStaticPath(path string) bool {
	prefixes := []string{
		"/images/",
		"/fonts/",
		"/javascripts/",
		"/stylesheets/",
		"/css/",
		"/js/",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func parseRules(raw []string) []routeRule {
	rules := make([]routeRule, 0, len(raw))
	for _, value := range raw {
		method, pattern := parseRule(value)
		if pattern == "" {
			continue
		}
		rules = append(rules, routeRule{method: method, pattern: pattern})
	}
	return rules
}

func parseRule(raw string) (method string, pattern string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}

	parts := strings.Fields(raw)
	if len(parts) == 1 {
		return "", normalizePath(parts[0])
	}

	candidate := strings.ToUpper(parts[0])
	switch candidate {
	case "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT":
		return candidate, normalizePath(strings.Join(parts[1:], " "))
	default:
		return "", normalizePath(raw)
	}
}

func matchesRules(rules []routeRule, method string, path string) bool {
	for _, rule := range rules {
		if rule.method != "" && rule.method != method {
			continue
		}
		if matchPath(rule.pattern, path) {
			return true
		}
	}
	return false
}

func matchPath(pattern string, path string) bool {
	pattern = normalizePath(pattern)
	path = normalizePath(path)

	if pattern == path {
		return true
	}

	if strings.HasSuffix(pattern, "/**") {
		base := strings.TrimSuffix(pattern, "/**")
		if base == "" || base == "/" {
			return true
		}
		if path == base {
			return true
		}
		return strings.HasPrefix(path, base+"/")
	}

	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}

	trimmedPattern := strings.TrimPrefix(pattern, "/")
	trimmedPath := strings.TrimPrefix(path, "/")
	ok, err := pathpkg.Match(trimmedPattern, trimmedPath)
	if err != nil {
		return false
	}
	return ok
}

func normalizePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "/"
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}
