package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

// ChannelCopyItem describes one copied channel.
type ChannelCopyItem struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Action string `json:"action"`
}

// ChannelCopyResult reports one or many copied channels.
type ChannelCopyResult struct {
	From         SiteRef           `json:"from"`
	To           SiteRef           `json:"to"`
	Items        []ChannelCopyItem `json:"items,omitempty"`
	Placeholders []string          `json:"placeholders,omitempty"`
}

// ChannelDiffResult reports channel-level and field-level differences.
type ChannelDiffResult struct {
	From        ChannelRef `json:"from"`
	To          ChannelRef `json:"to"`
	ChannelDiff DiffSet    `json:"channel_diff"`
	FieldsDiff  DiffSet    `json:"fields_diff"`
}

// ChannelInfoResult exposes one channel plus dependency metadata.
type ChannelInfoResult struct {
	Channel      api.ChannelDetail `json:"channel"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Dependants   []string          `json:"dependants,omitempty"`
	Circular     bool              `json:"circular"`
	TypeScript   string            `json:"typescript,omitempty"`
}

// CopyChannel copies one channel definition.
func CopyChannel(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef ChannelRef) (ChannelCopyResult, error) {
	detail, err := api.GetChannelDetail(ctx, fromClient, fromRef.Channel)
	if err != nil {
		return ChannelCopyResult{From: fromRef.SiteRef, To: toRef.SiteRef}, err
	}
	item, err := copyChannelDetail(ctx, toClient, detail, toRef.Channel, false)
	if err != nil {
		return ChannelCopyResult{From: fromRef.SiteRef, To: toRef.SiteRef}, err
	}
	return ChannelCopyResult{From: fromRef.SiteRef, To: toRef.SiteRef, Items: []ChannelCopyItem{item}}, nil
}

// CopyAllChannels copies all channels from source to target.
func CopyAllChannels(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, dryRun bool) (ChannelCopyResult, error) {
	channels, err := api.ListChannelDetails(ctx, fromClient)
	if err != nil {
		return ChannelCopyResult{From: fromRef, To: toRef}, err
	}

	graph := api.BuildChannelDependencyGraph(channels)
	ordered := topoSortChannels(channels, graph)
	result := ChannelCopyResult{From: fromRef, To: toRef}
	total := len(ordered)
	for i, channel := range ordered {
		emitStageItem(ctx, "Channels", channel.Slug, int64(i+1), int64(total))
		if graph.HasCircularDependencies(channel.Slug) {
			created, err := ensureCircularPlaceholder(ctx, toClient, channel.Slug, dryRun)
			if err != nil {
				return result, err
			}
			if created {
				result.Placeholders = append(result.Placeholders, channel.Slug)
			}
		}
		item, err := copyChannelDetail(ctx, toClient, channel, channel.Slug, dryRun)
		if err != nil {
			return result, fmt.Errorf("copy channel %s: %w", channel.Slug, err)
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

// DiffChannel compares two channel definitions after normalization.
func DiffChannel(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef ChannelRef) (ChannelDiffResult, error) {
	fromDetail, err := api.GetChannelDetail(ctx, fromClient, fromRef.Channel)
	if err != nil {
		return ChannelDiffResult{From: fromRef, To: toRef}, err
	}
	toDetail, err := api.GetChannelDetail(ctx, toClient, toRef.Channel)
	if err != nil {
		return ChannelDiffResult{From: fromRef, To: toRef}, err
	}
	return ChannelDiffResult{
		From:        fromRef,
		To:          toRef,
		ChannelDiff: DiffNormalized(NormalizeChannel(fromDetail), NormalizeChannel(toDetail)),
		FieldsDiff:  DiffNormalized(NormalizeCustomizations(fromDetail.Customizations), NormalizeCustomizations(toDetail.Customizations)),
	}, nil
}

// ChannelInfo loads a channel plus dependency metadata and TS output.
func ChannelInfo(ctx context.Context, client *api.Client, ref ChannelRef) (ChannelInfoResult, error) {
	detail, err := api.GetChannelDetail(ctx, client, ref.Channel)
	if err != nil {
		return ChannelInfoResult{}, err
	}
	all, err := api.ListChannelDetails(ctx, client)
	if err != nil {
		return ChannelInfoResult{Channel: detail}, err
	}
	graph := api.BuildChannelDependencyGraph(all)
	return ChannelInfoResult{
		Channel:      detail,
		Dependencies: graph.DirectDependencies(detail.Slug),
		Dependants:   graph.DirectDependants(detail.Slug),
		Circular:     graph.HasCircularDependencies(detail.Slug),
		TypeScript:   ChannelTypeScript(detail),
	}, nil
}

// ChannelTypeScript renders a TypeScript interface for one channel schema.
func ChannelTypeScript(detail api.ChannelDetail) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("export interface %s {", toPascalCase(detail.Slug)))
	for _, field := range detail.Customizations {
		lines = append(lines, fmt.Sprintf("  %s%s: %s;", field.Name, optionalSuffix(field.Required), fieldTypeScript(field)))
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}

func copyChannelDetail(ctx context.Context, client *api.Client, detail api.ChannelDetail, targetSlug string, dryRun bool) (ChannelCopyItem, error) {
	wasSubmittable := detail.Submittable
	sourceFieldIDs := detail.SubmittableFieldIDs

	payload := channelPayload(detail, targetSlug)
	path := "/channels/" + url.PathEscape(strings.TrimSpace(targetSlug))
	var existing api.ChannelDetail
	action := "create"
	err := client.Get(ctx, path, &existing)
	switch {
	case err == nil:
		action = "update"
		payload["customizations"] = mergeCustomizations(detail.Customizations, existing.Customizations)
		if dryRun {
			action = "dry-run:" + action
		} else if err := client.Patch(ctx, path, payload, &existing); err != nil {
			return ChannelCopyItem{}, err
		}
	case api.IsNotFound(err):
		if dryRun {
			action = "dry-run:" + action
		} else if err := client.Post(ctx, "/channels", payload, &existing); err != nil {
			return ChannelCopyItem{}, err
		}
	default:
		return ChannelCopyItem{}, err
	}

	if wasSubmittable && !dryRun && len(sourceFieldIDs) > 0 {
		if err := enableSubmittable(ctx, client, detail, targetSlug, sourceFieldIDs); err != nil {
			return ChannelCopyItem{}, err
		}
	}

	return ChannelCopyItem{Source: detail.Slug, Target: targetSlug, Action: action}, nil
}

// enableSubmittable maps source submittable_field_ids to target field IDs by name
// and enables submittable on the target channel.
func enableSubmittable(ctx context.Context, client *api.Client, source api.ChannelDetail, targetSlug string, sourceFieldIDs []string) error {
	// Build source field ID → name mapping
	sourceIDToName := make(map[string]string, len(source.Customizations))
	for _, f := range source.Customizations {
		if f.ID != "" && f.Name != "" {
			sourceIDToName[f.ID] = f.Name
		}
	}

	// Read back target channel to get new field IDs
	target, err := api.GetChannelDetail(ctx, client, targetSlug)
	if err != nil {
		return fmt.Errorf("read back channel %s for submittable setup: %w", targetSlug, err)
	}

	// Build target field name → ID mapping
	targetNameToID := make(map[string]string, len(target.Customizations))
	for _, f := range target.Customizations {
		if f.ID != "" && f.Name != "" {
			targetNameToID[f.Name] = f.ID
		}
	}

	// Map source submittable_field_ids → target field IDs
	var mappedIDs []string
	for _, srcID := range sourceFieldIDs {
		name := sourceIDToName[srcID]
		if name == "" {
			continue
		}
		if targetID := targetNameToID[name]; targetID != "" {
			mappedIDs = append(mappedIDs, targetID)
		}
	}

	if len(mappedIDs) == 0 {
		return nil
	}

	path := "/channels/" + url.PathEscape(strings.TrimSpace(targetSlug))
	update := map[string]any{
		"submittable":           true,
		"submittable_field_ids": mappedIDs,
	}
	var updated api.ChannelDetail
	return client.Patch(ctx, path, update, &updated)
}

func ensureCircularPlaceholder(ctx context.Context, client *api.Client, slug string, dryRun bool) (bool, error) {
	var existing api.ChannelDetail
	err := client.Get(ctx, "/channels/"+url.PathEscape(slug), &existing)
	if err == nil {
		return false, nil
	}
	if !api.IsNotFound(err) {
		return false, err
	}
	if dryRun {
		return true, nil
	}
	payload := map[string]any{
		"name": slug,
		"slug": slug,
		"customizations": []map[string]any{
			{
				"name":  "dummy",
				"label": "Dummy Field for Circular Dependencies",
				"type":  "string",
			},
		},
	}
	return true, client.Post(ctx, "/channels", payload, &existing)
}

// mergeCustomizations builds a customizations payload that preserves target field/option IDs.
// Source fields are sent without IDs (server matches by name). Target-only fields get _destroy markers.
func mergeCustomizations(source, target []api.CustomField) []map[string]any {
	targetByName := make(map[string]api.CustomField, len(target))
	for _, f := range target {
		if f.Name != "" {
			targetByName[f.Name] = f
		}
	}

	sourceNames := make(map[string]bool, len(source))
	items := make([]map[string]any, 0, len(source)+len(target))
	for _, src := range source {
		sourceNames[src.Name] = true
		raw := normalizeField(src)
		if tgt, ok := targetByName[src.Name]; ok {
			mergeSelectOptions(raw, src, tgt)
		}
		items = append(items, raw)
	}

	for _, tgt := range target {
		if !sourceNames[tgt.Name] && tgt.ID != "" {
			items = append(items, map[string]any{"id": tgt.ID, "_destroy": true})
		}
	}
	return items
}

// mergeSelectOptions adds _destroy markers for target-only select options.
func mergeSelectOptions(fieldMap map[string]any, src, tgt api.CustomField) {
	sourceSlugs := make(map[string]bool, len(src.SelectOptions))
	for _, opt := range src.SelectOptions {
		if opt.Slug != "" {
			sourceSlugs[opt.Slug] = true
		}
	}

	options, _ := fieldMap["select_options"].([]any)
	for _, tgtOpt := range tgt.SelectOptions {
		if !sourceSlugs[tgtOpt.Slug] && tgtOpt.ID != "" {
			options = append(options, map[string]any{"id": tgtOpt.ID, "_destroy": true})
		}
	}
	if len(options) > 0 {
		fieldMap["select_options"] = options
	}
}

func channelPayload(detail api.ChannelDetail, targetSlug string) map[string]any {
	var payload map[string]any
	data, _ := json.Marshal(detail)
	_ = json.Unmarshal(data, &payload)
	delete(payload, "id")
	delete(payload, "created_at")
	delete(payload, "updated_at")
	delete(payload, "submittable")
	delete(payload, "submittable_field_ids")
	payload["slug"] = targetSlug
	payload["customizations"] = NormalizeCustomizations(detail.Customizations)
	return payload
}

func topoSortChannels(channels []api.ChannelDetail, graph api.ChannelDependencyGraph) []api.ChannelDetail {
	bySlug := make(map[string]api.ChannelDetail, len(channels))
	for _, channel := range channels {
		bySlug[channel.Slug] = channel
	}
	slugs := make([]string, 0, len(channels))
	for _, channel := range channels {
		slugs = append(slugs, channel.Slug)
	}
	sort.Strings(slugs)
	var out []api.ChannelDetail
	seen := map[string]bool{}
	var visit func(string)
	visit = func(slug string) {
		if seen[slug] {
			return
		}
		seen[slug] = true
		for _, dep := range graph.DirectDependencies(slug) {
			visit(dep)
		}
		if channel, ok := bySlug[slug]; ok {
			out = append(out, channel)
		}
	}
	for _, slug := range slugs {
		visit(slug)
	}
	return out
}

func fieldTypeScript(field api.CustomField) string {
	switch field.Type {
	case "belongs_to", "customer":
		return "Nimbu.ReferenceTo"
	case "belongs_to_many":
		return "Nimbu.ReferenceMany"
	case "boolean":
		return "boolean"
	case "calculated", "email", "string", "text":
		return "string"
	case "date":
		return "Nimbu.Date"
	case "date_time", "time":
		return "Nimbu.DateTime"
	case "file":
		return "Nimbu.File"
	case "float", "integer":
		return "number"
	case "gallery":
		return "Nimbu.Gallery"
	case "multi_select":
		return fmt.Sprintf("Nimbu.MultiSelect<'%s'>", strings.Join(fieldOptionNames(field), "' | '"))
	case "select":
		return fmt.Sprintf("Nimbu.Select<'%s'>", strings.Join(fieldOptionNames(field), "' | '"))
	default:
		return "any"
	}
}

func fieldOptionNames(field api.CustomField) []string {
	names := make([]string, 0, len(field.SelectOptions))
	for _, option := range field.SelectOptions {
		if strings.TrimSpace(option.Name) != "" {
			names = append(names, option.Name)
		}
	}
	if len(names) == 0 {
		return []string{"string"}
	}
	return names
}

func toPascalCase(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	for idx, part := range parts {
		if part == "" {
			continue
		}
		parts[idx] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

func optionalSuffix(required bool) string {
	if required {
		return ""
	}
	return "?"
}
