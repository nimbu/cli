package apps

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// entrypoint is the file the cloud code runtime evaluates; other files only
// run when main.js (transitively) requires them.
const entrypoint = "main.js"

var (
	requirePattern = regexp.MustCompile("\\brequire\\s*\\(\\s*(?:\"([^\"]+)\"|'([^']+)'|`([^`$]+)`)\\s*\\)")
	importPattern  = regexp.MustCompile(`(?m)(?:^|[\n;])\s*import\s*(?:[\s\w{},*]+from\s*)?["']([^"']+)["']`)
	exportPattern  = regexp.MustCompile(`(?m)(?:^|[\n;])\s*export[\s\w{},*]*from\s*["']([^"']+)["']`)

	// Mirrors Nimbu::CloudCode::Loader on the server: bare requires of these
	// names load a built-in module instead of a cloud code file.
	builtinModulePattern = regexp.MustCompile(`^(?:` +
		`ejs|ejs/filters|ejs/utils|handlebars|moment|lodash|lodash/core|path|underscore|util|` +
		`atob|azure/storage|btoa|crypto|csv|fs|gc|googleauth|html|http|i18n|imgkit|jwt|mail|mollie|` +
		`oauth2|order_helpers|pdf|s3|signer|site_variables|soap|stripe|xml|zip` +
		`)(?:\.js)?$`)
	ignoredModulePattern = regexp.MustCompile(`^(?:xmlhttprequest|localstorage)`)
)

// Graph is the local require graph of an app's cloud code, keyed by remote name.
type Graph struct {
	app   AppConfig
	files map[string]string   // remote name -> project-relative path
	deps  map[string][]string // remote name -> remote names of local files it requires
	// accepts holds, per file and dependency, the remote names the server
	// would accept for every require of that dependency.
	accepts map[string]map[string][]string
}

// MissingDependency is a file that requires a local file which is neither
// part of the push nor present on the server.
type MissingDependency struct {
	Name     string
	Requires string
}

// LoadGraph reads files from disk and builds their require graph.
func LoadGraph(app AppConfig, files []string) (*Graph, error) {
	sources := make(map[string]string, len(files))
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(app.ProjectRoot, filepath.FromSlash(file)))
		if err != nil {
			return nil, err
		}
		sources[file] = string(data)
	}
	return NewGraph(app, sources), nil
}

// NewGraph builds the require graph from project-relative paths and their source.
func NewGraph(app AppConfig, sources map[string]string) *Graph {
	g := &Graph{
		app:     app,
		files:   make(map[string]string, len(sources)),
		deps:    make(map[string][]string, len(sources)),
		accepts: make(map[string]map[string][]string, len(sources)),
	}
	for file := range sources {
		g.files[RemoteName(app, file)] = file
	}
	for file, source := range sources {
		name := RemoteName(app, file)
		g.deps[name], g.accepts[name] = g.dependencyNames(name, source)
	}
	return g
}

// Order returns files in upload order: every file after the local files it
// requires, with main.js last. The runtime tolerates require cycles, so they
// are broken (walking from main.js first) and returned instead of rejected.
// Files unknown to the graph are kept, without dependencies.
func (g *Graph) Order(files []string) ([]string, [][]string) {
	selected := g.selectNames(files)

	visiting := map[string]bool{}
	visited := map[string]bool{}
	var stack, order []string
	var cycles [][]string

	var visit func(string)
	visit = func(name string) {
		visiting[name] = true
		stack = append(stack, name)
		for _, dep := range g.deps[name] {
			if _, ok := selected[dep]; !ok || visited[dep] {
				continue
			}
			if visiting[dep] {
				start := len(stack) - 1
				for stack[start] != dep {
					start--
				}
				cycles = append(cycles, append(append([]string{}, stack[start:]...), dep))
				continue
			}
			visit(dep)
		}
		stack = stack[:len(stack)-1]
		visiting[name] = false
		visited[name] = true
		order = append(order, name)
	}

	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)
	if _, ok := selected[entrypoint]; ok {
		visit(entrypoint)
	}
	for _, name := range names {
		if !visited[name] {
			visit(name)
		}
	}

	// Everything main.js reaches already precedes it; whatever follows is
	// unreachable from it, so moving main.js last is safe and keeps the
	// entrypoint from compiling before files a dynamic require() might need.
	ordered := make([]string, 0, len(order))
	for _, name := range order {
		if name != entrypoint {
			ordered = append(ordered, selected[name])
		}
	}
	if file, ok := selected[entrypoint]; ok {
		ordered = append(ordered, file)
	}
	return ordered, cycles
}

// Missing lists selected files that require a local file which is not being
// pushed and that the server cannot resolve from its current files either;
// uploading them would fail with UnknownModule.
func (g *Graph) Missing(files []string, remote []api.AppCodeFile) []MissingDependency {
	selected := g.selectNames(files)
	present := make(map[string]bool, len(remote))
	for _, file := range remote {
		present[file.Name] = true
	}
	// An extensionless require is satisfied by any remote file the server
	// would try (.js, .coffee, .ejs); an explicit .ejs only by itself.
	resolvable := func(name, dep string) bool {
		for _, candidate := range g.accepts[name][dep] {
			if present[candidate] {
				return true
			}
		}
		return false
	}
	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)

	var missing []MissingDependency
	for _, name := range names {
		for _, dep := range g.deps[name] {
			if _, ok := selected[dep]; !ok && !resolvable(name, dep) {
				missing = append(missing, MissingDependency{Name: name, Requires: dep})
			}
		}
	}
	return missing
}

// WarnCycles reports require cycles found while ordering an upload.
func WarnCycles(ctx context.Context, cycles [][]string) error {
	for _, cycle := range cycles {
		last := cycle[len(cycle)-2]
		msg := fmt.Sprintf("require cycle %s; uploading %s before %s, which it requires", strings.Join(cycle, " -> "), last, cycle[0])
		if err := output.Warn(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// selectNames maps the remote name of each file to its project-relative path.
func (g *Graph) selectNames(files []string) map[string]string {
	selected := make(map[string]string, len(files))
	for _, file := range files {
		selected[RemoteName(g.app, file)] = file
	}
	return selected
}

func (g *Graph) dependencyNames(name, source string) ([]string, map[string][]string) {
	source = codeSkeleton(source)
	found := map[string][]string{}
	for _, pattern := range []*regexp.Regexp{requirePattern, importPattern, exportPattern} {
		for _, match := range pattern.FindAllStringSubmatch(source, -1) {
			for _, spec := range match[1:] {
				if spec == "" {
					continue
				}
				dep, candidates, ok := g.resolve(name, spec)
				if !ok || dep == name {
					continue
				}
				if previous, seen := found[dep]; seen {
					candidates = intersect(previous, candidates)
				}
				found[dep] = candidates
			}
		}
	}
	deps := make([]string, 0, len(found))
	for dep := range found {
		deps = append(deps, dep)
	}
	sort.Strings(deps)
	return deps, found
}

func intersect(a, b []string) []string {
	var both []string
	for _, item := range a {
		if slices.Contains(b, item) {
			both = append(both, item)
		}
	}
	return both
}

// resolve maps a require() argument to a local remote name the way the
// server does: ids are lowercased, "./" and "../" resolve against the
// requiring file, bare ids resolve from the app root (unless they name a
// built-in module), and extensionless ids try .js, .coffee, then .ejs.
// It also returns every remote name the server would try for spec.
func (g *Graph) resolve(currentName, spec string) (string, []string, bool) {
	spec = strings.ToLower(strings.TrimSpace(spec))
	if spec == "" {
		return "", nil, false
	}
	id := spec
	if strings.HasPrefix(spec, ".") {
		id = path.Join(path.Dir(currentName), spec)
	} else if builtinModulePattern.MatchString(spec) || ignoredModulePattern.MatchString(spec) {
		return "", nil, false
	}
	if strings.HasSuffix(id, ".js") {
		id = strings.TrimSuffix(id, ".js")
	} else {
		id = strings.TrimSuffix(id, ".coffee")
	}
	if id == "" || id == "." || id == ".." || strings.HasPrefix(id, "../") {
		return "", nil, false
	}

	candidates := []string{id + ".js", id + ".coffee", id + ".ejs"}
	switch path.Ext(id) {
	case ".js", ".coffee", ".ejs":
		candidates = []string{id}
	}
	for _, candidate := range candidates {
		if _, ok := g.files[candidate]; ok {
			return candidate, candidates, true
		}
	}
	return "", nil, false
}
