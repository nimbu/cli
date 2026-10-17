//go:build !windows

package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/output"
)

// A signal during the git steps of `init --no-input` cancels the command
// context: git must stop and init must report the interruption, not success.
func TestInitNoInputStopsGitWhenInterrupted(t *testing.T) {
	sourceDir := t.TempDir()
	writeInitStarterFixture(t, sourceDir)
	outputDir := t.TempDir()
	srv := initTwoThemeServer(t)

	// The fake git stalls on `git init` and succeeds at everything else.
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "git-init-started")
	script := "#!/bin/sh\nfor a in \"$@\"; do\n  if [ \"$a\" = init ]; then : > \"$GIT_MARKER\"; exec sleep 3; fi\ndone\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIT_MARKER", marker)

	baseCtx, stdout, _ := newInitTestContext(t, srv.URL, output.Mode{JSON: true})
	ctx, cancel := context.WithCancelCause(baseCtx)
	defer cancel(nil)
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(marker); err == nil {
				cancel(errInterrupted) // what the root SIGINT/SIGTERM handler does
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	var err error
	start := time.Now()
	withTempCWD(t, t.TempDir(), func() {
		withTempStdin(t, "", func() {
			cmd := &InitCmd{Dir: sourceDir, OutputDir: outputDir, Theme: "outlet", Repeatables: "all", Bundles: "blog-articles", OnConflict: initConflictAsk}
			err = cmd.Run(ctx, &RootFlags{NoInput: true, Site: "demo-shop"})
		})
	})

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != initExitInterrupted {
		t.Fatalf("Run() = %v, want exit %d", err, initExitInterrupted)
	}
	if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
		t.Fatalf("init took %s; the stalled git step should have been killed", elapsed)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no success result, got %s", stdout.String())
	}
}
