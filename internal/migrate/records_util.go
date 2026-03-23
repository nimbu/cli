package migrate

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strings"

	cryptorand "crypto/rand"

	"github.com/nimbu/cli/internal/api"
)

func (c *recordCopier) prepareAttachments(ctx context.Context, payload map[string]any, info schemaInfo) error {
	for _, field := range info.fileFields {
		raw := payload[field.Name]
		if raw == nil {
			continue
		}
		file, ok := raw.(map[string]any)
		if !ok {
			emitWarning(ctx, fmt.Sprintf("%s.%s: unexpected file format %T, skipping attachment", info.resource, field.Name, raw))
			continue
		}
		if err := c.embedFile(ctx, file); err != nil {
			return err
		}
	}
	for _, field := range info.galleryFields {
		raw := payload[field.Name]
		if raw == nil {
			continue
		}
		gallery, ok := raw.(map[string]any)
		if !ok {
			emitWarning(ctx, fmt.Sprintf("%s.%s: unexpected gallery format %T, skipping", info.resource, field.Name, raw))
			continue
		}
		gallery["__type"] = "Gallery"
		images, ok := gallery["images"].([]any)
		if !ok {
			continue
		}
		for _, rawImage := range images {
			image, ok := rawImage.(map[string]any)
			if !ok {
				continue
			}
			image["__type"] = "GalleryImage"
			delete(image, "id")
			file, ok := image["file"].(map[string]any)
			if !ok {
				continue
			}
			if err := c.embedFile(ctx, file); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *recordCopier) embedFile(ctx context.Context, file map[string]any) error {
	return embedFileFromClient(ctx, c.fromClient, file)
}

// embedFileFromClient downloads a file and encodes it as a base64 attachment.
func embedFileFromClient(ctx context.Context, client *api.Client, file map[string]any) error {
	if _, ok := file["attachment"]; ok {
		return nil
	}
	rawURL := stringValue(file["url"])
	if rawURL == "" {
		rawURL = stringValue(file["public_url"])
	}
	if rawURL == "" {
		return nil
	}
	data, err := downloadBinary(ctx, client, rawURL)
	if err != nil {
		return err
	}
	file["attachment"] = base64.StdEncoding.EncodeToString(data)
	file["__type"] = "File"
	delete(file, "url")
	delete(file, "public_url")
	// Strip output-only metadata from rich API responses
	delete(file, "permanent_url")
	delete(file, "size")
	delete(file, "width")
	delete(file, "height")
	delete(file, "checksum")
	delete(file, "version")
	delete(file, "private")
	return nil
}

func downloadBinary(ctx context.Context, client *api.Client, rawURL string) ([]byte, error) {
	resp, resolvedURL, err := openDownloadResponse(ctx, client, rawURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download %s: HTTP %d", resolvedURL, resp.StatusCode)
	}
	if resp.ContentLength > maxRecordAttachmentBytes {
		return nil, fmt.Errorf("download %s: attachment exceeds %d-byte limit", resolvedURL, maxRecordAttachmentBytes)
	}

	limited := &io.LimitedReader{R: resp.Body, N: maxRecordAttachmentBytes + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxRecordAttachmentBytes {
		return nil, fmt.Errorf("download %s: attachment exceeds %d-byte limit", resolvedURL, maxRecordAttachmentBytes)
	}
	return data, nil
}

func (c *recordCopier) ensureMapping(channel string) map[string]string {
	if c.mapping[channel] == nil {
		c.mapping[channel] = map[string]string{}
	}
	return c.mapping[channel]
}

func (c *recordCopier) queueRequest(channel string, ids map[string]struct{}) bool {
	if c.queued[channel] == nil {
		c.queued[channel] = map[string]struct{}{}
	}
	if len(ids) == 0 {
		if _, ok := c.queued[channel]["*"]; ok {
			return false
		}
		c.queued[channel]["*"] = struct{}{}
		return true
	}
	var fresh bool
	for id := range ids {
		if _, ok := c.queued[channel][id]; ok {
			continue
		}
		c.queued[channel][id] = struct{}{}
		fresh = true
	}
	return fresh
}

func (c *recordCopier) lookupMappedID(channel string, sourceID string) (string, bool) {
	if strings.TrimSpace(sourceID) == "" {
		return "", false
	}
	targetID, ok := c.ensureMapping(channel)[sourceID]
	return targetID, ok && strings.TrimSpace(targetID) != ""
}

func buildSchemaInfo(resource string, fields []api.CustomField) schemaInfo {
	info := schemaInfo{resource: resource}
	for _, field := range fields {
		switch field.Type {
		case "belongs_to", "belongs_to_many", "customer":
			info.referenceFields = append(info.referenceFields, field)
			if referenceClass(field) == resource {
				info.selfRefs = append(info.selfRefs, field)
			}
			if referenceClass(field) == "customers" {
				info.customerFields = append(info.customerFields, field)
			}
		case "file":
			info.fileFields = append(info.fileFields, field)
		case "gallery":
			info.galleryFields = append(info.galleryFields, field)
		case "select":
			info.selectFields = append(info.selectFields, field)
		case "multi_select":
			info.multiFields = append(info.multiFields, field)
		}
	}
	return info
}

func buildRecordQuery(opts RecordCopyOptions, ids map[string]struct{}) (map[string]string, error) {
	if strings.TrimSpace(opts.Query) != "" && strings.TrimSpace(opts.Where) != "" {
		return nil, fmt.Errorf("--query and --where cannot be combined")
	}
	params := map[string]string{}
	if len(ids) > 0 {
		parts := make([]string, 0, len(ids))
		for id := range ids {
			parts = append(parts, fmt.Sprintf(`id:"%s"`, escapeWhereValue(id)))
		}
		sort.Strings(parts)
		params["where"] = strings.Join(parts, " OR ")
		return params, nil
	}
	if strings.TrimSpace(opts.Where) != "" {
		params["where"] = opts.Where
	}
	if strings.TrimSpace(opts.Query) != "" {
		values, err := url.ParseQuery(strings.ReplaceAll(opts.Query, "?", "&"))
		if err != nil {
			return nil, fmt.Errorf("parse query: %w", err)
		}
		for key, value := range values {
			if len(value) > 0 {
				params[key] = value[len(value)-1]
			}
		}
	}
	return params, nil
}

func flattenSelectFields(payload map[string]any, info schemaInfo) {
	for _, field := range info.selectFields {
		if value, ok := payload[field.Name].(map[string]any); ok {
			if flattened := value["value"]; flattened != nil {
				payload[field.Name] = flattened
			}
		}
	}
	for _, field := range info.multiFields {
		if value, ok := payload[field.Name].(map[string]any); ok {
			if flattened := value["values"]; flattened != nil {
				payload[field.Name] = flattened
			}
		}
	}
}

func parseUpsertFields(raw string, target string) []string {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	var fields []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, ":") {
			items := strings.SplitN(part, ":", 2)
			if strings.TrimSpace(items[0]) != target {
				continue
			}
			part = items[1]
		}
		fields = append(fields, part)
	}
	return fields
}

func deepCopyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = deepCopyValue(value)
	}
	return out
}

func deepCopyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return deepCopyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = deepCopyValue(item)
		}
		return out
	default:
		return typed
	}
}

func stripSystemFields(payload map[string]any) {
	for _, key := range []string{"id", "_id", "created_at", "updated_at"} {
		delete(payload, key)
	}
}

func ensureCustomerPassword(payload map[string]any, length int) error {
	if payload["password"] != nil || payload["password_confirmation"] != nil {
		return nil
	}
	password, err := randomPassword(max(length, 8))
	if err != nil {
		return err
	}
	payload["password"] = password
	payload["password_confirmation"] = password
	return nil
}

func recordIdentifier(payload map[string]any) string {
	for _, key := range []string{"title_field_value", "email", "slug", "name", "title"} {
		if value := stringValue(payload[key]); value != "" {
			return value
		}
	}
	return "<unknown>"
}

func referenceClass(field api.CustomField) string {
	if field.Type == "customer" {
		return "customers"
	}
	return field.Reference
}

func isRecoverableRecordError(err error) bool {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.IsValidation() || apiErr.StatusCode == http.StatusBadRequest
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func lookupValue(payload map[string]any, path string) string {
	current := any(payload)
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[part]
	}
	switch typed := current.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func escapeWhereValue(value string) string {
	return strings.ReplaceAll(value, `"`, `\"`)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

// countActions counts synced (create/update) and skipped items.
func countActions(items []RecordCopyItem) (synced, skipped int) {
	for _, item := range items {
		switch item.Action {
		case "skip":
			skipped++
		case "create", "update":
			synced++
		}
	}
	return synced, skipped
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func randomPassword(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*_-+="
	buf := make([]byte, length)
	maxIndex := big.NewInt(int64(len(alphabet)))
	for i := range buf {
		n, err := cryptorand.Int(passwordRandReader, maxIndex)
		if err != nil {
			return "", fmt.Errorf("generate customer password: %w", err)
		}
		buf[i] = alphabet[n.Int64()]
	}
	return string(buf), nil
}
