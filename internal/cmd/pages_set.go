package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/pagepath"
)

// PagesSetCmd sets one page field or editable by path.
type PagesSetCmd struct {
	Page     string `required:"" help:"Page fullpath or id"`
	Path     string `required:"" help:"Human or raw path to the field or editable"`
	Value    string `arg:"" optional:"" xor:"set-source" help:"Literal value to set"`
	File     string `help:"JSON value from file (use - for stdin)" xor:"set-source"`
	FromFile string `name:"from-file" help:"Local file to upload as a file editable" xor:"set-source"`
	Locale   string `help:"Content locale for localized fields"`
	Diff     bool   `help:"Show a unified diff of the changed subtree"`
	DryRun   bool   `help:"Resolve paths and print the operations without writing"`
}

// Run executes pages set.
func (c *PagesSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "set page"); err != nil {
		return err
	}
	if err := exclusiveValueSource(c.Value, c.File, c.FromFile); err != nil {
		return err
	}

	session, err := openSurgicalPage(ctx, flags, c.Page, c.Locale)
	if err != nil {
		return err
	}
	before := cloneMap(session.doc)
	write := surgicalWriteFlags{Locale: c.Locale, Diff: c.Diff, DryRun: c.DryRun}

	op, err := c.plan(session)
	if err != nil {
		return err
	}
	result, err := session.runBatch([]plannedOp{op}, write)
	if err != nil || write.DryRun {
		return err
	}
	return printSurgicalResult(ctx, session, []plannedOp{op}, result, before, write, "")
}

func (c *PagesSetCmd) plan(session *surgicalSession) (plannedOp, error) {
	build := func(s *surgicalSession) (api.BatchOperation, error) {
		resolved, err := s.resolve(c.Path)
		if err != nil {
			return api.BatchOperation{}, err
		}
		value, err := c.valueFor(resolved)
		if err != nil {
			return api.BatchOperation{}, err
		}
		return api.BatchOperation{Op: "set", Path: resolved.RawPath, Value: value}, nil
	}
	op, err := build(session)
	if err != nil {
		return plannedOp{}, err
	}
	return plannedOp{Human: c.Path, Op: op, rebuild: build}, nil
}

func (c *PagesSetCmd) valueFor(resolved pagepath.Resolved) (any, error) {
	switch {
	case c.File != "":
		return readJSONAnyInput(c.File)
	case c.FromFile != "":
		if resolved.Type != "file" {
			return nil, fmt.Errorf("--from-file requires a file editable, got %q", resolved.Type)
		}
		return encodeLocalFileValue(c.FromFile)
	default:
		return coerceSetValue(resolved, c.Value)
	}
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
