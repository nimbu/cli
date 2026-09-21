package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	Draft    bool   `help:"Write to the page draft instead of the live page"`
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
	write := surgicalWriteFlags{Locale: c.Locale, Diff: c.Diff, DryRun: c.DryRun, Draft: c.Draft}
	if err := session.applyWriteMode(write); err != nil {
		return err
	}

	op, err := c.plan(session)
	if err != nil {
		return err
	}
	before := cloneMap(session.doc)
	result, err := session.runBatch([]plannedOp{op}, write)
	if err != nil || write.DryRun {
		return err
	}
	return printSurgicalResult(ctx, session, []plannedOp{op}, result, before, write, "")
}

func (c *PagesSetCmd) plan(session *surgicalSession) (plannedOp, error) {
	var kind pagepath.Kind
	build := func(s *surgicalSession) (api.BatchOperation, error) {
		resolved, err := s.resolveUserPath(c.Path)
		if err != nil {
			return api.BatchOperation{}, err
		}
		kind = resolved.Kind
		value, err := c.valueFor(s, resolved)
		if err != nil {
			return api.BatchOperation{}, err
		}
		return api.BatchOperation{Op: "set", Path: resolved.RawPath, Value: value}, nil
	}
	op, err := build(session)
	if err != nil {
		return plannedOp{}, err
	}
	return plannedOp{Human: c.Path, Kind: kind, Op: op, rebuild: build}, nil
}

func (c *PagesSetCmd) valueFor(session *surgicalSession, resolved pagepath.Resolved) (any, error) {
	switch {
	case c.File != "":
		raw, err := readJSONAnyInput(c.File)
		if err != nil {
			return nil, err
		}
		return expandSetFileValue(session, raw)
	case c.FromFile != "":
		if resolved.Type != "file" {
			return nil, fmt.Errorf("--from-file requires a file editable, got %q", resolved.Type)
		}
		return encodeLocalFileValue(c.FromFile)
	default:
		if resolved.Type == "file" {
			if value, ok, err := parseFileSetValue(c.Value); err != nil {
				return nil, err
			} else if ok {
				return expandSetFileValue(session, value)
			}
		}
		return coerceSetValue(resolved, c.Value)
	}
}

func parseFileSetValue(raw string) (any, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false, nil
	}
	if strings.HasPrefix(trimmed, "{") {
		var value any
		if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
			return nil, false, fmt.Errorf("parse file value JSON: %w", err)
		}
		return value, true, nil
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "nimbu://") {
		return map[string]any{"attachment_url": trimmed}, true, nil
	}
	return nil, false, nil
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	data, err := json.Marshal(in)
	if err != nil {
		out := make(map[string]any, len(in))
		for key, value := range in {
			out[key] = value
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
