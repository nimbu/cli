package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/pagepath"
)

type surgicalJSONResult struct {
	Results   []api.BatchOpResult `json:"results"`
	ETag      string              `json:"etag"`
	UpdatedAt string              `json:"updated_at"`
	Diff      []surgicalDiffEntry `json:"diff,omitempty"`
}

type dryRunOutput struct {
	Operations []api.BatchOperation `json:"operations"`
	Paths      []dryRunPath         `json:"paths,omitempty"`
}

type dryRunPath struct {
	Path    string `json:"path"`
	RawPath string `json:"raw_path"`
}

func printDryRun(ctx context.Context, ops []plannedOp) error {
	out := dryRunOutput{Operations: make([]api.BatchOperation, len(ops))}
	for i, op := range ops {
		out.Operations[i] = op.Op
		if op.Human != "" {
			out.Paths = append(out.Paths, dryRunPath{Path: op.Human, RawPath: op.Op.Path})
		}
	}
	return output.JSON(ctx, out)
}

func printSurgicalResult(ctx context.Context, session *surgicalSession, ops []plannedOp, result *api.BatchResult, before map[string]any, write surgicalWriteFlags, extraHuman string) error {
	if write.Draft {
		return printDraftSurgicalResult(ctx, session, ops, result, before, write, extraHuman)
	}
	mode := output.FromContext(ctx)
	diffs, err := computeSurgicalDiffs(ops, before, session.doc, result)
	if err != nil {
		return err
	}
	if mode.JSON {
		payload := surgicalJSONResult{Results: result.Results, ETag: result.ETag, UpdatedAt: result.UpdatedAt}
		if write.Diff {
			payload.Diff = diffs
		}
		return output.JSON(ctx, payload)
	}
	writeBatchResultLines(output.WriterFromContext(ctx).Out, ops, result.Results)
	if extraHuman != "" {
		if _, err := output.Fprintln(ctx, extraHuman); err != nil {
			return err
		}
	}
	if _, err := output.Fprintf(ctx, "Updated page %s (etag %s)\n", session.fullpath, shortETag(result.ETag)); err != nil {
		return err
	}
	if write.Diff {
		return printSurgicalDiffs(ctx, diffs)
	}
	return nil
}

func printDraftSurgicalResult(ctx context.Context, session *surgicalSession, ops []plannedOp, result *api.BatchResult, before map[string]any, write surgicalWriteFlags, extraHuman string) error {
	mode := output.FromContext(ctx)
	diffOK := session.draftConverted
	var diffs []surgicalDiffEntry
	if diffOK {
		var err error
		diffs, err = computeSurgicalDiffs(ops, before, session.doc, result)
		if err != nil {
			return err
		}
	}
	if mode.JSON {
		payload := map[string]any{
			"results": result.Results,
			"draft":   draftJSONMeta(session),
		}
		if write.Diff && diffOK {
			payload["diff"] = diffs
		}
		return output.JSON(ctx, payload)
	}
	writeBatchResultLines(output.WriterFromContext(ctx).Out, ops, result.Results)
	if extraHuman != "" {
		if _, err := output.Fprintln(ctx, extraHuman); err != nil {
			return err
		}
	}
	if _, err := output.Fprintf(ctx, "Updated draft of %s (draft %s); preview: nimbu pages draft preview-url --page %s\n", session.fullpath, draftID(session), session.fullpath); err != nil {
		return err
	}
	if write.Diff && !diffOK {
		_, err := output.Fprintln(ctx, "(diff unavailable for drafts)")
		return err
	}
	if write.Diff {
		return printSurgicalDiffs(ctx, diffs)
	}
	return nil
}

func draftJSONMeta(session *surgicalSession) map[string]any {
	meta := map[string]any{
		"id":      draftID(session),
		"page_id": session.pageID,
	}
	if session.draft != nil && session.draft.UpdatedAt != "" {
		meta["updated_at"] = session.draft.UpdatedAt
	}
	return meta
}

func draftID(session *surgicalSession) string {
	if session != nil && session.draft != nil {
		return session.draft.ID
	}
	return ""
}

func writeBatchResultLines(w io.Writer, ops []plannedOp, results []api.BatchOpResult) {
	for i, result := range results {
		opName := ""
		if i < len(ops) {
			opName = ops[i].Op.Op
		}
		line := fmt.Sprintf("%-5s %-6s %s", result.Status, opName, displayRawPath(result.Path))
		if result.Warning != "" {
			line += "  warning: " + result.Warning
		}
		if result.Error != nil && result.Error.Message != "" {
			line += "  " + result.Error.Message
		}
		_, _ = fmt.Fprintln(w, line)
	}
}

func displayRawPath(raw string) string {
	if raw == "" {
		return raw
	}
	parts := strings.Split(raw, "/")
	for i, part := range parts {
		if part == "" || part == "items" || part == "repeatables" {
			continue
		}
		decoded, err := pagepath.UnescapeName(part)
		if err != nil {
			continue
		}
		parts[i] = decoded
	}
	return strings.Join(parts, "/")
}

func shortETag(etag string) string {
	etag = strings.Trim(etag, `"`)
	if len(etag) > 8 {
		return etag[:8]
	}
	return etag
}

func mergeBatchResults(parts ...*api.BatchResult) *api.BatchResult {
	merged := &api.BatchResult{}
	for _, part := range parts {
		if part == nil {
			continue
		}
		merged.Results = append(merged.Results, part.Results...)
		if part.ETag != "" {
			merged.ETag = part.ETag
		}
		if part.UpdatedAt != "" {
			merged.UpdatedAt = part.UpdatedAt
		}
		if len(part.Page) > 0 {
			merged.Page = part.Page
		}
	}
	return merged
}

func prettyJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}
