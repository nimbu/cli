package channeltypes

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestModuleTypeChecksAgainstSDK compiles the golden module plus a cloud code
// sample (testdata/typecheck/use.ts) against a nimbu-js-sdk checkout with
// strict tsc. It needs node and the SDK's dependencies, so it only runs when
// NIMBU_JS_SDK_DIR points at the checkout:
//
//	NIMBU_JS_SDK_DIR=../nimbu-js-sdk go test ./internal/channeltypes -run TypeChecks
//
// The SDK declarations are emitted into a temp dir, so the checkout's dist/
// does not need rebuilding and is never touched.
func TestModuleTypeChecksAgainstSDK(t *testing.T) {
	sdkDir := os.Getenv("NIMBU_JS_SDK_DIR")
	if sdkDir == "" {
		t.Skip("set NIMBU_JS_SDK_DIR to a nimbu-js-sdk checkout to type-check the generated module")
	}
	sdkDir, err := filepath.Abs(sdkDir)
	if err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join(sdkDir, "node_modules", ".bin", "tsc")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("tsc not found in the SDK checkout (run pnpm install there): %v", err)
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found")
	}

	tmp := t.TempDir()
	pkg := filepath.Join(tmp, "nimbu-js-sdk")
	run(t, sdkDir, tsc, "--project", "tsconfig.build.json", "--declaration", "--emitDeclarationOnly",
		"--noEmit", "false", "--outDir", filepath.Join(pkg, "dist"))
	copyFile(t, filepath.Join(sdkDir, "package.json"), filepath.Join(pkg, "package.json"))
	run(t, sdkDir, "cp", "-R", filepath.Join(sdkDir, "cloud"), filepath.Join(pkg, "cloud"))
	symlink(t, filepath.Join(sdkDir, "node_modules"), filepath.Join(pkg, "node_modules"))

	app := filepath.Join(tmp, "app")
	symlink(t, pkg, filepath.Join(app, "node_modules", "nimbu-js-sdk"))
	writeFile(t, filepath.Join(app, "tsconfig.json"), `{
  "compilerOptions": {
    "strict": true, "noEmit": true, "lib": ["es2022"], "types": ["nimbu-js-sdk/cloud"],
    "module": "commonjs", "moduleResolution": "node", "skipLibCheck": true
  },
  "include": ["code/**/*"]
}
`)
	golden, err := os.ReadFile(filepath.Join("testdata", "channels.d.ts"))
	if err != nil {
		t.Fatal(err)
	}
	// Checked as .ts, the file name the docs recommend: a .d.ts file would be
	// skipped under skipLibCheck and hide broken imports.
	writeFile(t, filepath.Join(app, "code", "nimbu-channels.ts"), string(golden))
	copyFile(t, filepath.Join("testdata", "typecheck", "use.ts"), filepath.Join(app, "code", "use.ts"))

	run(t, app, tsc, "--project", ".")
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, to, string(data))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
