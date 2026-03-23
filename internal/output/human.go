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
