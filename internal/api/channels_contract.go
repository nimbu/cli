package api

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
)

// ChannelDetail is the canonical rich channel contract.
type ChannelDetail struct {
	ID                  string         `json:"id"`
	Slug                string         `json:"slug"`
	Name                string         `json:"name"`
	Description         string         `json:"description,omitempty"`
	ACL                 map[string]any `json:"acl,omitempty"`
	Customizations      []CustomField  `json:"customizations,omitempty"`
	EntriesURL          string         `json:"entries_url,omitempty"`
	LabelField          string         `json:"label_field,omitempty"`
	TitleField          string         `json:"title_field,omitempty"`
	OrderBy             string         `json:"order_by,omitempty"`
	OrderDirection      string         `json:"order_direction,omitempty"`
	RSSEnabled          bool           `json:"rss_enabled,omitempty"`
	Submittable         bool           `json:"submittable,omitempty"`
	SubmittableFieldIDs []string       `json:"submittable_field_ids,omitempty"`
	URL                 string         `json:"url,omitempty"`
}

// SelectOption represents a select or multi-select option in a channel customization.
type SelectOption struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Position int    `json:"position,omitempty"`
	Slug     string `json:"slug,omitempty"`
}

// CustomField represents a channel customization field.
type CustomField struct {
	ID                   string         `json:"id,omitempty"`
	Name                 string         `json:"name,omitempty"`
	Label                string         `json:"label,omitempty"`
	Type                 string         `json:"type,omitempty"`
	Required             bool           `json:"required,omitempty"`
	RequiredExpression   string         `json:"required_expression,omitempty"`
	Unique               bool           `json:"unique,omitempty"`
	Localized            bool           `json:"localized,omitempty"`
	Encrypted            bool           `json:"encrypted,omitempty"`
	Hint                 string         `json:"hint,omitempty"`
	Reference            string         `json:"reference,omitempty"`
	SelectOptions        []SelectOption `json:"select_options,omitempty"`
	GeoType              string         `json:"geo_type,omitempty"`
	CalculatedExpression string         `json:"calculated_expression,omitempty"`
	CalculationType      string         `json:"calculation_type,omitempty"`
	PrivateStorage       bool           `json:"private_storage,omitempty"`
	Extra                map[string]any `json:"-"`
}

// GetChannelDetail fetches the canonical channel detail contract.
func GetChannelDetail(ctx context.Context, c *Client, slug string, opts ...RequestOption) (ChannelDetail, error) {
	var detail ChannelDetail
	if err := c.Get(ctx, "/channels/"+url.PathEscape(strings.TrimSpace(slug)), &detail, opts...); err != nil {
		return ChannelDetail{}, err
	}
	return detail, nil
}

// ListChannelDetails fetches all channels with their richer schema fields.
func ListChannelDetails(ctx context.Context, c *Client, opts ...RequestOption) ([]ChannelDetail, error) {
	return List[ChannelDetail](ctx, c, "/channels", opts...)
}

// IsRelational reports whether the field references another channel.
func (f CustomField) IsRelational() bool {
	switch f.Type {
	case "belongs_to", "belongs_to_many":
		return true
	default:
		return false
	}
}

// MarshalJSON emits known fields plus any Extra attributes preserved during unmarshal.
func (f CustomField) MarshalJSON() ([]byte, error) {
	type alias CustomField
	data, err := json.Marshal(alias(f))
	if err != nil {
		return nil, err
	}
	if len(f.Extra) == 0 {
		return data, nil
	}
	var merged map[string]any
	if err := json.Unmarshal(data, &merged); err != nil {
		return nil, err
	}
	for k, v := range f.Extra {
		merged[k] = v
	}
	return json.Marshal(merged)
}

// UnmarshalJSON preserves unknown field attributes while decoding the known customization fields.
func (f *CustomField) UnmarshalJSON(data []byte) error {
	type alias CustomField
	var known alias
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	for _, key := range []string{
		"id",
		"name",
		"label",
		"type",
		"required",
		"required_expression",
		"unique",
		"localized",
		"encrypted",
		"hint",
		"reference",
		"select_options",
		"geo_type",
		"calculated_expression",
		"calculation_type",
		"private_storage",
	} {
		delete(raw, key)
	}

	*f = CustomField(known)
	if len(raw) > 0 {
		f.Extra = raw
	}
	return nil
}

// ChannelDependencyGraph captures dependency and dependant relationships between channels.
type ChannelDependencyGraph struct {
	directDependencies map[string][]string
	directDependants   map[string][]string
}

// BuildChannelDependencyGraph constructs dependency relationships from relational custom fields.
func BuildChannelDependencyGraph(channels []ChannelDetail) ChannelDependencyGraph {
	graph := ChannelDependencyGraph{
		directDependencies: map[string][]string{},
		directDependants:   map[string][]string{},
	}

	known := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		if channel.Slug == "" {
			continue
		}
		known[channel.Slug] = struct{}{}
		graph.directDependencies[channel.Slug] = nil
		graph.directDependants[channel.Slug] = nil
	}

	for _, channel := range channels {
		if channel.Slug == "" {
			continue
		}
		seen := map[string]struct{}{}
		for _, field := range channel.Customizations {
			if !field.IsRelational() || field.Reference == "" {
				continue
			}
			if _, ok := known[field.Reference]; !ok {
				continue
			}
			if _, ok := seen[field.Reference]; ok {
				continue
			}
			seen[field.Reference] = struct{}{}
			graph.directDependencies[channel.Slug] = append(graph.directDependencies[channel.Slug], field.Reference)
			graph.directDependants[field.Reference] = append(graph.directDependants[field.Reference], channel.Slug)
		}
	}

	for slug := range graph.directDependencies {
		sort.Strings(graph.directDependencies[slug])
	}
	for slug := range graph.directDependants {
		sort.Strings(graph.directDependants[slug])
	}

	return graph
}

// DirectDependencies returns the direct dependencies for a channel.
func (g ChannelDependencyGraph) DirectDependencies(slug string) []string {
	return cloneStrings(g.directDependencies[slug])
}

// DirectDependants returns the direct dependants for a channel.
func (g ChannelDependencyGraph) DirectDependants(slug string) []string {
	return cloneStrings(g.directDependants[slug])
}

// TransitiveDependencies returns all dependency descendants for a channel.
func (g ChannelDependencyGraph) TransitiveDependencies(slug string) []string {
	return g.walk(g.directDependencies, slug)
}

// TransitiveDependants returns all dependant descendants for a channel.
func (g ChannelDependencyGraph) TransitiveDependants(slug string) []string {
	return g.walk(g.directDependants, slug)
}

// HasCircularDependencies reports whether a channel participates in a cycle.
func (g ChannelDependencyGraph) HasCircularDependencies(slug string) bool {
	return g.pathExists(slug, slug, g.directDependencies, map[string]struct{}{})
}

func (g ChannelDependencyGraph) walk(edges map[string][]string, slug string) []string {
	seen := map[string]struct{}{}
	var visit func(string)
	visit = func(node string) {
		for _, next := range edges[node] {
			if _, ok := seen[next]; ok {
				continue
			}
			seen[next] = struct{}{}
			visit(next)
		}
	}
	visit(slug)
	delete(seen, slug)
	out := make([]string, 0, len(seen))
	for item := range seen {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func (g ChannelDependencyGraph) pathExists(origin, current string, edges map[string][]string, seen map[string]struct{}) bool {
	for _, next := range edges[current] {
		if next == origin {
			return true
		}
		if _, ok := seen[next]; ok {
			continue
		}
		seen[next] = struct{}{}
		if g.pathExists(origin, next, edges, seen) {
			return true
		}
	}
	return false
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
