package apps

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func testApp(root string) AppConfig {
	return AppConfig{
		AppProjectConfig: config.AppProjectConfig{Dir: "code"},
		ProjectRoot:      root,
	}
}

func filesOf(sources map[string]string) []string {
	files := make([]string, 0, len(sources))
	for file := range sources {
		files = append(files, file)
	}
	return files
}

func TestLoadGraphOrdersRequireAndImport(t *testing.T) {
	root := t.TempDir()
	app := testApp(root)
	for path, content := range map[string]string{
		"code/index.js":      `const util = require("./util")`,
		"code/util.js":       `import helper from "./helper"`,
		"code/helper.js":     `export {x} from "./leaf"`,
		"code/leaf.js":       `export const x = 1`,
		"code/standalone.js": `console.log("x")`,
	} {
		abs := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	files := []string{"code/index.js", "code/util.js", "code/helper.js", "code/leaf.js", "code/standalone.js"}
	graph, err := LoadGraph(app, files)
	if err != nil {
		t.Fatalf("load graph: %v", err)
	}
	ordered, cycles := graph.Order(files)
	if len(cycles) != 0 {
		t.Fatalf("unexpected cycles: %v", cycles)
	}
	got := strings.Join(ordered, ",")
	if !strings.Contains(got, "code/leaf.js,code/helper.js,code/util.js,code/index.js") {
		t.Fatalf("unexpected order: %s", got)
	}
}

// Regression: bare requires resolve from the app root on the server, so
// modules sorting after main.js must still be uploaded before it.
func TestOrderHandlesBareRootRelativeRequires(t *testing.T) {
	sources := map[string]string{
		"code/main.js":               "require('services/mailer');\nrequire(\"utils\");\nconst _ = require('lodash');",
		"code/services/mailer.js":    "const t = require('./templates');\nconst cfg = require('../utils.js');",
		"code/services/templates.js": "module.exports = {};",
		"code/utils.js":              "module.exports = {};",
		"code/lodash.js":             "// shadowed by the built-in module",
	}
	graph := NewGraph(testApp(""), sources)
	ordered, cycles := graph.Order(filesOf(sources))
	if len(cycles) != 0 {
		t.Fatalf("unexpected cycles: %v", cycles)
	}
	want := []string{"code/services/templates.js", "code/utils.js", "code/services/mailer.js", "code/lodash.js", "code/main.js"}
	if !reflect.DeepEqual(ordered, want) {
		t.Fatalf("order = %v, want %v", ordered, want)
	}
	if deps := graph.deps["main.js"]; !reflect.DeepEqual(deps, []string{"services/mailer.js", "utils.js"}) {
		t.Fatalf("main.js deps = %v (built-in lodash must not resolve to lodash.js)", deps)
	}
}

func TestOrderResolvesLikeServerLoader(t *testing.T) {
	sources := map[string]string{
		"code/main.js": strings.Join([]string{
			`require( './Models/Entry' )`, // ids are lowercased
			"require(`./model`)",          // .coffee when no .js exists
			`require("./mail.ejs")`,       // explicit extension
			`require("./views/page")`,     // .ejs as last candidate
			`require("/utils")`,           // leading slash never matches a file
			`require("../outside")`,       // above the app root
			`require(name)`,               // dynamic
		}, "\n"),
		"code/models/entry.js": ``,
		"code/model.coffee":    ``,
		"code/mail.ejs":        ``,
		"code/views/page.ejs":  ``,
		"code/utils.js":        ``,
	}
	graph := NewGraph(testApp(""), sources)
	want := []string{"mail.ejs", "model.coffee", "models/entry.js", "views/page.ejs"}
	if got := graph.deps["main.js"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("deps = %v, want %v", got, want)
	}
}

func TestOrderIgnoresCommentedRequires(t *testing.T) {
	sources := map[string]string{
		"code/a.js": strings.Join([]string{
			`// require('./b')`,
			`/* require('./c')`,
			`   still a comment */`,
			`var url = "http://example.com/*"; require('./d') // trailing`,
			`var s = 'it\'s'; require('./e')`,
		}, "\n"),
		"code/b.js": ``,
		"code/c.js": ``,
		"code/d.js": ``,
		"code/e.js": ``,
	}
	graph := NewGraph(testApp(""), sources)
	if got, want := graph.deps["a.js"], []string{"d.js", "e.js"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deps = %v, want %v", got, want)
	}
}

func TestOrderBreaksCyclesFromEntrypoint(t *testing.T) {
	sources := map[string]string{
		"code/main.js": `require('./a')`,
		"code/a.js":    `require('./b')`,
		"code/b.js":    `require('./a')`,
		"code/zz.js":   ``,
	}
	graph := NewGraph(testApp(""), sources)
	ordered, cycles := graph.Order(filesOf(sources))
	want := []string{"code/b.js", "code/a.js", "code/zz.js", "code/main.js"}
	if !reflect.DeepEqual(ordered, want) {
		t.Fatalf("order = %v, want %v", ordered, want)
	}
	if wantCycles := [][]string{{"a.js", "b.js", "a.js"}}; !reflect.DeepEqual(cycles, wantCycles) {
		t.Fatalf("cycles = %v, want %v", cycles, wantCycles)
	}
}

func TestOrderOnlyUsesSelectedFilesAndKeepsUnknownOnes(t *testing.T) {
	sources := map[string]string{
		"code/main.js":  `require('./a')`,
		"code/a.js":     `require('./b')`,
		"code/b.js":     ``,
		"code/other.js": ``,
	}
	graph := NewGraph(testApp(""), sources)
	ordered, _ := graph.Order([]string{"code/main.js", "code/b.js", "code/unknown.js"})
	if want := []string{"code/b.js", "code/unknown.js", "code/main.js"}; !reflect.DeepEqual(ordered, want) {
		t.Fatalf("order = %v, want %v", ordered, want)
	}
}

func TestMissingReportsUnpushedLocalDependencies(t *testing.T) {
	sources := map[string]string{
		"code/main.js":       `require('lib/new'); require('./lib/old'); require('crypto')`,
		"code/lib/new.js":    `require('./deeper')`,
		"code/lib/old.js":    ``,
		"code/lib/deeper.js": ``,
	}
	graph := NewGraph(testApp(""), sources)
	remote := []api.AppCodeFile{{Name: "main.js"}, {Name: "lib/old.js"}}

	got := graph.Missing([]string{"code/main.js"}, remote)
	if want := []MissingDependency{{Name: "main.js", Requires: "lib/new.js"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}
	got = graph.Missing([]string{"code/main.js", "code/lib/new.js"}, remote)
	if want := []MissingDependency{{Name: "lib/new.js", Requires: "lib/deeper.js"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}
	// The server resolves require('lib/new') to a remote lib/new.coffee too.
	if got := graph.Missing([]string{"code/main.js"}, append(remote, api.AppCodeFile{Name: "lib/new.coffee"})); len(got) != 0 {
		t.Fatalf("remote sibling extension should satisfy the require, got %v", got)
	}
	// An explicit .ejs require only accepts that exact file.
	ejs := NewGraph(testApp(""), map[string]string{"code/main.js": `require('./mail.ejs')`, "code/mail.ejs": ``})
	got = ejs.Missing([]string{"code/main.js"}, []api.AppCodeFile{{Name: "mail.js"}})
	if want := []MissingDependency{{Name: "main.js", Requires: "mail.ejs"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}
	if got := graph.Missing(filesOf(sources), nil); len(got) != 0 {
		t.Fatalf("full push should never report missing files, got %v", got)
	}
}

// Regression (codex review): regex literals must not open comments,
// require-like text inside strings must not become a dependency, and
// template ${...} interpolations stay code.
func TestDependencyScanSkipsRegexAndStringContents(t *testing.T) {
	sources := map[string]string{
		"code/a.js": strings.Join([]string{
			`const sep = /[/*]/; require('./b')`,
			`var quote = /'/g; require('./c')`,
			`function f(x) { return /x"/.test(x) } require('./d')`,
			`var half = total / 2; var third = (total) / 3; require('./e') // done`,
			`const doc = "require('./helper')"; const tpl = ` + "`require('./helper2')`",
			`import "./f"; export { g } from './g'`,
			"const label = `${require('./h').name} {${ {k: '}'}.k }} ${`nested ${require('./i')}`}`; require('./j')",
			"const t = `${ /* } */ require('./k') } ${ /}/.test(x) ? require('./l') : 0 }`",
			`if (enabled) /"/.test(value); require('./m')`,
			`var half = count++ / 2; var less = n-- / 2; require('./n')`,
		}, "\n"),
		"code/b.js": ``, "code/c.js": ``, "code/d.js": ``, "code/e.js": ``,
		"code/f.js": ``, "code/g.js": ``, "code/h.js": ``, "code/i.js": ``, "code/j.js": ``,
		"code/k.js": ``, "code/l.js": ``, "code/m.js": ``, "code/n.js": ``,
		"code/helper.js": ``, "code/helper2.js": ``,
	}
	graph := NewGraph(testApp(""), sources)
	if got, want := graph.deps["a.js"], []string{"b.js", "c.js", "d.js", "e.js", "f.js", "g.js", "h.js", "i.js", "j.js", "k.js", "l.js", "m.js", "n.js"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deps = %v, want %v", got, want)
	}
}

func TestCodeSkeletonHandlesUnterminatedInput(t *testing.T) {
	for input, want := range map[string]string{
		`a /* open`:         `a `,
		`a // tail`:         `a `,
		`a "open`:           `a ""`,
		"a 'x\nb // c":      "a ''\nb ",
		"a `multi\nline` b": "a `` b",
		`a "esc\`:           `a ""`,
		"x = /open\ny":      "x = /open\ny",
		`require( "./x" )`:  `require( "./x" )`,
	} {
		if got := codeSkeleton(input); got != want {
			t.Fatalf("codeSkeleton(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWarnCyclesNamesTheBrokenEdge(t *testing.T) {
	stderr := &strings.Builder{}
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: &strings.Builder{}, Err: stderr, NoTTY: true})
	if err := WarnCycles(ctx, [][]string{{"a.js", "b.js", "a.js"}}); err != nil {
		t.Fatalf("warn: %v", err)
	}
	want := "warning: require cycle a.js -> b.js -> a.js; uploading b.js before a.js, which it requires\n"
	if got := stderr.String(); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}
