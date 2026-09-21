package cmd

import (
	"context"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/pagepath"
)

// PagesDeleteBlockCmd deletes a repeatable block.
type PagesDeleteBlockCmd struct {
	Page   string `required:"" help:"Page fullpath or id"`
	Path   string `required:"" help:"Repeatable path to delete"`
	Diff   bool   `help:"Show a unified diff of the canvas repeatables"`
	DryRun bool   `help:"Resolve paths and print the operations without writing"`
	Draft  bool   `help:"Write to the page draft instead of the live page"`
}

// Run executes pages delete-block.
func (c *PagesDeleteBlockCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete page block"); err != nil {
		return err
	}
	if err := requireForce(flags, "block "+c.Path); err != nil {
		return err
	}

	session, err := openSurgicalPage(ctx, flags, c.Page, "")
	if err != nil {
		return err
	}
	write := surgicalWriteFlags{Diff: c.Diff, DryRun: c.DryRun, Draft: c.Draft}
	if err := session.applyWriteMode(write); err != nil {
		return err
	}
	before := cloneMap(session.doc)

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

func (c *PagesDeleteBlockCmd) plan(session *surgicalSession) (plannedOp, error) {
	build := func(s *surgicalSession) (api.BatchOperation, error) {
		resolved, err := s.resolveUserPath(c.Path)
		if err != nil {
			return api.BatchOperation{}, err
		}
		if err := requireResolvedKind(resolved, c.Path, pagepath.KindRepeatable, formatRepeatableCandidates(resolved.Siblings)); err != nil {
			return api.BatchOperation{}, err
		}
		return api.BatchOperation{Op: "delete", Path: resolved.RawPath}, nil
	}
	op, err := build(session)
	if err != nil {
		return plannedOp{}, err
	}
	return plannedOp{Human: c.Path, Op: op, rebuild: build}, nil
}
