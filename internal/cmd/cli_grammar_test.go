package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicCommandsUseFlagsForIdentity(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("*.go"))
	if err != nil {
		t.Fatalf("glob command files: %v", err)
	}

	var offenders []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for idx, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, `arg:""`) {
				continue
			}
			compactLine := strings.Join(strings.Fields(line), " ")
			if strings.Contains(compactLine, "Assignments []string") || strings.Contains(compactLine, "Words []string") {
				continue
			}
			offenders = append(offenders, filepath.ToSlash(file)+":"+itoa(idx+1)+": "+strings.TrimSpace(line))
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("public identity values must be flags, not positional args:\n%s", strings.Join(offenders, "\n"))
	}
}

func TestFlagFirstSyntaxParsesRepresentativeCommands(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	tests := [][]string{
		{"channels", "get", "--channel=blog"},
		{"channels", "fields", "list", "--channel=blog"},
		{"channels", "fields", "add", "--channel=blog", "--name=title", "type=string", "label=Title"},
		{"channels", "fields", "update", "--channel=blog", "--field=title", "label=Headline"},
		{"channels", "entries", "update", "--channel=blog", "--entry=start", "title=Hello"},
		{"pages", "get", "--page=about/team"},
		{"products", "update", "--product=sku-123", "name=Wine"},
		{"config", "set", "--key=default_site", "--value=demo"},
		{"api", "--method=GET", "--path=/channels"},
		{"completion", "--shell=zsh"},
		{"themes", "push", "--only=assets/app.css"},
		{"themes", "push", "--only=assets/app.css,layouts/theme.liquid"},
		{"themes", "sync", "--only=assets/app.css,layouts/theme.liquid"},
		{"apps", "push", "--only=code/main.js,code/hooks.js"},
		{"sites", "settings", "--site=staging"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, err := parser.Parse(args); err != nil {
				t.Fatalf("parse failed: %v", err)
			}
		})
	}
}

func TestOldPositionalIdentitySyntaxFails(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	tests := [][]string{
		{"channels", "get", "blog"},
		{"channels", "entries", "update", "blog", "start", "title=Hello"},
		{"pages", "get", "about/team"},
		{"products", "update", "sku-123", "name=Wine"},
		{"config", "set", "default_site", "demo"},
		{"api", "GET", "/channels"},
		{"completion", "zsh"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, err := parser.Parse(args); err == nil {
				t.Fatalf("expected parse failure")
			}
		})
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
