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
			// pages set takes a payload value, not a resource identity.
			if strings.Contains(compactLine, "Value string") && strings.Contains(compactLine, `xor:"set-source"`) {
				continue
			}
			if file == "api.go" && strings.Contains(compactLine, "Path string") {
				continue
			}
			// `nimbu init [directory]` takes a filesystem destination (like
			// `git init <dir>`), not a resource-identity value, so a positional
			// argument is the idiomatic syntax here.
			if strings.Contains(compactLine, "Directory string") && strings.Contains(compactLine, `help:"Target directory`) {
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
		{"themes", "push", "--only=templates/page.liquid", "--no-deps"},
		{"pages", "update", "--page=about", "--dry-run", "title=About"},
		{"themes", "sync", "--only=assets/app.css,layouts/theme.liquid"},
		{"apps", "push", "--only=code/main.js,code/hooks.js"},
		{"sites", "settings", "--site=staging"},
		{"roles", "customers", "add", "--role=bingo", "--customer=c1"},
		{"roles", "customers", "set", "--role=bingo", "--customer=c1", "--customer=c2"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, err := parser.Parse(args); err != nil {
				t.Fatalf("parse failed: %v", err)
			}
		})
	}
}

func TestLocalizedCreateLocaleFlagParsesIntoCommand(t *testing.T) {
	parser, cli, err := newParser()
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}
	if _, err := parser.Parse([]string{"products", "create", "--locale=nl-BE", "name=Wine"}); err != nil {
		t.Fatalf("parse localized create: %v", err)
	}
	if cli.Products.Create.Locale != "nl-BE" {
		t.Fatalf("products create locale = %q", cli.Products.Create.Locale)
	}
}

func TestThemeResourceGetAliasesParseToSameValue(t *testing.T) {
	tests := []struct {
		name string
		args [][]string
		got  func(*CLI) string
		want string
	}{
		{
			name: "templates",
			args: [][]string{
				{"themes", "templates", "get", "--theme=t", "--name=page.liquid"},
				{"themes", "templates", "get", "--theme=t", "--template=page.liquid"},
			},
			got:  func(cli *CLI) string { return cli.Themes.Templates.Get.Name },
			want: "page.liquid",
		},
		{
			name: "snippets",
			args: [][]string{
				{"themes", "snippets", "get", "--theme=t", "--name=header.liquid"},
				{"themes", "snippets", "get", "--theme=t", "--snippet=header.liquid"},
			},
			got:  func(cli *CLI) string { return cli.Themes.Snippets.Get.Name },
			want: "header.liquid",
		},
		{
			name: "layouts",
			args: [][]string{
				{"themes", "layouts", "get", "--theme=t", "--name=theme.liquid"},
				{"themes", "layouts", "get", "--theme=t", "--layout=theme.liquid"},
			},
			got:  func(cli *CLI) string { return cli.Themes.Layouts.Get.Name },
			want: "theme.liquid",
		},
		{
			name: "assets",
			args: [][]string{
				{"themes", "assets", "get", "--theme=t", "--path=assets/app.css"},
				{"themes", "assets", "get", "--theme=t", "--asset=assets/app.css"},
			},
			got:  func(cli *CLI) string { return cli.Themes.Assets.Get.Path },
			want: "assets/app.css",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var values []string
			for _, args := range test.args {
				parser, cli, err := newParser()
				if err != nil {
					t.Fatalf("newParser: %v", err)
				}
				if _, err := parser.Parse(args); err != nil {
					t.Fatalf("parse %q: %v", strings.Join(args, " "), err)
				}
				values = append(values, test.got(cli))
			}
			if values[0] != test.want || values[1] != test.want {
				t.Fatalf("parsed values = %q, want both %q", values, test.want)
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
