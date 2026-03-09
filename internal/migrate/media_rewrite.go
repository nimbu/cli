package migrate

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

var mediaURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

// MediaRewritePlan tracks source upload URLs and rewrites matching strings to target URLs.
type MediaRewritePlan struct {
	replacements map[string]string
	sourceHosts  map[string]struct{}
	sourcePaths  map[string]struct{}
	sourceDirs   map[string]struct{}
	warnings     []string
	warned       map[string]struct{}
}

// NewMediaRewritePlan creates an empty media rewrite plan.
func NewMediaRewritePlan() *MediaRewritePlan {
	return &MediaRewritePlan{
		replacements: map[string]string{},
		sourceHosts:  map[string]struct{}{},
		sourcePaths:  map[string]struct{}{},
		sourceDirs:   map[string]struct{}{},
		warnings:     []string{},
		warned:       map[string]struct{}{},
	}
}

// Add registers one source upload URL to target URL rewrite.
func (p *MediaRewritePlan) Add(sourceURL, targetURL string) {
	if p == nil {
		return
	}
	sourceURL = strings.TrimSpace(sourceURL)
	targetURL = strings.TrimSpace(targetURL)
	if sourceURL == "" || targetURL == "" {
		return
	}
	p.replacements[sourceURL] = targetURL
	if normalized := normalizeMediaURL(sourceURL); normalized != "" {
		p.replacements[normalized] = targetURL
	}
	p.trackSourceURL(sourceURL)
}

// RewriteValue rewrites matching URLs inside nested map/slice/string payloads in place.
func (p *MediaRewritePlan) RewriteValue(path string, value any) any {
	if p == nil {
		return value
	}
	switch typed := value.(type) {
	case api.PageDocument:
		for key, child := range typed {
			if key == "attachment" || key == "attachment_path" {
				continue
			}
			typed[key] = p.RewriteValue(joinRewritePath(path, key), child)
		}
		return typed
	case api.MenuDocument:
		for key, child := range typed {
			if key == "attachment" || key == "attachment_path" {
				continue
			}
			typed[key] = p.RewriteValue(joinRewritePath(path, key), child)
		}
		return typed
	case map[string]any:
		for key, child := range typed {
			if key == "attachment" || key == "attachment_path" {
				continue
			}
			typed[key] = p.RewriteValue(joinRewritePath(path, key), child)
		}
		return typed
	case []any:
		for idx, child := range typed {
			typed[idx] = p.RewriteValue(joinRewritePath(path, "["+itoa(idx)+"]"), child)
		}
		return typed
	case string:
		return p.RewriteString(path, typed)
	default:
		return value
	}
}

// RewriteString rewrites known source upload URLs inside one string.
func (p *MediaRewritePlan) RewriteString(path, value string) string {
	if p == nil || value == "" {
		return value
	}
	matches := mediaURLPattern.FindAllStringIndex(value, -1)
	if len(matches) == 0 {
		return value
	}

	var out strings.Builder
	last := 0
	for _, match := range matches {
		start, end := match[0], match[1]
		raw := value[start:end]
		candidate, suffix, replacement, ok := p.lookupWithTrim(raw)

		out.WriteString(value[last:start])
		if ok {
			out.WriteString(replacement)
			out.WriteString(suffix)
		} else {
			out.WriteString(raw)
			p.warnIfPotentialSourceURL(path, candidate)
		}
		last = end
	}
	out.WriteString(value[last:])
	return out.String()
}

// Warnings returns deduplicated rewrite warnings collected so far.
func (p *MediaRewritePlan) Warnings() []string {
	if p == nil {
		return nil
	}
	out := make([]string, len(p.warnings))
	copy(out, p.warnings)
	return out
}

func (p *MediaRewritePlan) lookup(rawURL string) (string, bool) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", false
	}
	if replacement, ok := p.replacements[rawURL]; ok {
		return replacement, true
	}
	normalized := normalizeMediaURL(rawURL)
	replacement, ok := p.replacements[normalized]
	return replacement, ok
}

func (p *MediaRewritePlan) lookupWithTrim(rawURL string) (string, string, string, bool) {
	if replacement, ok := p.lookup(rawURL); ok {
		return rawURL, "", replacement, true
	}
	candidate, suffix := trimTrailingURLPunctuation(rawURL)
	if candidate == rawURL {
		return rawURL, "", "", false
	}
	replacement, ok := p.lookup(candidate)
	return candidate, suffix, replacement, ok
}

func (p *MediaRewritePlan) warnIfPotentialSourceURL(path, rawURL string) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return
	}
	if _, ok := p.sourceHosts[strings.ToLower(parsed.Host)]; !ok {
		return
	}
	cleanPath := normalizeSourcePath(parsed.Path)
	if _, ok := p.sourcePaths[cleanPath]; !ok {
		if !p.matchesSourceDir(cleanPath) {
			return
		}
	}
	key := path + "|" + rawURL
	if _, ok := p.warned[key]; ok {
		return
	}
	p.warned[key] = struct{}{}
	location := path
	if location == "" {
		location = "<root>"
	}
	p.warnings = append(p.warnings, "unresolved media URL at "+location+": "+rawURL)
}

func (p *MediaRewritePlan) trackSourceURL(rawURL string) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return
	}
	if parsed.Host != "" {
		p.sourceHosts[strings.ToLower(parsed.Host)] = struct{}{}
	}
	cleanPath := normalizeSourcePath(parsed.Path)
	if cleanPath == "" {
		return
	}
	p.sourcePaths[cleanPath] = struct{}{}
	if dir := mediaDirPrefix(cleanPath); dir != "" {
		p.sourceDirs[dir] = struct{}{}
	}
}

func (p *MediaRewritePlan) matchesSourceDir(cleanPath string) bool {
	for dir := range p.sourceDirs {
		if strings.HasPrefix(cleanPath, dir) {
			return true
		}
	}
	return false
}

func normalizeMediaURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func trimTrailingURLPunctuation(rawURL string) (string, string) {
	end := len(rawURL)
	for end > 0 {
		switch rawURL[end-1] {
		case '.', ',', ';':
			end--
		case ')':
			if strings.Count(rawURL[:end], ")") <= strings.Count(rawURL[:end], "(") {
				return rawURL[:end], rawURL[end:]
			}
			end--
		default:
			return rawURL[:end], rawURL[end:]
		}
	}
	return rawURL[:end], rawURL[end:]
}

func normalizeSourcePath(rawPath string) string {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return ""
	}
	if !strings.HasPrefix(rawPath, "/") {
		rawPath = "/" + rawPath
	}
	return rawPath
}

func mediaDirPrefix(cleanPath string) string {
	idx := strings.LastIndex(cleanPath, "/")
	if idx <= 0 {
		return ""
	}
	return cleanPath[:idx+1]
}

func joinRewritePath(parent, child string) string {
	if parent == "" {
		return child
	}
	if strings.HasPrefix(child, "[") {
		return parent + child
	}
	return parent + "." + child
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	pos := len(digits)
	for value > 0 {
		pos--
		digits[pos] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[pos:])
}
