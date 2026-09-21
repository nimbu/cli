package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/pagepath"
)

// PagesBatchCmd applies multiple page operations from a file.
type PagesBatchCmd struct {
	Page   string `required:"" help:"Page fullpath or id"`
	File   string `required:"" help:"JSON operations object or array"`
	Atomic bool   `default:"true" negatable:"" help:"Fail the whole batch if any operation fails"`
	Locale string `help:"Content locale for localized fields"`
	Diff   bool   `help:"Show a unified diff of each changed subtree"`
	DryRun bool   `help:"Resolve paths and print the operations without writing"`
	Draft  bool   `help:"Write to the page draft instead of the live page"`
}

// Run executes pages batch.
func (c *PagesBatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "batch page operations"); err != nil {
		return err
	}
	if c.Draft && !c.Atomic {
		return newDetailedError(
			errors.New("--no-atomic cannot be combined with --draft: draft batches are always atomic server-side"),
			errorRequestInvalid, ExitUsage,
			map[string]any{"flags": []string{"--draft", "--no-atomic"}},
		)
	}

	session, err := openSurgicalPage(ctx, flags, c.Page, c.Locale)
	if err != nil {
		return err
	}
	write := surgicalWriteFlags{Locale: c.Locale, Diff: c.Diff, DryRun: c.DryRun, Draft: c.Draft}
	if err := session.applyWriteMode(write); err != nil {
		return err
	}
	before := cloneMap(session.doc)

	ops, err := c.plan(session)
	if err != nil {
		return err
	}
	if write.DryRun {
		return printDryRun(ctx, ops)
	}

	result, err := c.post(session, ops)
	if err != nil {
		return err
	}
	session.absorbResult(result)
	return printSurgicalResult(ctx, session, ops, result, before, write, "")
}

func (c *PagesBatchCmd) plan(session *surgicalSession) ([]plannedOp, error) {
	raw, err := readJSONAnyInput(c.File)
	if err != nil {
		return nil, err
	}
	list, err := batchOperationsFromFile(raw)
	if err != nil {
		return nil, err
	}
	if len(list) > maxPageBatchOps {
		return nil, fmt.Errorf("batch is limited to %d operations", maxPageBatchOps)
	}

	ops := make([]plannedOp, 0, len(list))
	for _, item := range list {
		human := stringAny(item["path"])
		op, err := decodeBatchOp(item)
		if err != nil {
			return nil, err
		}
		build := func(s *surgicalSession) (api.BatchOperation, error) {
			next := op
			switch {
			case human != "" && !strings.HasPrefix(human, "/"):
				resolved, err := s.resolve(human)
				if err != nil {
					return api.BatchOperation{}, err
				}
				if next.Op == "insert" {
					if err := requireResolvedKind(resolved, human, pagepath.KindCanvas, canvasCandidates(s.doc)); err != nil {
						return api.BatchOperation{}, err
					}
				}
				next.Path = resolved.RawPath
			case next.Path != "":
				normalized, err := normalizeRawRepeatableIndexes(s.doc, next.Path)
				if err != nil {
					return api.BatchOperation{}, err
				}
				next.Path = normalized
			}
			if next.Op == "insert" {
				next.Path = insertRepeatablesPath(next.Path)
			}
			if next.Op == "set" {
				expanded, err := expandSetFileValue(s, next.Value)
				if err != nil {
					return api.BatchOperation{}, err
				}
				next.Value = expanded
			}
			return next, nil
		}
		built, err := build(session)
		if err != nil {
			return nil, err
		}
		ops = append(ops, plannedOp{Human: human, Op: built, rebuild: build})
	}
	return ops, nil
}

func (c *PagesBatchCmd) post(session *surgicalSession, ops []plannedOp) (*api.BatchResult, error) {
	if session.draftMode {
		return session.postBatch(ops)
	}
	batch := make([]api.BatchOperation, len(ops))
	for i, op := range ops {
		batch[i] = op.Op
	}
	result, err := session.client.PostPageBatch(session.ctx, session.pageID, batch, api.BatchOptions{
		Atomic:        c.Atomic,
		IncludeResult: true,
		ContentLocale: session.locale,
		IfMatch:       session.etag,
	})
	if err == nil {
		return result, nil
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 412 {
		return nil, err
	}
	if refetchErr := session.reload(); refetchErr != nil {
		return nil, refetchErr
	}
	if current := apiErr.CurrentETag(); current != "" {
		session.etag = strings.Trim(current, `"`)
	}
	retried := make([]plannedOp, len(ops))
	for i, op := range ops {
		next := op
		if op.rebuild != nil {
			rebuilt, rebuildErr := op.rebuild(session)
			if rebuildErr != nil {
				return nil, rebuildErr
			}
			next.Op = rebuilt
		}
		retried[i] = next
	}
	batch = make([]api.BatchOperation, len(retried))
	for i, op := range retried {
		batch[i] = op.Op
	}
	return session.client.PostPageBatch(session.ctx, session.pageID, batch, api.BatchOptions{
		Atomic:        c.Atomic,
		IncludeResult: true,
		ContentLocale: session.locale,
		IfMatch:       session.etag,
	})
}

func batchOperationsFromFile(raw any) ([]map[string]any, error) {
	switch typed := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for i, item := range typed {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("operations[%d] must be an object", i)
			}
			out = append(out, object)
		}
		return out, nil
	case map[string]any:
		list, ok := typed["operations"].([]any)
		if !ok {
			return nil, fmt.Errorf("file must be {\"operations\":[...]} or a bare array")
		}
		return batchOperationsFromFile(list)
	default:
		return nil, fmt.Errorf("file must be {\"operations\":[...]} or a bare array")
	}
}

func decodeBatchOp(item map[string]any) (api.BatchOperation, error) {
	op := api.BatchOperation{
		Op:    stringAny(item["op"]),
		Path:  stringAny(item["path"]),
		Value: item["value"],
	}
	if op.Op == "" {
		return api.BatchOperation{}, fmt.Errorf("operation is missing op")
	}
	if err := validateBatchOpName(op.Op); err != nil {
		return api.BatchOperation{}, err
	}
	if err := validateBatchOpPath(op.Path); err != nil {
		return api.BatchOperation{}, err
	}
	if raw, ok := item["after"]; ok {
		switch typed := raw.(type) {
		case nil:
			op.After = &api.Anchor{Set: true}
		case string:
			op.After = &api.Anchor{Set: true, ID: typed}
		default:
			return api.BatchOperation{}, fmt.Errorf("after must be null or an id")
		}
	}
	return op, nil
}
