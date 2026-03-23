package output

import (
	"bytes"
	"context"
)

func testContextWithMode(out, errOut *bytes.Buffer, mode Mode) context.Context {
	ctx := context.Background()
	ctx = WithMode(ctx, mode)
	ctx = WithWriter(ctx, &Writer{Out: out, Err: errOut, Mode: mode, NoTTY: true})
	return ctx
}
