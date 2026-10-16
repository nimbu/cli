package output

import (
	"context"
	"fmt"
)

// Fprintf writes formatted text to the context Writer's stdout.
func Fprintf(ctx context.Context, format string, args ...any) (int, error) {
	w := WriterFromContext(ctx)
	return fmt.Fprintf(w.Out, format, args...)
}

// Fprintln writes a line to the context Writer's stdout.
func Fprintln(ctx context.Context, args ...any) (int, error) {
	w := WriterFromContext(ctx)
	return fmt.Fprintln(w.Out, args...)
}

// Warn writes "warning: <msg>" to the context Writer's stderr. Warnings never
// go to stdout, so they cannot corrupt --plain rows or --json documents.
func Warn(ctx context.Context, msg string) error {
	w := WriterFromContext(ctx)
	if w.Err == nil {
		return nil
	}
	_, err := fmt.Fprintf(w.Err, "%s %s\n", w.ErrTheme().Warning("warning:"), msg)
	return err
}
