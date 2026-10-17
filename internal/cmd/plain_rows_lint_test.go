package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Regression: --plain rows written with Fprintf("%s\t%s\n", ...) skip
// output.WritePlain's escaping, so a tab or newline inside a value (an entry
// title, a role name, a theme path) splits the record. Every tab-separated
// line in this package must go through output.Plain/PlainRows/WritePlain.
func TestNoHandWrittenTSVRows(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var offenders []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			// Sprint* too: a row built with Sprintf("%s\t%s") and printed
			// later (diff lines) bypasses the escaper just the same.
			if !ok || (!strings.HasPrefix(sel.Sel.Name, "Fprint") && !strings.HasPrefix(sel.Sel.Name, "Sprint")) {
				return true
			}
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err == nil && strings.Contains(value, "\t") {
					offenders = append(offenders, fset.Position(call.Pos()).String())
				}
			}
			return true
		})
	}
	if len(offenders) > 0 {
		t.Errorf("tab-separated output written by hand; use output.Plain/PlainRows/WritePlain:\n  %s", strings.Join(offenders, "\n  "))
	}
}
