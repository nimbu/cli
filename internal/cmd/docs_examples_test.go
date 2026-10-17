package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docFiles are the user-facing docs whose shell examples must parse.
var docFiles = []string{
	filepath.Join("..", "..", "README.md"),
	filepath.Join("..", "..", ".claude", "skills", "nimbu", "SKILL.md"),
}

// docCommandLines returns the `nimbu ...` lines inside shell code blocks
// (```bash, ```sh, ```shell), with `\` continuations joined.
func docCommandLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	inFence := false // inside a shell block
	opened := false  // inside any block
	pending := ""
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if lang, ok := strings.CutPrefix(line, "```"); ok {
			opened = !opened
			inFence = opened && (lang == "bash" || lang == "sh" || lang == "shell")
			pending = ""
			continue
		}
		if !inFence {
			continue
		}
		if pending != "" {
			line = pending + " " + line
			pending = ""
		}
		if strings.HasSuffix(line, `\`) {
			pending = strings.TrimSuffix(line, `\`)
			continue
		}
		if strings.HasPrefix(line, "nimbu ") {
			lines = append(lines, line)
		}
	}
	return lines
}

// Regression: the README `nimbu server` override example used
// `--arg --port`, which Kong rejects. Every documented command must parse.
func TestDocExamplesParse(t *testing.T) {
	count := 0
	for _, path := range docFiles {
		for _, line := range docCommandLines(t, path) {
			// Shell operators end the nimbu invocation.
			for _, stop := range []string{" && ", " || ", " ; ", " > ", " 2>", " # "} {
				if i := strings.Index(line, stop); i >= 0 {
					line = line[:i]
				}
			}
			args := exampleArgs(line)
			if len(args) == 0 || args[0] == helpCommandName || hasHelpFlag(args) || hasCLIFlag(args, "--help-all") || hasCLIFlag(args, "--version") {
				continue // help output exits through the parser
			}
			parser, _, err := newParser()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parser.Parse(args); err != nil {
				t.Errorf("%s: %q does not parse: %v", filepath.Base(path), line, err)
			}
			count++
		}
	}
	if count < 50 {
		t.Fatalf("parsed only %d documented commands", count)
	}
}

// The README `## Commands` overview must list every visible top-level
// command, so it cannot drift from `nimbu --help` again.
func TestReadmeCommandsListsEveryCommand(t *testing.T) {
	data, err := os.ReadFile(docFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)
	start := strings.Index(readme, "## Commands\n")
	if start < 0 {
		t.Fatal("README has no ## Commands section")
	}
	section := readme[start:]
	if end := strings.Index(section[len("## Commands\n"):], "\n## "); end >= 0 {
		section = section[:end+len("## Commands\n")]
	}
	listed := map[string]bool{}
	for _, line := range strings.Split(section, "\n") {
		if fields := strings.Fields(line); len(fields) > 1 && fields[0] == "nimbu" {
			listed[fields[1]] = true
		}
	}
	parser, _, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range parser.Model.Children {
		if child.Hidden {
			continue
		}
		if !listed[child.Name] {
			t.Errorf("README ## Commands is missing %q", child.Name)
		}
	}
}
