package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// confirmationRoots are the helpers that end in a --force/--yes decision.
// requireForce, confirmPrompt, the audited delete helpers and the protected
// environment check all reach one of them.
var confirmationRoots = map[string]bool{
	"requireForceAction": true,
	"confirmPromptFlag":  true,
	"confirmedByFlag":    true,
}

// confirmationFields are RootFlags fields whose read means the code path
// branches on a confirmation override.
var confirmationFields = map[string]bool{"Force": true, "Yes": true}

type safetyScan struct {
	funcs   map[string]*ast.FuncDecl   // package-level functions by name
	methods map[string][]*ast.FuncDecl // methods by name (any receiver)
	runs    map[string]*ast.FuncDecl   // Run methods by receiver type
	imports map[*ast.FuncDecl]map[string]bool
	memo    map[*ast.FuncDecl]int // 0 unknown, 1 visiting, 2 no, 3 yes
}

func receiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if ident, ok := recv.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func newSafetyScan(t *testing.T) *safetyScan {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	s := &safetyScan{
		funcs:   map[string]*ast.FuncDecl{},
		methods: map[string][]*ast.FuncDecl{},
		runs:    map[string]*ast.FuncDecl{},
		imports: map[*ast.FuncDecl]map[string]bool{},
		memo:    map[*ast.FuncDecl]int{},
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		imported := map[string]bool{}
		for _, spec := range file.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			local := path[strings.LastIndex(path, "/")+1:]
			if spec.Name != nil {
				local = spec.Name.Name
			}
			imported[local] = true
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			s.imports[fn] = imported
			recv := receiverTypeName(fn)
			switch {
			case recv == "":
				s.funcs[fn.Name.Name] = fn
			case fn.Name.Name == "Run":
				s.runs[recv] = fn
			default:
				s.methods[fn.Name.Name] = append(s.methods[fn.Name.Name], fn)
			}
		}
	}
	return s
}

// confirms reports whether fn can reach a confirmation decision, following
// calls to (and references of) functions and methods in this package.
func (s *safetyScan) confirms(fn *ast.FuncDecl) bool {
	switch s.memo[fn] {
	case 1, 2:
		return false
	case 3:
		return true
	}
	s.memo[fn] = 1
	imported := s.imports[fn]
	assigned := map[ast.Expr]bool{}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range x.Lhs {
				assigned[lhs] = true
			}
		case *ast.Ident:
			if confirmationRoots[x.Name] {
				found = true
			} else if callee := s.funcs[x.Name]; callee != nil && s.confirms(callee) {
				found = true
			}
		case *ast.SelectorExpr:
			if pkg, ok := x.X.(*ast.Ident); ok && imported[pkg.Name] {
				return false // another package: not followed
			}
			if confirmationFields[x.Sel.Name] && !assigned[x] {
				found = true
				return false
			}
			if x.Sel.Name == "Run" {
				// (&OtherCmd{...}).Run delegates to that command.
				if typ := compositeTypeName(x.X); typ != "" {
					if run := s.runs[typ]; run != nil && s.confirms(run) {
						found = true
					}
				}
				return true
			}
			for _, callee := range s.methods[x.Sel.Name] {
				if s.confirms(callee) {
					found = true
					break
				}
			}
		}
		return !found
	})
	if found {
		s.memo[fn] = 3
	} else {
		s.memo[fn] = 2
	}
	return found
}

func compositeTypeName(expr ast.Expr) string {
	for {
		switch x := expr.(type) {
		case *ast.ParenExpr:
			expr = x.X
		case *ast.UnaryExpr:
			expr = x.X
		case *ast.CompositeLit:
			if ident, ok := x.Type.(*ast.Ident); ok {
				return ident.Name
			}
			return ""
		default:
			return ""
		}
	}
}

// confirmationCommandTypesFromSource returns the command structs whose Run
// can stop on a confirmation: --force/--yes guards, y/N prompts and protected
// environment checks, reached directly or through helpers.
func confirmationCommandTypesFromSource(t *testing.T) map[string]bool {
	t.Helper()
	s := newSafetyScan(t)
	out := map[string]bool{}
	for typ, run := range s.runs {
		// Reset the in-progress marks so a cycle seen from one Run does not
		// hide a path for another.
		for fn, state := range s.memo {
			if state == 1 {
				delete(s.memo, fn)
			}
		}
		if s.confirms(run) {
			out[typ] = true
		}
	}
	return out
}

// Every command that can stop on a confirmation must be registered in
// confirmationCommands, so its help shows --force/--yes under "Safety:";
// every registered command must still need it.
func TestConfirmationCommandsRegistryMatchesSource(t *testing.T) {
	fromSource := confirmationCommandTypesFromSource(t)
	if len(fromSource) < 30 {
		t.Fatalf("scan found only %d confirmation commands; scanner broken?", len(fromSource))
	}
	registered := map[string]bool{}
	for typ := range confirmationCommands {
		registered[typ.Name()] = true
	}
	var missing, stale []string
	for name := range fromSource {
		if !registered[name] {
			missing = append(missing, name)
		}
	}
	for name := range registered {
		if !fromSource[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("commands that can require confirmation but are not in confirmationCommands (help_safety.go):\n  (*%s)(nil),", strings.Join(missing, ")(nil),\n  (*"))
	}
	if len(stale) > 0 {
		t.Errorf("confirmationCommands entries that no longer reach a confirmation:\n  %s", strings.Join(stale, "\n  "))
	}
}

// The walk proves each registered command is reachable in the CLI tree and
// that its default help renders the Safety section.
func TestConfirmationCommandsShowSafetyInHelp(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[reflect.Type]bool{}
	var noSafety []string
	_ = kong.Visit(parser.Model.Node, func(v kong.Visitable, next kong.Next) error {
		node, ok := v.(*kong.Node)
		if !ok || node.Type != kong.CommandNode || !node.Target.IsValid() {
			return next(nil)
		}
		if confirmationCommands[node.Target.Type()] {
			seen[node.Target.Type()] = true
			sections := flagSections(parser.Model, node, helpRenderOptions{Style: plainHelpStyle()})
			hasForce := false
			for _, section := range sections {
				if section.title != "Safety:" {
					continue
				}
				for _, row := range section.rows {
					if strings.Contains(row.left, "--force") {
						hasForce = true
					}
				}
			}
			if !hasForce {
				noSafety = append(noSafety, node.FullPath())
			}
		} else if needsConfirmation(node) {
			t.Errorf("%s shows Safety but is not registered", node.FullPath())
		}
		return next(nil)
	})
	sort.Strings(noSafety)
	if len(noSafety) > 0 {
		t.Errorf("commands missing --force under Safety:\n  %s", strings.Join(noSafety, "\n  "))
	}
	for typ := range confirmationCommands {
		if !seen[typ] {
			t.Errorf("%s is registered but not reachable in the command tree", typ.Name())
		}
	}
}

// Regression: commands that only reach --force through a prompt (schema
// apply, apps push, sites copy, ...) used to miss the Safety section because
// detection looked for "--force" in their help text.
func TestIndirectConfirmationCommandsAreDetected(t *testing.T) {
	fromSource := confirmationCommandTypesFromSource(t)
	for _, name := range []string{
		"SchemaApplyCmd", "AppsPushCmd", "CustomersConfigCopyCmd", "ProductsConfigCopyCmd",
		"ThemePushCmd", "ThemeSyncCmd", "ReleaseCmd", "RolesCustomersSetCmd",
		"APIDeleteCmd", "MenusCopyCmd", "SitesCopyCmd", "ChannelsDeleteCmd",
	} {
		if !fromSource[name] {
			t.Errorf("%s not detected as a confirmation command", name)
		}
	}
}
