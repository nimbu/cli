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
	item, err := copyChannelDetail(ctx, toClient, detail, toRef.Channel)
	if err != nil {
		return ChannelCopyResult{From: fromRef.SiteRef, To: toRef.SiteRef}, err
	}
	return ChannelCopyResult{From: fromRef.SiteRef, To: toRef.SiteRef, Items: []ChannelCopyItem{item}}, nil
}

// CopyAllChannels copies all channels from source to target.
func CopyAllChannels(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef) (ChannelCopyResult, error) {
	channels, err := api.ListChannelDetails(ctx, fromClient)
	if err != nil {
		return ChannelCopyResult{From: fromRef, To: toRef}, err
	}

	graph := api.BuildChannelDependencyGraph(channels)
	ordered := topoSortChannels(channels, graph)
	result := ChannelCopyResult{From: fromRef, To: toRef}
	for _, channel := range ordered {
		if graph.HasCircularDependencies(channel.Slug) {
			created, err := ensureCircularPlaceholder(ctx, toClient, channel.Slug)
			if err != nil {
				return result, err
			}
			if created {
				result.Placeholders = append(result.Placeholders, channel.Slug)
			}
		}
		item, err := copyChannelDetail(ctx, toClient, channel, channel.Slug)
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

func copyChannelDetail(ctx context.Context, client *api.Client, detail api.ChannelDetail, targetSlug string) (ChannelCopyItem, error) {
	payload := channelPayload(detail, targetSlug)
	path := "/channels/" + url.PathEscape(strings.TrimSpace(targetSlug))
	var existing api.ChannelDetail
	action := "create"
	err := client.Get(ctx, path, &existing)
	switch {
	case err == nil:
		action = "update"
		if err := client.Patch(ctx, path, payload, &existing, api.WithParam("replace", "1")); err != nil {
			return ChannelCopyItem{}, err
		}
	case api.IsNotFound(err):
		if err := client.Post(ctx, "/channels", payload, &existing); err != nil {
			return ChannelCopyItem{}, err
		}
	default:
		return ChannelCopyItem{}, err
	}
	return ChannelCopyItem{Source: detail.Slug, Target: targetSlug, Action: action}, nil
}

func ensureCircularPlaceholder(ctx context.Context, client *api.Client, slug string) (bool, error) {
	var existing api.ChannelDetail
	err := client.Get(ctx, "/channels/"+url.PathEscape(slug), &existing)
	if err == nil {
		return false, nil
	}
	if !api.IsNotFound(err) {
		return false, err
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

func channelPayload(detail api.ChannelDetail, targetSlug string) map[string]any {
	var payload map[string]any
	data, _ := json.Marshal(detail)
	_ = json.Unmarshal(data, &payload)
	delete(payload, "id")
	delete(payload, "created_at")
	delete(payload, "updated_at")
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
