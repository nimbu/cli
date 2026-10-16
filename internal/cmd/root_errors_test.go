package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

// captureStderr runs fn with os.Stderr redirected and returns what it wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = writer
	done := make(chan []byte)
	go func() {
		data, _ := io.ReadAll(reader)
		done <- data
	}()
	defer func() { os.Stderr = original }()
	fn()
	_ = writer.Close()
	os.Stderr = original
	return string(<-done)
}

func decodeEnvelope(t *testing.T, stderr string) errorEnvelope {
	t.Helper()
	var envelope errorEnvelope
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v\n%s", err, stderr)
	}
	return envelope
}

func TestRequestedJSON(t *testing.T) {
	t.Setenv("NIMBU_JSON", "")
	cases := map[string]bool{
		"--json":         true,
		"--json=true":    true,
		"--json=false":   false,
		"--plain":        false,
		"-- --json":      false,
		"themes --json":  true,
		"themes --jsonx": false,
	}
	for args, want := range cases {
		if got := requestedJSON(strings.Fields(args)); got != want {
			t.Fatalf("requestedJSON(%q) = %v, want %v", args, got, want)
		}
	}
	t.Setenv("NIMBU_JSON", "yes")
	if !requestedJSON(nil) {
		t.Fatal("NIMBU_JSON=yes should request JSON")
	}
}

func TestParseErrorEmitsJSONEnvelopeWhenRequested(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		env  string
	}{
		{name: "flag", args: []string{"--json", "themes", "push", "--bogus"}},
		{name: "env", args: []string{"themes", "push", "--bogus"}, env: "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NIMBU_JSON", tc.env)
			var code int
			stderr := captureStderr(t, func() { code = Execute(tc.args) })
			if code != ExitUsage {
				t.Fatalf("exit = %d, want %d", code, ExitUsage)
			}
			envelope := decodeEnvelope(t, stderr)
			if envelope.Status != "error" || envelope.Error.Code != errorUsageInvalid ||
				envelope.Error.Message != "unknown flag --bogus" || envelope.Error.ExitCode != ExitUsage {
				t.Fatalf("envelope = %+v", envelope)
			}
			if envelope.Error.Hint != "run `nimbu themes push --help` for usage" {
				t.Fatalf("hint = %q", envelope.Error.Hint)
			}
		})
	}
}

func TestParseErrorHumanOutputHasLabelsWithoutANSI(t *testing.T) {
	t.Setenv("NIMBU_JSON", "")
	var code int
	stderr := captureStderr(t, func() { code = Execute([]string{"themes", "push", "--bogus"}) })
	if code != ExitUsage {
		t.Fatalf("exit = %d", code)
	}
	want := "error: unknown flag --bogus\nhint: run `nimbu themes push --help` for usage\n"
	if stderr != want {
		t.Fatalf("stderr = %q, want %q", stderr, want)
	}
}

func TestJSONPlainConflictEmitsEnvelope(t *testing.T) {
	t.Setenv("NIMBU_JSON", "")
	t.Setenv("NIMBU_PLAIN", "1")
	var code int
	stderr := captureStderr(t, func() { code = Execute([]string{"--json", "config", "path"}) })
	if code != ExitUsage {
		t.Fatalf("exit = %d", code)
	}
	envelope := decodeEnvelope(t, stderr)
	if envelope.Error.Code != errorUsageInvalid || !strings.Contains(envelope.Error.Hint, "NIMBU_PLAIN") {
		t.Fatalf("envelope = %+v", envelope)
	}
}

func TestHumanErrorColorFollowsStderr(t *testing.T) {
	desc := classifyError(requireForce(&RootFlags{}, "channel news"))
	for _, tc := range []struct {
		color    string
		wantANSI bool
	}{
		{color: "never", wantANSI: false},
		{color: "auto", wantANSI: false}, // a buffer is not a terminal
		{color: "always", wantANSI: true},
	} {
		var buf bytes.Buffer
		writeHumanError(&output.Writer{Out: &bytes.Buffer{}, Err: &buf, Color: tc.color}, errors.New(desc.Message), desc)
		got := buf.String()
		if strings.Contains(got, "\x1b[") != tc.wantANSI {
			t.Fatalf("color=%s: output %q", tc.color, got)
		}
		lines := strings.Split(strings.TrimSpace(got), "\n")
		if len(lines) != 2 || !strings.Contains(lines[0], "error:") || !strings.Contains(lines[1], "hint:") {
			t.Fatalf("color=%s: want error line then hint line, got %q", tc.color, got)
		}
	}
}

func TestHumanErrorIsSilentOnInterrupt(t *testing.T) {
	var buf bytes.Buffer
	err := &interruptedError{err: errors.New("upload failed")}
	writeHumanError(&output.Writer{Err: &buf, Color: "never"}, err, classifyError(err))
	if buf.Len() != 0 {
		t.Fatalf("interrupt printed %q", buf.String())
	}
}

func TestErrorEnvelopeKeepsAngleBracketsReadable(t *testing.T) {
	var buf bytes.Buffer
	writeErrorEnvelope(&buf, classifyError(siteRequiredError(errProjectHasNoSite)))
	if strings.Contains(buf.String(), "\\u003c") || !strings.Contains(buf.String(), "--site <site>") {
		t.Fatalf("envelope = %s", buf.String())
	}
	decodeEnvelope(t, buf.String())
}

func TestRootWarningIsJSONInJSONMode(t *testing.T) {
	var buf bytes.Buffer
	writeRootWarning(&buf, true, `parse config: invalid character '}'`)
	var payload struct {
		Status  string `json:"status"`
		Warning struct {
			Message string `json:"message"`
		} `json:"warning"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil || payload.Status != "warning" || !strings.Contains(payload.Warning.Message, "parse config") {
		t.Fatalf("warning = %q (%v)", buf.String(), err)
	}
	buf.Reset()
	writeRootWarning(&buf, false, "invalid timeout in config")
	if buf.String() != "warning: invalid timeout in config\n" {
		t.Fatalf("human warning = %q", buf.String())
	}
}
