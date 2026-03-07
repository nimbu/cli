package apps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/config"
)

func TestOrderFilesHandlesRequireAndImport(t *testing.T) {
	root := t.TempDir()
	app := AppConfig{
		AppProjectConfig: config.AppProjectConfig{Dir: "code"},
		ProjectRoot:      root,
	}
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

	ordered, err := OrderFiles(app, []string{"code/index.js", "code/util.js", "code/helper.js", "code/leaf.js", "code/standalone.js"})
	if err != nil {
		t.Fatalf("order files: %v", err)
	}
	got := strings.Join(ordered, ",")
	if !strings.Contains(got, "code/leaf.js,code/helper.js,code/util.js,code/index.js") {
		t.Fatalf("unexpected order: %s", got)
	}
}

func TestOrderFilesRejectsCycles(t *testing.T) {
	root := t.TempDir()
	app := AppConfig{
		AppProjectConfig: config.AppProjectConfig{Dir: "code"},
		ProjectRoot:      root,
	}
	for path, content := range map[string]string{
		"code/a.js": `require("./b")`,
		"code/b.js": `require("./a")`,
	} {
		abs := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	_, err := OrderFiles(app, []string{"code/a.js", "code/b.js"})
	if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}
