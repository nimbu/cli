package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// forceCommandTypes returns the command structs whose Run method directly
// depends on --force: it calls requireForce or an audited delete helper, or
// reads a .Force field.
func forceCommandTypes(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	out := map[string]bool{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Run" || fn.Body == nil {
				continue
			}
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			ident, ok := recv.(*ast.Ident)
			if !ok {
				continue
			}
			assigned := map[ast.Expr]bool{}
			uses := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.AssignStmt:
					for _, lhs := range x.Lhs {
						assigned[lhs] = true
					}
				case *ast.CallExpr:
					if id, ok := x.Fun.(*ast.Ident); ok {
						if id.Name == "requireForce" || (strings.HasPrefix(id.Name, "audited") && strings.HasSuffix(id.Name, "Delete")) {
							uses = true
						}
					}
				case *ast.SelectorExpr:
					if x.Sel.Name == "Force" && !assigned[x] {
						uses = true
					}
				}
				return true
			})
			if uses {
				out[ident.Name] = true
			}
		}
	}
	return out
}

// Commands that need --force must say so in their help, so default help
// shows --force under "Safety:" instead of hiding it behind --help-all.
func TestForceCommandsMentionForceInHelp(t *testing.T) {
	types := forceCommandTypes(t)
	if len(types) < 20 {
		t.Fatalf("scan found only %d --force commands; scanner broken?", len(types))
	}
	parser, _, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var missing []string
	_ = kong.Visit(parser.Model.Node, func(v kong.Visitable, next kong.Next) error {
		if node, ok := v.(*kong.Node); ok && node.Type == kong.CommandNode && node.Target.IsValid() {
			name := node.Target.Type().Name()
			if types[name] {
				seen[name] = true
				if !strings.Contains(node.Help+node.Detail, "--force") {
					missing = append(missing, node.FullPath()+" ("+name+")")
				}
			}
		}
		return next(nil)
	})
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("commands using --force without mentioning it in help:\n  %s", strings.Join(missing, "\n  "))
	}
	for name := range types {
		if !seen[name] {
			t.Errorf("%s uses --force but is not reachable in the command tree", name)
		}
	}
}
