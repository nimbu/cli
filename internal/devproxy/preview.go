package devproxy

import (
	"net/http"
	"strings"
)

// PreviewInjector adds a draft preview token to matching local requests so
// the simulator sees ?preview=<token> even when the browser uses the normal URL.
type PreviewInjector struct {
	tokens map[string]string
}

// NewPreviewInjector maps page fullpaths (with a leading slash) to preview tokens.
func NewPreviewInjector(tokens map[string]string) *PreviewInjector {
	normalized := make(map[string]string, len(tokens))
	for path, token := range tokens {
		path = normalizePreviewPath(path)
		token = strings.TrimSpace(token)
		if path == "" || token == "" {
			continue
		}
		normalized[path] = token
	}
	return &PreviewInjector{tokens: normalized}
}

// Apply injects preview=<token> when the request path is exactly a registered
// page fullpath, a translation fullpath, or a locale-prefixed default fullpath.
// Child paths such as /page/child are not matched.
func (p *PreviewInjector) Apply(req *http.Request) {
	if p == nil || req == nil || req.URL == nil || len(p.tokens) == 0 {
		return
	}
	if req.URL.Query().Get("preview") != "" {
		return
	}
	token := p.tokenForPath(requestPath(req))
	if token == "" {
		return
	}
	query := req.URL.Query()
	query.Set("preview", token)
	req.URL.RawQuery = query.Encode()
}

func (p *PreviewInjector) tokenForPath(path string) string {
	path = normalizePreviewPath(path)
	if token := p.tokens[path]; token != "" {
		return token
	}
	if rest, ok := stripLocalePrefix(path); ok {
		return p.tokens[rest]
	}
	return ""
}

func normalizePreviewPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

func stripLocalePrefix(path string) (string, bool) {
	trimmed := strings.TrimPrefix(path, "/")
	slash := strings.IndexByte(trimmed, '/')
	if slash <= 0 {
		return "", false
	}
	prefix := trimmed[:slash]
	if !isLocalePrefix(prefix) {
		return "", false
	}
	return "/" + trimmed[slash+1:], true
}

func isLocalePrefix(value string) bool {
	switch len(value) {
	case 2:
		return isAlpha(value)
	case 5:
		return isAlpha(value[:2]) && value[2] == '-' && isAlpha(value[3:])
	default:
		return false
	}
}

func isAlpha(value string) bool {
	for _, r := range value {
		if r < 'A' || (r > 'Z' && r < 'a') || r > 'z' {
			return false
		}
	}
	return value != ""
}
