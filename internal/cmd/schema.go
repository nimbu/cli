package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type SchemaCmd struct {
	Pull  SchemaPullCmd  `cmd:"" help:"Pull live schemas into local files"`
	Plan  SchemaPlanCmd  `cmd:"" help:"Preview declarative schema changes"`
	Apply SchemaApplyCmd `cmd:"" help:"Apply declarative schema changes"`
}
type SchemaPlanCmd struct {
	Plans []schemaPlan `kong:"-"`
	Prune bool         `help:"Remove fields and options absent from supplied arrays"`
}
type SchemaApplyCmd struct {
	DisableRenamePrompts bool         `kong:"-"`
	Plans                []schemaPlan `kong:"-"`
	Prune                bool         `help:"Remove fields and options absent from supplied arrays"`
	Yes                  bool         `help:"Confirm schema changes, including destructive changes"`
}
type schemaDocument struct {
	Path, Target, Endpoint string
	Body                   map[string]any
}
type schemaPlan struct {
	Target      string           `json:"target"`
	Fingerprint string           `json:"fingerprint"`
	Exists      bool             `json:"exists"`
	Ops         []map[string]any `json:"ops"`
	Drift       []map[string]any `json:"drift"`
	Warnings    []string         `json:"warnings"`
	Info        []string         `json:"info"`
}

func loadSchemaDocuments(root string) ([]schemaDocument, error) {
	paths, err := filepath.Glob(filepath.Join(root, "schema", "*.yml"))
	if err != nil {
		return nil, err
	}
	for _, kind := range []string{"channels", "blogs", "checkout_profiles"} {
		found, e := filepath.Glob(filepath.Join(root, "schema", kind, "*.yml"))
		if e != nil {
			return nil, e
		}
		paths = append(paths, found...)
	}
	sort.Strings(paths)
	docs := make([]schemaDocument, 0, len(paths))
	for _, path := range paths {
		data, e := os.ReadFile(path)
		if e != nil {
			return nil, e
		}
		var body map[string]any
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		if e = decoder.Decode(&body); e != nil {
			return nil, fmt.Errorf("%s: %w", path, e)
		}
		var trailing any
		if e = decoder.Decode(&trailing); e != io.EOF {
			return nil, fmt.Errorf("%s: expected a single YAML document", path)
		}
		if _, ok := body["fields"].([]any); !ok {
			return nil, fmt.Errorf("%s: fields must be an explicit array", path)
		}
		for _, key := range []string{"prune", "fingerprint", "confirm_destructive"} {
			if _, ok := body[key]; ok {
				return nil, fmt.Errorf("%s: %s is a command control, not a schema attribute", path, key)
			}
		}
		name := strings.TrimSuffix(filepath.Base(path), ".yml")
		kind := filepath.Base(filepath.Dir(path))
		endpoint := ""
		target := ""
		switch kind {
		case "schema":
			if name != "products" && name != "customers" {
				return nil, fmt.Errorf("unsupported schema file %s", path)
			}
			endpoint = "/" + name + "/customizations"
			target = name
		case "channels", "blogs", "checkout_profiles":
			slug, ok := body["slug"].(string)
			if !ok || slug != name {
				return nil, fmt.Errorf("%s: slug must match filename %q", path, name)
			}
			endpoint = "/" + kind + "/" + url.PathEscape(slug)
			target = strings.TrimSuffix(kind, "s") + ":" + slug
			if kind == "checkout_profiles" {
				endpoint = "/products" + endpoint
			}
		}
		docs = append(docs, schemaDocument{Path: path, Target: target, Endpoint: endpoint, Body: body})
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no schema files found; run nimbu schema pull first")
	}
	return orderSchemaDocuments(docs), nil
}

func orderSchemaDocuments(docs []schemaDocument) []schemaDocument {
	bySlug := map[string]int{}
	for i, d := range docs {
		if strings.HasPrefix(d.Endpoint, "/channels/") {
			bySlug[fmt.Sprint(d.Body["slug"])] = i
		}
	}
	state := make([]int, len(docs))
	ordered := make([]schemaDocument, 0, len(docs))
	var visit func(int)
	visit = func(i int) {
		if state[i] != 0 {
			return
		}
		state[i] = 1
		for _, raw := range docs[i].Body["fields"].([]any) {
			f, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if reference := schemaChannelReference(f); reference != "" {
				if j, ok := bySlug[reference]; ok {
					visit(j)
				}
			}
		}
		state[i] = 2
		ordered = append(ordered, docs[i])
	}
	for i := range docs {
		visit(i)
	}
	return ordered
}

func schemaRequestBody(doc schemaDocument, prune bool) map[string]any {
	body := make(map[string]any, len(doc.Body)+3)
	for k, v := range doc.Body {
		body[k] = v
	}
	if prune {
		body["prune"] = true
	}
	return body
}

func fetchSchemaPlan(ctx context.Context, client *api.Client, doc schemaDocument, prune bool) (schemaPlan, error) {
	var plan schemaPlan
	// Planning uses POST but does not mutate server state.
	planner := *client
	planner.Readonly = false
	if err := planner.Post(ctx, doc.Endpoint+"/plan", schemaRequestBody(doc, prune), &plan); err != nil {
		return plan, fmt.Errorf("plan %s: %w", doc.Target, err)
	}
	if plan.Target == "" {
		plan.Target = doc.Target
	}
	if plan.Fingerprint == "" {
		return plan, fmt.Errorf("plan %s: server omitted fingerprint", doc.Target)
	}
	addRenameWarnings(&plan, doc)
	return plan, nil
}

var schemaOpKinds = map[string]bool{"create_target": true, "update_target": true, "add_field": true, "update_attr": true, "rename_field": true, "retype_field": true, "remove_field": true, "add_option": true, "update_option": true, "remove_option": true, "reorder_fields": true, "reorder_options": true, "toggle_localized": true}

func schemaPlanExit(plans []schemaPlan) int {
	code := 0
	for _, p := range plans {
		if schemaHasMissingReferences(p) && code == 0 {
			code = 2
		}
		for _, op := range p.Ops {
			kind, _ := op["kind"].(string)
			risk, _ := op["risk"].(string)
			if !schemaOpKinds[kind] || risk == "blocked" || (risk != "safe" && risk != "destructive") {
				return 1
			}
			if risk == "destructive" {
				code = 3
			} else if code == 0 {
				code = 2
			}
		}
	}
	return code
}

func addRenameWarnings(plan *schemaPlan, doc schemaDocument) {
	for _, raw := range doc.Body["fields"].([]any) {
		field, ok := raw.(map[string]any)
		if !ok || field["renamed_from"] != nil {
			continue
		}
		name, _ := field["name"].(string)
		typ, _ := field["type"].(string)
		if typ == "" {
			continue
		}
		added := false
		for _, op := range plan.Ops {
			if op["kind"] == "add_field" && op["field"] == name {
				added = true
			}
		}
		if !added {
			continue
		}
		for _, drift := range plan.Drift {
			if drift["type"] == typ {
				plan.Warnings = append(plan.Warnings, fmt.Sprintf("possible rename %s → %s; declare renamed_from in %s", drift["field"], name, doc.Path))
				plan.Ops = append(plan.Ops, map[string]any{"kind": "rename_field", "field": name, "from": drift["field"], "to": name, "risk": "blocked", "reason": "possible rename; declare renamed_from"})
			}
		}
	}
}

func renderSchemaPlans(ctx context.Context, plans []schemaPlan) error {
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, plans)
	}
	for _, p := range plans {
		if _, err := output.Fprintf(ctx, "%s: %d operation(s)\n", p.Target, len(p.Ops)); err != nil {
			return err
		}
		for _, op := range p.Ops {
			if _, err := output.Fprintf(ctx, "  %s %s %v\n", op["risk"], op["kind"], op); err != nil {
				return err
			}
		}
		for _, d := range p.Drift {
			_, _ = output.Fprintf(ctx, "  drift: %v\n", d)
		}
		for _, w := range p.Warnings {
			_, _ = output.Fprintf(ctx, "  warning: %s\n", w)
		}
		for _, info := range p.Info {
			_, _ = output.Fprintf(ctx, "  %s\n", info)
		}
	}
	return nil
}

func planSchemaDocuments(ctx context.Context, client *api.Client, docs []schemaDocument, prune bool) ([]schemaPlan, error) {
	plans := make([]schemaPlan, 0, len(docs))
	cyclic := schemaCycleTargets(docs)
	for _, doc := range docs {
		p, e := fetchSchemaPlan(ctx, client, doc, prune)
		if e != nil {
			return nil, e
		}
		if !p.Exists && strings.HasPrefix(doc.Endpoint, "/channels/") && cyclic[fmt.Sprint(doc.Body["slug"])] {
			p.Info = append(p.Info, "Circular references require a temporary channel field; without --prune it remains as drift until explicitly removed.")
		}
		plans = append(plans, p)
	}
	return plans, nil
}

func schemaCommandContext(ctx context.Context) (*api.Client, []schemaDocument, error) {
	project, err := resolveProjectContext()
	if err != nil {
		return nil, nil, err
	}
	docs, err := loadSchemaDocuments(project.ProjectRoot)
	if err != nil {
		return nil, nil, err
	}
	site, err := RequireSite(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	return client, docs, err
}

func (c *SchemaPlanCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, docs, err := schemaCommandContext(ctx)
	if err != nil {
		return err
	}
	plans, err := planSchemaDocuments(ctx, client, docs, c.Prune)
	if err != nil {
		return err
	}
	inventory, err := schemaTargetDrift(ctx, client, docs)
	if err != nil {
		return err
	}
	plans = append(plans, inventory...)
	c.Plans = plans
	if err = renderSchemaPlans(ctx, plans); err != nil {
		return err
	}
	if code := schemaPlanExit(plans); code != 0 {
		return &displayedError{err: &ExitError{Code: code}}
	}
	return nil
}
