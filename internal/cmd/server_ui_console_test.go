package cmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/muesli/termenv"

	"github.com/nimbu/cli/internal/output"
)

func TestServerPresenterSkipsBannerWhenStderrIsNotTerminal(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "always"})

	forceBannerTerminal(t, false)
	newServerPresenter(ctx, false).PrintBanner()
	if buf.Len() != 0 {
		t.Fatalf("expected no banner on a non-terminal stderr, got %q", buf.String())
	}

	// A terminal outside CI keeps the banner.
	forceBannerTerminal(t, true)
	buf.Reset()
	newServerPresenter(ctx, false).PrintBanner()
	if !strings.Contains(buf.String(), "██╗") {
		t.Fatalf("expected banner on a terminal, got %q", buf.String())
	}

	// CI hides it, like every other banner.
	t.Setenv("CI", "true")
	buf.Reset()
	newServerPresenter(ctx, false).PrintBanner()
	if buf.Len() != 0 {
		t.Fatalf("expected no banner in CI, got %q", buf.String())
	}
}

func TestServerPresenterHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "auto"})

	presenter := newServerPresenter(ctx, false)
	presenter.PrintStopping("interrupt", serverForceHint(os.Interrupt, true))
	presenter.PrintReady("http://localhost:3456")

	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatalf("expected no ANSI with NO_COLOR, got %q", buf.String())
	}
}

func TestServerPresenterStopAndReadyNotices(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	presenter := newServerPresenter(ctx, false)
	presenter.PrintReady("http://localhost:3456")
	presenter.PrintStopping("interrupt", serverForceHint(os.Interrupt, true))
	presenter.PrintForceStop()

	want := "  ✓ ready http://localhost:3456\nStopping… (Ctrl-C again to force)\nForcing stop.\n"
	if got := buf.String(); got != want {
		t.Fatalf("notices = %q, want %q", got, want)
	}
}

func TestServerPresenterMachineModeStopNotices(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{JSON: true})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	presenter := newServerPresenter(ctx, false)
	presenter.PrintReady("http://localhost:3456")
	presenter.PrintStopping("", serverForceHint(nil, false))
	presenter.PrintStopping("hangup", serverForceHint(syscall.SIGHUP, false))
	presenter.PrintForceStop()

	want := "received signal: hangup; stopping (send the signal again to force)\nforcing stop\n"
	if got := buf.String(); got != want {
		t.Fatalf("machine notices = %q, want %q", got, want)
	}
}

// Regression: a SIGTERM from a supervisor (or any signal without a terminal on
// stdin) was answered with "Ctrl-C again to force".
func TestServerForceHintNamesCtrlCOnlyForInteractiveInterrupts(t *testing.T) {
	cases := []struct {
		name     string
		sig      os.Signal
		stdinTTY bool
		want     string
	}{
		{"ctrl-c on a terminal", os.Interrupt, true, "Ctrl-C again to force"},
		{"q shortcut", nil, true, "Ctrl-C again to force"},
		{"sigint without a terminal", os.Interrupt, false, "send the signal again to force"},
		{"sigterm on a terminal", syscall.SIGTERM, true, "send the signal again to force"},
		{"sigterm without a terminal", syscall.SIGTERM, false, "send the signal again to force"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serverForceHint(tc.sig, tc.stdinTTY); got != tc.want {
				t.Fatalf("serverForceHint(%v, %v) = %q, want %q", tc.sig, tc.stdinTTY, got, tc.want)
			}
		})
	}

	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{JSON: true})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})
	newServerPresenter(ctx, false).PrintStopping(serverSignalName(syscall.SIGTERM), serverForceHint(syscall.SIGTERM, false))
	if got := buf.String(); strings.Contains(got, "Ctrl-C") || !strings.Contains(got, "received signal: terminated; stopping") {
		t.Fatalf("machine SIGTERM notice = %q", got)
	}
}

func TestServerPresenterPrintShortcutHelp(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	presenter := newServerPresenter(ctx, false)
	presenter.PrintShortcutHelp(serverShortcutLinks{DevURL: "http://localhost:3456"})

	want := "  Shortcuts\n" +
		"    o      open the dev server  http://localhost:3456\n" +
		"    enter  print a blank line to mark a spot in the log\n" +
		"    ? h    show this help\n" +
		"    q      quit (Ctrl-C works too; press it twice to force)\n"
	if got := buf.String(); got != want {
		t.Fatalf("help = %q, want %q", got, want)
	}
}

func TestServerReadyTarget(t *testing.T) {
	if got := serverReadyTarget(serverSummary{ReadyURL: "http://127.0.0.1:3456", ProxyURL: "http://127.0.0.1:4568"}); got != "http://localhost:3456" {
		t.Fatalf("ready target = %q", got)
	}
	if got := serverReadyTarget(serverSummary{ProxyURL: "http://127.0.0.1:4568"}); got != "http://localhost:4568" {
		t.Fatalf("proxy ready target = %q", got)
	}
}

func TestNewServerStreamsPassesThroughWhenNotInteractive(t *testing.T) {
	var out, errOut bytes.Buffer
	writer := &output.Writer{Out: &out, Err: &errOut, Color: "always"}

	for _, human := range []bool{true, false} {
		streams := newServerStreams(writer, human)
		if streams.childOut != nil || streams.childErr != nil {
			t.Fatalf("human=%v: expected raw child passthrough", human)
		}
		// stdout is wrapped only to notice a closed pipe.
		watch, ok := streams.out.(*serverPipeWatch)
		if !ok || watch.dst != &out || streams.err != &errOut {
			t.Fatalf("human=%v: expected nimbu lines on the writer streams", human)
		}
		if streams.childForceColor != "" {
			t.Fatalf("human=%v: no FORCE_COLOR for an inheriting child", human)
		}
		if streams.requestLogColor != human {
			t.Fatalf("human=%v: requestLogColor = %v", human, streams.requestLogColor)
		}
	}
}

func TestApplyServerChildColorEnv(t *testing.T) {
	noEnv := func(string) string { return "" }

	env := map[string]string{}
	applyServerChildColorEnv(env, "3", noEnv)
	if env["FORCE_COLOR"] != "3" {
		t.Fatalf("FORCE_COLOR = %q, want 3", env["FORCE_COLOR"])
	}

	env = map[string]string{}
	applyServerChildColorEnv(env, "", noEnv)
	if _, ok := env["FORCE_COLOR"]; ok {
		t.Fatal("no level must not set FORCE_COLOR")
	}

	env = map[string]string{"NO_COLOR": "1"}
	applyServerChildColorEnv(env, "3", noEnv)
	if _, ok := env["FORCE_COLOR"]; ok {
		t.Fatal("project NO_COLOR must win")
	}

	env = map[string]string{}
	applyServerChildColorEnv(env, "3", func(key string) string {
		if key == "FORCE_COLOR" {
			return "0"
		}
		return ""
	})
	if _, ok := env["FORCE_COLOR"]; ok {
		t.Fatal("user FORCE_COLOR must win")
	}
}

func TestServerForceColorLevel(t *testing.T) {
	for profile, want := range map[termenv.Profile]string{
		termenv.TrueColor: "3",
		termenv.ANSI256:   "2",
		termenv.ANSI:      "1",
		termenv.Ascii:     "",
	} {
		if got := serverForceColorLevel(profile); got != want {
			t.Fatalf("profile %v: level %q, want %q", profile, got, want)
		}
	}
}

// Regression: under NIMBU_ASCII=1 (or TERM=dumb, or a non-UTF-8 locale) the
// ready line switched to "ok" but the stop line kept a Unicode ellipsis, the
// child prefix kept "│" and the banner kept its block art.
func TestServerPresenterASCIIGlyphs(t *testing.T) {
	t.Setenv("NIMBU_ASCII", "1")
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	forceBannerTerminal(t, true)
	presenter := newServerPresenter(ctx, false)
	presenter.PrintBanner()
	presenter.PrintReady("http://localhost:3456")
	presenter.PrintStopping("interrupt", serverForceHint(os.Interrupt, true))

	want := "  ok ready http://localhost:3456\nStopping... (Ctrl-C again to force)\n"
	if got := buf.String(); got != want {
		t.Fatalf("ASCII notices = %q, want %q", got, want)
	}
	if got := serverChildPrefix(); got != "dev | " {
		t.Fatalf("ASCII child prefix = %q, want %q", got, "dev | ")
	}
}
