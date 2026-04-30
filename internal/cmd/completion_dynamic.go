package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/config"
)

const (
	completionCacheTTL         = 5 * time.Minute
	completionTimeout          = 2 * time.Second
	completionDebugLogMaxBytes = 1 << 20
)

type completionKind string

const (
	completionKindSite       completionKind = "site"
	completionKindChannel    completionKind = "channel"
	completionKindChannelRef completionKind = "channel-ref"
	completionKindTheme      completionKind = "theme"
	completionKindThemeRef   completionKind = "theme-ref"
)

type InternalCompleteCmd struct {
	Shell        string   `help:"Shell requesting completions" default:"bash"`
	Current      string   `help:"Current token being completed"`
	FlagNames    bool     `help:"Complete flag names" hidden:""`
	CommandNames bool     `help:"Complete command names" hidden:""`
	Words        []string `arg:"" optional:"" passthrough:""`
}

type completionRequest struct {
	Shell       string
	Current     string
	Words       []string
	CommandPath []string
	Flag        string
	Prefix      string
	Assignment  bool
}

type completionItem struct {
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

func (c *InternalCompleteCmd) Run(ctx context.Context) error {
	if c.FlagNames || c.CommandNames {
		parser, _, err := newParser()
		if err != nil {
			completionDebugf("create parser for completion: %v", err)
			return nil
		}
		prefix := c.Current
		if c.FlagNames && prefix == "" {
			prefix = "--"
		}
		req := completionRequest{
			Shell:       c.Shell,
			Current:     c.Current,
			Words:       c.Words,
			CommandPath: completionCommandPath(c.Words),
			Prefix:      prefix,
		}
		var items []completionItem
		if c.CommandNames {
			items = completeCommandNames(parser, req)
		} else {
			items = completeFlagNames(parser, req)
		}
		printCompletionItems(c.Shell, items)
		return nil
	}

	req, ok := resolveCompletionRequest(c.Words, c.Current)
	if !ok {
		return nil
	}
	req.Shell = c.Shell
	req.Current = c.Current
	req.Words = c.Words

	items, err := completeDynamic(ctx, req)
	if err != nil {
		completionDebugf("dynamic completion failed: %v", err)
		return nil
	}
	printCompletionItems(c.Shell, items)
	return nil
}

func printCompletionItems(shell string, items []completionItem) {
	for _, item := range items {
		switch shell {
		case "zsh", "fish":
			if item.Description != "" {
				fmt.Printf("%s\t%s\n", item.Value, item.Description)
				continue
			}
		}
		fmt.Println(item.Value)
	}
}

func resolveCompletionRequest(words []string, current string) (completionRequest, bool) {
	req := completionRequest{Current: current}

	if flag, prefix, ok := splitFlagAssignment(current); ok {
		req.Flag = flag
		req.Prefix = prefix
		req.Assignment = true
		req.CommandPath = completionCommandPath(words)
		return req, req.Flag != ""
	}

	if len(words) > 0 {
		prev := words[len(words)-1]
		if flag, ok := splitLongFlag(prev); ok {
			req.Flag = flag
			req.Prefix = current
			req.CommandPath = completionCommandPath(words[:len(words)-1])
			return req, req.Flag != ""
		}
	}

	return completionRequest{}, false
}

func splitFlagAssignment(token string) (flag, prefix string, ok bool) {
	if !strings.HasPrefix(token, "--") {
		return "", "", false
	}
	nameValue := strings.TrimPrefix(token, "--")
	idx := strings.IndexByte(nameValue, '=')
	if idx < 0 {
		return "", "", false
	}
	if nameValue[:idx] == "" {
		return "", "", false
	}
	return nameValue[:idx], nameValue[idx+1:], true
}

func splitLongFlag(token string) (string, bool) {
	if !strings.HasPrefix(token, "--") || strings.Contains(token, "=") {
		return "", false
	}
	flag := strings.TrimPrefix(token, "--")
	if flag == "" {
		return "", false
	}
	return flag, true
}

func completionCommandPath(words []string) []string {
	start := 0
	for start < len(words) && words[start] == "--" {
		start++
	}
	if start < len(words) && (words[start] == "nimbu" || words[start] == "nb") {
		start++
	}

	path := make([]string, 0, len(words))
	for i := start; i < len(words); i++ {
		word := words[i]
		if word == "--" {
			continue
		}
		if strings.HasPrefix(word, "-") {
			if completionFlagConsumesValue(word) && !strings.Contains(word, "=") && i+1 < len(words) && !strings.HasPrefix(words[i+1], "-") {
				i++
			}
			continue
		}
		path = append(path, word)
	}
	return path
}

func completionFlagConsumesValue(word string) bool {
	flag := strings.TrimLeft(word, "-")
	return completionValueFlags[flag]
}

func completeFlagNames(parser *kong.Kong, req completionRequest) []completionItem {
	if parser == nil || parser.Model == nil || parser.Model.Node == nil {
		return nil
	}

	prefix := req.Prefix
	if prefix == "" {
		prefix = "--"
	}
	node := completionNodeForPath(parser.Model.Node, req.CommandPath)
	if node == nil {
		return nil
	}

	seen := map[string]struct{}{}
	var items []completionItem
	for _, group := range node.AllFlags(true) {
		for _, flag := range group {
			if flag == nil || flag.Hidden || flag.Name == "" {
				continue
			}
			value := "--" + flag.Name
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			items = append(items, completionItem{
				Value:       value,
				Description: strings.TrimSpace(flag.Help),
			})
		}
	}

	sortCompletionItems(items)
	return filterCompletionItems(items, prefix)
}

func completeCommandNames(parser *kong.Kong, req completionRequest) []completionItem {
	if parser == nil || parser.Model == nil || parser.Model.Node == nil {
		return nil
	}
	if strings.HasPrefix(req.Current, "-") {
		return nil
	}
	if len(req.Words) > 0 && strings.HasPrefix(req.Words[len(req.Words)-1], "-") && completionFlagConsumesValue(req.Words[len(req.Words)-1]) {
		return nil
	}
	node := completionNodeForPathExact(parser.Model.Node, req.CommandPath)
	if node == nil || len(node.Children) == 0 {
		return nil
	}

	var items []completionItem
	for _, child := range node.Children {
		if child.Hidden || child.Name == "" {
			continue
		}
		items = append(items, completionItem{
			Value:       child.Name,
			Description: strings.TrimSpace(child.Help),
		})
		for _, alias := range child.Aliases {
			if alias == "" {
				continue
			}
			items = append(items, completionItem{
				Value:       alias,
				Description: strings.TrimSpace(child.Help),
			})
		}
	}
	sortCompletionItems(items)
	return filterCompletionItems(items, req.Prefix)
}

func completionNodeForPath(root *kong.Node, path []string) *kong.Node {
	node := root
	for _, part := range path {
		if part == "" {
			continue
		}
		var next *kong.Node
		for _, child := range node.Children {
			if child.Hidden {
				continue
			}
			if child.Name == part || completionStringInSlice(child.Aliases, part) {
				next = child
				break
			}
		}
		if next == nil {
			return node
		}
		node = next
	}
	return node
}

func completionNodeForPathExact(root *kong.Node, path []string) *kong.Node {
	node := root
	for _, part := range path {
		if part == "" {
			continue
		}
		var next *kong.Node
		for _, child := range node.Children {
			if child.Hidden {
				continue
			}
			if child.Name == part || completionStringInSlice(child.Aliases, part) {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		node = next
	}
	return node
}

func completionStringInSlice(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

var completionValueFlags = map[string]bool{
	"apiurl":              true,
	"app":                 true,
	"arg":                 true,
	"backend":             true,
	"blog":                true,
	"branch":              true,
	"channel":             true,
	"collection":          true,
	"cmd":                 true,
	"color":               true,
	"code":                true,
	"confirm":             true,
	"content":             true,
	"content-type":        true,
	"coupon":              true,
	"current":             true,
	"customer":            true,
	"cwd":                 true,
	"data":                true,
	"dir":                 true,
	"domain":              true,
	"download-assets":     true,
	"email":               true,
	"enable-commands":     true,
	"entry":               true,
	"entry-channels":      true,
	"expires-in":          true,
	"field":               true,
	"fields":              true,
	"filters":             true,
	"file":                true,
	"function":            true,
	"from":                true,
	"from-host":           true,
	"id":                  true,
	"include":             true,
	"job":                 true,
	"key":                 true,
	"locale":              true,
	"max-body-mb":         true,
	"menu":                true,
	"method":              true,
	"name":                true,
	"notification":        true,
	"only":                true,
	"order":               true,
	"output":              true,
	"output-dir":          true,
	"page":                true,
	"password":            true,
	"password-length":     true,
	"path":                true,
	"per-page":            true,
	"post":                true,
	"product":             true,
	"proxy-host":          true,
	"proxy-port":          true,
	"query":               true,
	"ready-timeout":       true,
	"ready-url":           true,
	"reason":              true,
	"redirect":            true,
	"repo":                true,
	"role":                true,
	"sender":              true,
	"since":               true,
	"shell":               true,
	"site":                true,
	"sort":                true,
	"source":              true,
	"status":              true,
	"template-root":       true,
	"theme":               true,
	"timeout":             true,
	"token":               true,
	"to":                  true,
	"to-host":             true,
	"upsert":              true,
	"value":               true,
	"watch-scan-interval": true,
	"where":               true,
}

func completeDynamic(ctx context.Context, req completionRequest) ([]completionItem, error) {
	kind, ok := newCompletionRegistry().Lookup(req.CommandPath, req.Flag)
	if !ok {
		return nil, nil
	}

	credential, err := ResolveAuthCredential(ctx)
	if err != nil {
		completionDebugf("resolve auth: %v", err)
		return nil, nil
	}
	token := strings.TrimSpace(credential.Token)
	if token == "" {
		completionDebugf("resolve auth: empty token")
		return nil, nil
	}

	flags, _ := ctx.Value(rootFlagsKey{}).(*RootFlags)
	apiURL := config.Defaults().APIURL
	timeout := 30 * time.Second
	debug := false
	if flags != nil {
		if strings.TrimSpace(flags.APIURL) != "" {
			apiURL = flags.APIURL
		}
		if flags.Timeout > 0 {
			timeout = flags.Timeout
		}
		debug = flags.Debug
	}
	if value, ok := completionOriginalFlagValue(req.Words, "apiurl"); ok {
		apiURL = value
	}

	currentSite := completionSiteFromContext(ctx, req.Words)
	scope := completionScope(kind, req.Prefix, currentSite)
	cacheKey := completionCacheKey(apiURL, token, kind, scope)
	cache := readCompletionCache()
	if entry, ok := cache.Entries[cacheKey]; ok && time.Since(entry.FetchedAt) <= completionCacheTTL {
		return filterCompletionItems(entry.Items, req.Prefix), nil
	}

	liveCtx, cancel := context.WithTimeout(ctx, completionTimeout)
	defer cancel()
	client := api.New(apiURL, token).WithTimeout(minDuration(timeout, completionTimeout)).WithDebug(debug).WithVersion(version)

	items, err := fetchCompletionItems(liveCtx, client, kind, req.Prefix, currentSite)
	if err == nil {
		cache.set(cacheKey, items)
		if err := writeCompletionCache(cache); err != nil {
			completionDebugf("write cache: %v", err)
		}
		return filterCompletionItems(items, req.Prefix), nil
	}
	completionDebugf("fetch %s completions: %v", kind, err)

	if entry, ok := cache.Entries[cacheKey]; ok {
		return filterCompletionItems(entry.Items, req.Prefix), nil
	}
	return nil, nil
}

func completionScope(kind completionKind, prefix string, currentSite string) string {
	switch kind {
	case completionKindChannel, completionKindTheme:
		return currentSite
	case completionKindChannelRef, completionKindThemeRef:
		if site, _, ok := strings.Cut(prefix, "/"); ok {
			return site
		}
	}
	return ""
}

func completionSiteFromContext(ctx context.Context, words []string) string {
	if site, ok := completionOriginalFlagValue(words, "site"); ok {
		return site
	}
	if flags, ok := ctx.Value(rootFlagsKey{}).(*RootFlags); ok && flags != nil && strings.TrimSpace(flags.Site) != "" {
		return flags.Site
	}
	if cfg, ok := ctx.Value(configKey{}).(*config.Config); ok && cfg != nil && strings.TrimSpace(cfg.DefaultSite) != "" {
		return cfg.DefaultSite
	}
	if proj, err := config.ReadProjectConfig(); err == nil && strings.TrimSpace(proj.Site) != "" {
		return proj.Site
	}
	return ""
}

func completionOriginalFlagValue(words []string, flag string) (string, bool) {
	long := "--" + flag
	for i := 0; i < len(words); i++ {
		word := words[i]
		if word == "--" {
			continue
		}
		if name, value, ok := strings.Cut(word, "="); ok && name == long {
			value = strings.TrimSpace(value)
			return value, value != ""
		}
		if word == long && i+1 < len(words) {
			value := strings.TrimSpace(words[i+1])
			if value != "" && !strings.HasPrefix(value, "-") {
				return value, true
			}
		}
	}
	return "", false
}

func completionRefSite(prefix string) (site string, ok bool) {
	site, _, ok = strings.Cut(prefix, "/")
	return site, ok && strings.TrimSpace(site) != ""
}

func filterCompletionItems(items []completionItem, prefix string) []completionItem {
	filtered := make([]completionItem, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if item.Value == "" || !strings.HasPrefix(item.Value, prefix) {
			continue
		}
		if _, ok := seen[item.Value]; ok {
			continue
		}
		seen[item.Value] = struct{}{}
		filtered = append(filtered, item)
	}
	return filtered
}

func completionCacheKey(apiURL, token string, kind completionKind, scope string) string {
	normalizedURL := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if parsed, err := url.Parse(normalizedURL); err == nil && parsed.Host != "" {
		normalizedURL = parsed.Scheme + "://" + parsed.Host
	}
	sum := sha256.Sum256([]byte(token))
	return strings.Join([]string{
		normalizedURL,
		hex.EncodeToString(sum[:]),
		string(kind),
		scope,
	}, "|")
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func completionDebugf(format string, args ...any) {
	value := strings.TrimSpace(os.Getenv("NIMBU_COMPLETION_DEBUG"))
	if value == "" || value == "0" || strings.EqualFold(value, "false") {
		return
	}
	line := fmt.Sprintf("nimbu completion: "+format+"\n", args...)
	_, _ = fmt.Fprint(os.Stderr, line)
	writeCompletionDebugLog(line)
}

func writeCompletionDebugLog(line string) {
	dir, err := config.DataDir()
	if err != nil {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	path := completionDebugLogPath(dir)
	flags := os.O_CREATE | os.O_APPEND | os.O_WRONLY
	if info, err := os.Stat(path); err == nil && info.Size() > completionDebugLogMaxBytes {
		flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return
	}
	defer func() {
		_ = file.Close()
	}()
	_, _ = fmt.Fprintf(file, "%s %s", time.Now().Format(time.RFC3339), line)
}

func completionDebugLogPath(dir string) string {
	return filepath.Join(dir, "completion-debug.log")
}
