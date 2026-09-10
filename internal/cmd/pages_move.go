package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/pagepath"
)

// PagesMoveCmd moves a repeatable within its canvas.
type PagesMoveCmd struct {
	Page     string `required:"" help:"Page fullpath or id"`
	Path     string `required:"" help:"Repeatable path to move"`
	After    string `help:"Place after this sibling id or 0-based index" xor:"move-at"`
	Position *int   `help:"0-based target index" xor:"move-at"`
	Diff     bool   `help:"Show a unified diff of the canvas repeatables"`
	DryRun   bool   `help:"Resolve paths and print the operations without writing"`
}

// Run executes pages move.
func (c *PagesMoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "move page block"); err != nil {
		return err
	}
	if c.After == "" && c.Position == nil {
		return fmt.Errorf("provide --after or --position")
	}

	session, err := openSurgicalPage(ctx, flags, c.Page, "")
	if err != nil {
		return err
	}
	before := cloneMap(session.doc)
	write := surgicalWriteFlags{Diff: c.Diff, DryRun: c.DryRun}

	op, noopAt, err := c.plan(session)
	if err != nil {
		return err
	}
	if noopAt >= 0 {
		if output.FromContext(ctx).JSON {
			return output.JSON(ctx, map[string]any{"status": "noop", "position": noopAt})
		}
		_, err := output.Fprintf(ctx, "already at position %d\n", noopAt)
		return err
	}
	result, err := session.runBatch([]plannedOp{op}, write)
	if err != nil || write.DryRun {
		return err
	}
	return printSurgicalResult(ctx, session, []plannedOp{op}, result, before, write, "")
}

func (c *PagesMoveCmd) plan(session *surgicalSession) (plannedOp, int, error) {
	build := func(s *surgicalSession) (api.BatchOperation, error) {
		op, _, err := c.build(s)
		return op, err
	}
	op, noopAt, err := c.build(session)
	if err != nil {
		return plannedOp{}, -1, err
	}
	if noopAt >= 0 {
		return plannedOp{}, noopAt, nil
	}
	return plannedOp{Human: c.Path, Op: op, rebuild: build}, -1, nil
}

func (c *PagesMoveCmd) build(session *surgicalSession) (api.BatchOperation, int, error) {
	resolved, err := session.resolve(c.Path)
	if err != nil {
		return api.BatchOperation{}, -1, err
	}
	if err := requireResolvedKind(resolved, c.Path, pagepath.KindRepeatable, formatRepeatableCandidates(resolved.Siblings)); err != nil {
		return api.BatchOperation{}, -1, err
	}
	target, err := c.targetIndex(resolved)
	if err != nil {
		return api.BatchOperation{}, -1, err
	}
	anchor, err := pagepath.AnchorForMove(resolved.Siblings, resolved.Index, target)
	if errors.Is(err, pagepath.ErrAnchorNoop) {
		return api.BatchOperation{}, target, nil
	}
	if err != nil {
		return api.BatchOperation{}, -1, err
	}
	return api.BatchOperation{Op: "move", Path: resolved.RawPath, After: batchAnchor(anchor)}, -1, nil
}

func (c *PagesMoveCmd) targetIndex(resolved pagepath.Resolved) (int, error) {
	if c.Position != nil {
		return *c.Position, nil
	}
	id, err := pagepath.ParseAnchor(c.After, resolved.Siblings)
	if err != nil {
		return 0, pathResolveError(err)
	}
	for _, sib := range resolved.Siblings {
		if sib.ID == id {
			// --after means "place after this sibling", so target is sibling index+1
			// unless that sibling is the current item. AnchorForMove uses targetIndex
			// as the destination slot, so treat --after like insert-after.
			target := sib.Index + 1
			if target > resolved.Index {
				target--
			}
			if target >= len(resolved.Siblings) {
				target = len(resolved.Siblings) - 1
			}
			return target, nil
		}
	}
	return 0, fmt.Errorf("anchor %q not found", c.After)
}
