package cmd

import (
	"io"
	"os"

	"github.com/muesli/termenv"

	"github.com/nimbu/cli/internal/devserver"
	"github.com/nimbu/cli/internal/output"
)

// serverChildPrefix marks child dev server lines in interactive sessions.
const serverChildPrefix = "dev │ "

// serverStreams are the writers nimbu server uses for its own lines and for
// the child dev server.
//
// In an interactive human session (stdout and stderr both terminals, no
// --json/--plain/--events-json) everything goes through one devserver.Console:
// lines are serialized so nothing lands mid-line, and child lines carry a dim
// "dev │" prefix. Otherwise output passes through raw, as before: the child
// inherits nimbu's stdout/stderr and request logs go to stdout.
//
// Streams: child stdout and proxy request logs go to stdout (colored by the
// stdout decision); child stderr and nimbu's notices go to stderr (colored by
// the stderr decision).
type serverStreams struct {
	// out carries proxy request logs and log markers (stdout).
	out io.Writer
	// err carries nimbu's notices, hints and warnings (stderr).
	err io.Writer
	// childOut / childErr are nil for raw passthrough (the child inherits).
	childOut io.Writer
	childErr io.Writer
	// requestLogColor is the stdout color decision for request log lines.
	requestLogColor bool
	// childForceColor is the FORCE_COLOR level handed to a piped child so it
	// keeps its colors; empty when the child inherits the terminal.
	childForceColor string
	// stdoutClosed is closed once a write to stdout failed with EPIPE.
	stdoutClosed <-chan struct{}
	closers      []io.Closer
}

func newServerStreams(writer *output.Writer, human bool) *serverStreams {
	stdout, stderr := writer.Out, writer.Err
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	pipeWatch := newServerPipeWatch(stdout)
	stdout = pipeWatch
	streams := &serverStreams{
		out:             stdout,
		err:             stderr,
		requestLogColor: human && writer.UseColor(),
		stdoutClosed:    pipeWatch.Closed(),
	}
	if !human || !writer.IsTTY() || !writer.ErrIsTTY() {
		return streams
	}

	console := devserver.NewConsole()
	childOut := console.Writer(stdout, styledServerChildPrefix(writer.OutColorProfile()))
	childErr := console.Writer(stderr, styledServerChildPrefix(writer.ErrColorProfile()))
	streams.out = console.Writer(stdout, "")
	streams.err = console.Writer(stderr, "")
	streams.childOut = childOut
	streams.childErr = childErr
	streams.childForceColor = serverForceColorLevel(writer.OutColorProfile())
	streams.closers = []io.Closer{childOut, childErr}
	return streams
}

// Close flushes any unterminated child line. Call it after the child exited.
func (s *serverStreams) Close() {
	for _, closer := range s.closers {
		_ = closer.Close()
	}
}

func styledServerChildPrefix(profile termenv.Profile) string {
	return profile.String(serverChildPrefix).Faint().String()
}

// serverForceColorLevel maps nimbu's color decision to the FORCE_COLOR level
// that Node tooling (chalk, supports-color, picocolors) reads.
func serverForceColorLevel(profile termenv.Profile) string {
	switch profile {
	case termenv.TrueColor:
		return "3"
	case termenv.ANSI256:
		return "2"
	case termenv.ANSI:
		return "1"
	default:
		return ""
	}
}

// applyServerChildColorEnv keeps child colors when its output is piped
// through the console. An explicit FORCE_COLOR or NO_COLOR from the user or
// the project config wins.
func applyServerChildColorEnv(env map[string]string, level string, getenv func(string) string) {
	if level == "" {
		return
	}
	for _, key := range []string{"FORCE_COLOR", "NO_COLOR"} {
		if _, ok := env[key]; ok {
			return
		}
		if getenv(key) != "" {
			return
		}
	}
	env["FORCE_COLOR"] = level
}
