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
	Draft    bool   `help:"Write to the page draft instead of the live page"`
}

type insertPlan struct {
	insert    plannedOp
	files     []plannedFileSet
	slug      string
	canvas    string
	canvasRaw string
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
	write := surgicalWriteFlags{Locale: c.Locale, Diff: c.Diff, DryRun: c.DryRun, Draft: c.Draft}
	if err := session.applyWriteMode(write); err != nil {
		return err
	}
	before := cloneMap(session.doc)

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
					Path:  insertItemPath(plan.canvasRaw, "<new-id>", file.Field),
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
			fileOps = append(fileOps, plannedOp{
				Human: file.Field,
				Op:    api.BatchOperation{Op: "set", Path: insertItemPath(plan.canvasRaw, newID, file.Field), Value: file.Value},
			})
		}
		fileResult, err := session.runBatch(fileOps, write)
		if err != nil {
			return err
		}
		result = mergeBatchResults(insertResult, fileResult)
		ops = append(ops, fileOps...)
	}

	index := repeatableIndexAt(session.doc, plan.canvasRaw, insertResultID(insertResult))
	extra := fmt.Sprintf("Inserted %s at %s[%d] (id %s)", plan.slug, plan.canvas, index, insertResultID(insertResult))
	return printSurgicalResult(ctx, session, ops, result, before, write, extra)
}

func (c *PagesInsertCmd) plan(session *surgicalSession) (insertPlan, error) {
	resolved, items, slug, err := c.resolveInsert(session)
	if err != nil {
		return insertPlan{}, err
	}
	canvas := canvasNameFromRaw(resolved.RawPath)
	canvasRaw := strings.TrimSuffix(resolved.RawPath, "/repeatables")
	parentSlug := resolved.RepeatableSlug
	schema, err := session.ensureSchema()
	if err != nil {
		return insertPlan{}, err
	}
	seeded, err := prepareInsertItems(schema, canvas, parentSlug, slug, items)
	if err != nil {
		return insertPlan{}, err
	}

	cleanItems, files, err := splitInsertFileItems(session, schema, canvas, slug, seeded)
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
		insert:    plannedOp{Human: c.Path, Op: op, rebuild: build},
		files:     files,
		slug:      slug,
		canvas:    canvas,
		canvasRaw: canvasRaw,
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

func validateInsertSlug(schema *pagepath.Schema, canvas, parentSlug, slug string) error {
	blocks := schema.BlocksFor(canvas, parentSlug)
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

func prepareInsertItems(schema *pagepath.Schema, canvas, parentSlug, slug string, items map[string]any) (map[string]any, error) {
	blocks := schema.BlocksFor(canvas, parentSlug)
	if len(blocks) == 0 {
		return items, nil
	}
	if err := validateInsertSlug(schema, canvas, parentSlug, slug); err != nil {
		return nil, err
	}
	var fields []pagepath.FieldDef
	for _, block := range blocks {
		if block.Slug == slug {
			fields = block.Fields
			break
		}
	}
	if len(fields) == 0 {
		return items, nil
	}
	allowed := make(map[string]struct{}, len(fields))
	var listed []string
	for _, field := range fields {
		allowed[field.Slug] = struct{}{}
		label := field.Slug
		if field.Type != "" {
			label = field.Slug + " (" + field.Type + ")"
		}
		listed = append(listed, label)
	}
	for name := range items {
		if _, ok := allowed[name]; !ok {
			return nil, fmt.Errorf("editable %q is not part of block %s; editables: %s", name, slug, strings.Join(listed, ", "))
		}
	}
	seeded := make(map[string]any, len(fields))
	for _, field := range fields {
		if value, ok := items[field.Slug]; ok {
			seeded[field.Slug] = value
			continue
		}
		seeded[field.Slug] = nil
	}
	return seeded, nil
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

func splitInsertFileItems(session *surgicalSession, schema *pagepath.Schema, canvas, slug string, items map[string]any) (map[string]any, []plannedFileSet, error) {
	if len(items) == 0 {
		return map[string]any{}, nil, nil
	}
	clean := make(map[string]any, len(items))
	var files []plannedFileSet
	for name, value := range items {
		if value != nil && schema.FieldType(canvas, slug, name) == "file" && isFileEditablePayload(value) {
			expanded, err := expandInsertFileValue(session, value)
			if err != nil {
				return nil, nil, err
			}
			files = append(files, plannedFileSet{Field: name, Value: expanded})
			clean[name] = nil
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

func insertItemPath(canvasRaw, repeatableID, field string) string {
	base := strings.TrimSuffix(canvasRaw, "/repeatables")
	return base + "/repeatables/" + repeatableID + "/items/" + pagepath.EscapeName(field)
}

func repeatableIndexAt(doc map[string]any, canvasRaw, id string) int {
	node, _ := subtreeAt(doc, strings.TrimSuffix(canvasRaw, "/repeatables"))
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
