package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/pagepath"
)

// PagesInsertCmd inserts a repeatable into a canvas.
type PagesInsertCmd struct {
	Page     string `required:"" help:"Page fullpath or id"`
	Path     string `required:"" help:"Canvas path (Blokken, /items/Blokken, or /items/Blokken/repeatables)"`
	Slug     string `help:"Repeatable slug to insert"`
	After    string `help:"Place after this sibling id or 0-based index" xor:"insert-at"`
	Position *int   `help:"0-based insert index" xor:"insert-at"`
	File     string `help:"JSON items map, or {slug, items} object"`
	Locale   string `help:"Content locale for localized fields"`
	Diff     bool   `help:"Show a unified diff of the canvas repeatables"`
	DryRun   bool   `help:"Resolve paths and print the operations without writing"`
}

type insertPlan struct {
	insert plannedOp
	files  []plannedFileSet
	slug   string
	canvas string
}

type plannedFileSet struct {
	Field string
	Value any
}

// Run executes pages insert.
func (c *PagesInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "insert page block"); err != nil {
		return err
	}
	session, err := openSurgicalPage(ctx, flags, c.Page, c.Locale)
	if err != nil {
		return err
	}
	before := cloneMap(session.doc)
	write := surgicalWriteFlags{Locale: c.Locale, Diff: c.Diff, DryRun: c.DryRun}

	plan, err := c.plan(session)
	if err != nil {
		return err
	}

	if write.DryRun {
		ops := []plannedOp{plan.insert}
		for _, file := range plan.files {
			ops = append(ops, plannedOp{
				Human: file.Field,
				Op: api.BatchOperation{
					Op:    "set",
					Path:  "/items/" + pagepath.EscapeName(plan.canvas) + "/repeatables/<new-id>/items/" + pagepath.EscapeName(file.Field),
					Value: file.Value,
				},
			})
		}
		return printDryRun(ctx, ops)
	}

	insertResult, err := session.runBatch([]plannedOp{plan.insert}, write)
	if err != nil {
		return err
	}
	result := insertResult
	ops := []plannedOp{plan.insert}
	if len(plan.files) > 0 {
		newID := insertResultID(insertResult)
		if newID == "" {
			return fmt.Errorf("insert succeeded but the new repeatable id was missing")
		}
		fileOps := make([]plannedOp, 0, len(plan.files))
		for _, file := range plan.files {
			path := "/items/" + pagepath.EscapeName(plan.canvas) + "/repeatables/" + newID + "/items/" + pagepath.EscapeName(file.Field)
			fileOps = append(fileOps, plannedOp{
				Human: file.Field,
				Op:    api.BatchOperation{Op: "set", Path: path, Value: file.Value},
			})
		}
		fileResult, err := session.runBatch(fileOps, write)
		if err != nil {
			return err
		}
		result = mergeBatchResults(insertResult, fileResult)
		ops = append(ops, fileOps...)
	}

	index := repeatableIndex(session.doc, plan.canvas, insertResultID(insertResult))
	extra := fmt.Sprintf("Inserted %s at %s[%d] (id %s)", plan.slug, plan.canvas, index, insertResultID(insertResult))
	return printSurgicalResult(ctx, session, ops, result, before, write, extra)
}

func (c *PagesInsertCmd) plan(session *surgicalSession) (insertPlan, error) {
	resolved, items, slug, err := c.resolveInsert(session)
	if err != nil {
		return insertPlan{}, err
	}
	canvas := canvasNameFromRaw(resolved.RawPath)
	schema, err := session.ensureSchema()
	if err != nil {
		return insertPlan{}, err
	}
	if err := validateInsertSlug(schema, canvas, slug); err != nil {
		return insertPlan{}, err
	}

	cleanItems, files, err := splitInsertFileItems(schema, canvas, slug, items)
	if err != nil {
		return insertPlan{}, err
	}

	build := func(s *surgicalSession) (api.BatchOperation, error) {
		current, _, _, err := c.resolveInsert(s)
		if err != nil {
			return api.BatchOperation{}, err
		}
		after, err := insertAfter(current, c.After, c.Position)
		if err != nil {
			return api.BatchOperation{}, err
		}
		path := strings.TrimSuffix(current.RawPath, "/repeatables") + "/repeatables"
		return api.BatchOperation{
			Op:    "insert",
			Path:  path,
			Value: map[string]any{"slug": slug, "items": cleanItems},
			After: after,
		}, nil
	}
	op, err := build(session)
	if err != nil {
		return insertPlan{}, err
	}
	return insertPlan{
		insert: plannedOp{Human: c.Path, Op: op, rebuild: build},
		files:  files,
		slug:   slug,
		canvas: canvas,
	}, nil
}

func (c *PagesInsertCmd) resolveInsert(session *surgicalSession) (pagepath.Resolved, map[string]any, string, error) {
	items, slugFromFile, err := c.readItems()
	if err != nil {
		return pagepath.Resolved{}, nil, "", err
	}
	slug := c.Slug
	if slug == "" {
		slug = slugFromFile
	}
	if slug == "" {
		return pagepath.Resolved{}, nil, "", fmt.Errorf("--slug is required")
	}
	if slugFromFile != "" && slugFromFile != slug {
		return pagepath.Resolved{}, nil, "", fmt.Errorf("--slug %q does not match file slug %q", slug, slugFromFile)
	}

	input := strings.TrimSuffix(c.Path, "/repeatables")
	resolved, err := session.resolve(input)
	if err != nil {
		return pagepath.Resolved{}, nil, "", err
	}
	if err := requireResolvedKind(resolved, c.Path, pagepath.KindCanvas, canvasCandidates(session.doc)); err != nil {
		return pagepath.Resolved{}, nil, "", err
	}
	return resolved, items, slug, nil
}

func (c *PagesInsertCmd) readItems() (map[string]any, string, error) {
	if c.File == "" {
		return map[string]any{}, "", nil
	}
	raw, err := readJSONAnyInput(c.File)
	if err != nil {
		return nil, "", err
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("--file must be a JSON object")
	}
	if items, ok := object["items"].(map[string]any); ok {
		return items, stringAny(object["slug"]), nil
	}
	return object, "", nil
}

func validateInsertSlug(schema *pagepath.Schema, canvas, slug string) error {
	blocks := schema.Blocks(canvas)
	for _, block := range blocks {
		if block.Slug == slug {
			return nil
		}
	}
	var allowed []string
	for _, block := range blocks {
		label := block.Slug
		if block.Label != "" {
			label = block.Slug + " (" + block.Label + ")"
		}
		allowed = append(allowed, label)
	}
	if len(allowed) == 0 {
		return fmt.Errorf("slug %q is not allowed on canvas %s", slug, canvas)
	}
	return fmt.Errorf("slug %q is not allowed on canvas %s; allowed: %s", slug, canvas, strings.Join(allowed, ", "))
}

func insertAfter(resolved pagepath.Resolved, after string, position *int) (*api.Anchor, error) {
	if after != "" && position != nil {
		return nil, fmt.Errorf("--after and --position are mutually exclusive")
	}
	if position != nil {
		anchor, err := pagepath.AnchorForInsert(resolved.Siblings, *position)
		if err != nil {
			return nil, err
		}
		return batchAnchor(anchor), nil
	}
	if after == "" {
		return nil, nil
	}
	id, err := pagepath.ParseAnchor(after, resolved.Siblings)
	if err != nil {
		return nil, pathResolveError(err)
	}
	return &api.Anchor{Set: true, ID: id}, nil
}

func splitInsertFileItems(schema *pagepath.Schema, canvas, slug string, items map[string]any) (map[string]any, []plannedFileSet, error) {
	if len(items) == 0 {
		return map[string]any{}, nil, nil
	}
	clean := make(map[string]any, len(items))
	var files []plannedFileSet
	for name, value := range items {
		if schema.FieldType(canvas, slug, name) == "file" && isFileEditablePayload(value) {
			expanded, err := expandInsertFileValue(value)
			if err != nil {
				return nil, nil, err
			}
			files = append(files, plannedFileSet{Field: name, Value: expanded})
			continue
		}
		clean[name] = value
	}
	return clean, files, nil
}

func insertResultID(result *api.BatchResult) string {
	if result == nil || len(result.Results) == 0 {
		return ""
	}
	return result.Results[0].ID
}

func repeatableIndex(doc map[string]any, canvas, id string) int {
	node, _ := subtreeAt(doc, "/items/"+pagepath.EscapeName(canvas))
	for _, ref := range siblingRefs(asMapAny(node)) {
		if ref.ID == id {
			return ref.Index
		}
	}
	return 0
}

func asMapAny(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
