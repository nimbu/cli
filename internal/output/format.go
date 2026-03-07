package output

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/term"
)

// Mode represents the output format mode.
type Mode struct {
	JSON  bool
	Plain bool
}

// ParseError indicates invalid output mode configuration.
type ParseError struct{ msg string }

func (e *ParseError) Error() string { return e.msg }

// FromFlags creates a Mode from CLI flags.
func FromFlags(jsonOut, plainOut bool) (Mode, error) {
	if jsonOut && plainOut {
		return Mode{}, &ParseError{msg: "cannot combine --json and --plain"}
	}
	return Mode{JSON: jsonOut, Plain: plainOut}, nil
}

// FromEnv creates a Mode from environment variables.
func FromEnv() Mode {
	return Mode{
		JSON:  envBool("NIMBU_JSON"),
		Plain: envBool("NIMBU_PLAIN"),
	}
}

type ctxKey struct{}

// WithMode adds output mode to context.
func WithMode(ctx context.Context, mode Mode) context.Context {
	return context.WithValue(ctx, ctxKey{}, mode)
}

// FromContext extracts output mode from context.
func FromContext(ctx context.Context) Mode {
	if v := ctx.Value(ctxKey{}); v != nil {
		if m, ok := v.(Mode); ok {
			return m
		}
	}
	return Mode{}
}

// IsJSON returns true if JSON output is enabled.
func IsJSON(ctx context.Context) bool { return FromContext(ctx).JSON }

// IsPlain returns true if plain/TSV output is enabled.
func IsPlain(ctx context.Context) bool { return FromContext(ctx).Plain }

// IsHuman returns true if human-readable output is enabled (neither JSON nor plain).
func IsHuman(ctx context.Context) bool {
	m := FromContext(ctx)
	return !m.JSON && !m.Plain
}

// Writer holds output configuration.
type Writer struct {
	Out    io.Writer
	Err    io.Writer
	Mode   Mode
	NoTTY  bool
	Color  string // "auto", "always", "never"
	NoSpin bool   // Disable spinner
}

// DefaultWriter returns a writer with default settings.
func DefaultWriter() *Writer {
	return &Writer{
		Out:   os.Stdout,
		Err:   os.Stderr,
		Mode:  FromEnv(),
		Color: "auto",
	}
}

// IsTTY returns true if stdout is a terminal.
func (w *Writer) IsTTY() bool {
	if w.NoTTY {
		return false
	}
	if f, ok := w.Out.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// ErrIsTTY returns true if stderr is a terminal.
func (w *Writer) ErrIsTTY() bool {
	if w.NoTTY {
		return false
	}
	if f, ok := w.Err.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// UseColor returns true if color output should be used.
func (w *Writer) UseColor() bool {
	switch w.Color {
	case "always":
		return true
	case "never":
		return false
	default: // "auto"
		return w.IsTTY()
	}
}

type writerCtxKey struct{}

// WithWriter adds writer to context.
func WithWriter(ctx context.Context, w *Writer) context.Context {
	return context.WithValue(ctx, writerCtxKey{}, w)
}

// WriterFromContext extracts writer from context.
func WriterFromContext(ctx context.Context) *Writer {
	if v := ctx.Value(writerCtxKey{}); v != nil {
		if w, ok := v.(*Writer); ok {
			return w
		}
	}
	return DefaultWriter()
}

func envBool(key string) bool {
	v := os.Getenv(key)
	return v == "1" || v == "true" || v == "yes"
}

var ErrOutputConflict = errors.New("cannot combine --json and --plain")
