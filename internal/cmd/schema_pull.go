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

// SchemaPullCmd exports the site's declarative schemas.
type SchemaPullCmd struct {
	Only []string `name:"only" help:"Targets to pull, e.g. channels/events,products"`
}

func (c *SchemaPullCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "pull schema"); err != nil {
		return err
	}
	project, err := resolveProjectContext()
	if err != nil {
		return err
	}
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	written, err := pullSchemas(ctx, client, project.ProjectRoot, splitRepeatedCSV(c.Only))
	if err != nil {
		return err
	}
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, map[string]any{"written": written})
	}
	for _, path := range written {
		if _, err := output.Fprintln(ctx, path); err != nil {
			return err
		}
	}
	return nil
}

func pullSchemas(ctx context.Context, client *api.Client, root string, only []string) ([]string, error) {
	for _, selector := range only {
		if !validSchemaPullSelector(selector) {
			return nil, fmt.Errorf("invalid schema target %q", selector)
		}
	}
	var targets []string
	for _, kind := range []string{"channels", "blogs", "checkout_profiles", "products", "customers"} {
		if !schemaPullIncludesKind(only, kind) {
			continue
		}
		if kind == "products" || kind == "customers" {
			targets = append(targets, kind)
			continue
		}
		// Exact selectors avoid enumerating unrelated targets and fail if the target is missing.
		if len(only) > 0 && !schemaPullHas(only, kind) {
			for _, selector := range only {
				if strings.HasPrefix(selector, kind+"/") {
					targets = append(targets, selector)
				}
			}
			continue
		}
		items, err := api.List[map[string]any](ctx, client, schemaPullBasePath(kind))
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", kind, err)
		}
		for _, item := range items {
			slug, _ := item["slug"].(string)
			if slug == "" {
				slug, _ = item["handle"].(string)
			}
			target := kind + "/" + slug
			if !validSchemaPullSelector(target) {
				return nil, fmt.Errorf("invalid %s identity %q", kind, slug)
			}
			targets = append(targets, target)
		}
	}
	sort.Strings(targets)
	// Fetch and encode everything before changing files, so a failed remote read leaves local files intact.
	documents := make(map[string][]byte, len(targets))
	for _, target := range targets {
		if _, exists := documents[target]; exists {
			continue
		}
		var document map[string]any
		parts := strings.SplitN(target, "/", 2)
		endpoint := schemaPullBasePath(parts[0])
		if len(parts) == 2 {
			endpoint += "/" + url.PathEscape(parts[1])
		}
		if err := client.Get(ctx, endpoint+"/schema", &document); err != nil {
			return nil, fmt.Errorf("pull %s: %w", target, err)
		}
		if err := validateSchemaPullDocument(document, parts); err != nil {
			return nil, fmt.Errorf("pull %s: %w", target, err)
		}
		document = normalizeSchemaPullDocument(document, parts[0])
		path := filepath.Join(root, "schema", filepath.FromSlash(target)+".yml")
		previous, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if len(previous) > 0 {
			var local map[string]any
			decoder := yaml.NewDecoder(bytes.NewReader(previous))
			if err := decoder.Decode(&local); err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			var trailing any
			if err := decoder.Decode(&trailing); err != io.EOF {
				return nil, fmt.Errorf("read %s: expected a single YAML document", path)
			}
			preserveSchemaPullExtensions(document, local)
		}
		data, err := yaml.Marshal(document)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", target, err)
		}
		documents[target] = data
	}
	written := make([]string, 0, len(documents))
	for _, target := range targets {
		data, exists := documents[target]
		if !exists {
			continue
		}
		path := filepath.Join(root, "schema", filepath.FromSlash(target)+".yml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return written, err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return written, err
		}
		written = append(written, path)
		delete(documents, target)
	}
	return written, nil
}

func validSchemaPullSelector(selector string) bool {
	parts := strings.Split(selector, "/")
	if len(parts) > 2 || !schemaPullHas([]string{"channels", "blogs", "checkout_profiles", "products", "customers"}, parts[0]) {
		return false
	}
	if len(parts) == 1 {
		return true
	}
	return parts[0] != "products" && parts[0] != "customers" && parts[1] != "" && parts[1] != "." && parts[1] != ".." && !strings.ContainsAny(parts[1], "\\\x00")
}

func schemaPullIncludesKind(only []string, kind string) bool {
	if len(only) == 0 {
		return true
	}
	for _, value := range only {
		if value == kind || strings.HasPrefix(value, kind+"/") {
			return true
		}
	}
	return false
}

func schemaPullHas(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

// Preserve local extensions, including transient rename hints, by stable identity.
func preserveSchemaPullExtensions(remote, local map[string]any) {
	for key, value := range local {
		if _, exists := remote[key]; !exists && !schemaPullKnownKey(key) {
			remote[key] = value
		}
	}
	for key, identity := range map[string]string{"fields": "name", "options": "slug"} {
		remoteItems, _ := remote[key].([]any)
		localItems, _ := local[key].([]any)
		for _, item := range remoteItems {
			current, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for _, previous := range localItems {
				old, ok := previous.(map[string]any)
				if ok && current[identity] == old[identity] {
					preserveSchemaPullExtensions(current, old)
					break
				}
			}
		}
	}
}

func schemaPullKnownKey(key string) bool {
	return schemaPullHas(strings.Fields(schemaPullFieldKeys+" "+schemaPullChannelKeys+" slug name description label_field title_field order_by order_direction submittable submittable_fields publishable sensitive rss acl locales seo_title seo_description seo_keywords private_fields fields label type hint required localized unique auto_expand immutable encrypted required_expression geo_type calculated_expression calculation_type private_storage text_formatting reference options select_options id position url entries_url created_at updated_at"), key)
}

func schemaPullBasePath(kind string) string {
	switch kind {
	case "products", "customers":
		return "/" + kind + "/customizations"
	case "checkout_profiles":
		return "/products/checkout_profiles"
	default:
		return "/" + kind
	}
}

const (
	schemaPullFieldKeys   = "name label type hint required localized unique auto_expand immutable encrypted required_expression geo_type calculated_expression calculation_type private_storage text_formatting reference options"
	schemaPullChannelKeys = "slug name description sensitive publishable label_field title_field order_by order_direction submittable submittable_fields submittable_html_fields submittable_notifications submittable_receivers submittable_notification_template submittable_confirmations submittable_confirmation_receivers submittable_confirmation_template spam_detection rss_enabled rss_title rss_description rss_title_field rss_description_field rss_image_field"
)

func normalizeSchemaPullDocument(raw map[string]any, kind string) map[string]any {
	keys := ""
	switch kind {
	case "channels":
		keys = schemaPullChannelKeys
	case "blogs":
		keys = "slug name description seo_title seo_description seo_keywords locales"
	case "checkout_profiles":
		keys = "slug name private_fields"
	}
	document := schemaPullAllow(raw, keys)
	fields := make([]any, 0)
	items, _ := raw["fields"].([]any)
	for _, value := range items {
		field, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if field["name"] == "_status" || field["name"] == "_publish_at" {
			continue
		}
		normalized := schemaPullAllow(field, schemaPullFieldKeys)
		if options, ok := field["options"].([]any); ok {
			normalizedOptions := make([]any, 0, len(options))
			for _, value := range options {
				if option, ok := value.(map[string]any); ok {
					normalizedOptions = append(normalizedOptions, schemaPullAllow(option, "name slug"))
				}
			}
			normalized["options"] = normalizedOptions
		}
		fields = append(fields, normalized)
	}
	document["fields"] = fields
	return document
}

func schemaPullAllow(raw map[string]any, keys string) map[string]any {
	result := make(map[string]any)
	for _, key := range strings.Fields(keys) {
		if value, ok := raw[key]; ok {
			result[key] = value
		}
	}
	return result
}

func validateSchemaPullDocument(document map[string]any, target []string) error {
	if len(target) == 2 && document["slug"] != target[1] {
		return fmt.Errorf("response slug must match target %q", target[1])
	}
	fields, ok := document["fields"].([]any)
	if !ok {
		return fmt.Errorf("response must contain a fields array")
	}
	names := make(map[string]bool, len(fields))
	for _, value := range fields {
		field, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("response fields must be objects")
		}
		name, _ := field["name"].(string)
		if strings.TrimSpace(name) == "" || names[name] {
			return fmt.Errorf("response field names must be present and unique")
		}
		names[name] = true
		if raw, exists := field["options"]; exists {
			options, ok := raw.([]any)
			if !ok {
				return fmt.Errorf("response options for %q must be an array", name)
			}
			slugs := make(map[string]bool, len(options))
			for _, value := range options {
				option, ok := value.(map[string]any)
				if !ok {
					return fmt.Errorf("response options for %q must be objects", name)
				}
				slug, _ := option["slug"].(string)
				if strings.TrimSpace(slug) == "" || slugs[slug] {
					return fmt.Errorf("response option slugs for %q must be present and unique", name)
				}
				slugs[slug] = true
			}
		}
	}
	return nil
}
