package migrate

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

// RecordCopyOptions controls entry/customer copy behavior.
type RecordCopyOptions struct {
	AllowErrors    bool
	CopyCustomers  bool
	DryRun         bool
	Media          *MediaRewritePlan
	Only           []string
	PasswordLength int
	PerPage        int
	Query          string
	Recursive      bool
	Upsert         string
	Where          string
}

// RecordCopyItem captures one copied record.
type RecordCopyItem struct {
	Action     string `json:"action"`
	Identifier string `json:"identifier"`
	Resource   string `json:"resource"`
	SourceID   string `json:"source_id,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
}

// RecordCopyResult reports raw record copy output.
type RecordCopyResult struct {
	From     SiteRef          `json:"from"`
	To       SiteRef          `json:"to"`
	Resource string           `json:"resource"`
	Items    []RecordCopyItem `json:"items,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
}

// unresolvedRef captures a reference field that couldn't be mapped during copy.
type unresolvedRef struct {
	Channel    string // channel being copied
	Entry      string // human-readable entry identifier
	TargetID   string // target entry ID (for later patching)
	Field      string // field name
	FieldType  string // "belongs_to", "belongs_to_many", "customer"
	RefChannel string // referenced channel slug
}

type recordCopier struct {
	fromClient     *api.Client
	toClient       *api.Client
	options        RecordCopyOptions
	result         *RecordCopyResult
	mapping        map[string]map[string]string
	preMatched     map[string]map[string]string // channel → sourceID → targetID (matched but content differs)
	queued         map[string]map[string]struct{}
	channelMap     map[string]api.ChannelDetail
	deferredRefs   []deferredRef
	unresolvedRefs []unresolvedRef
}

type schemaInfo struct {
	resource        string
	referenceFields []api.CustomField
	selfRefs        []api.CustomField
	fileFields      []api.CustomField
	galleryFields   []api.CustomField
	selectFields    []api.CustomField
	multiFields     []api.CustomField
	customerFields  []api.CustomField
}

type pendingSelfRef struct {
	channel  string
	targetID string
	fields   map[string]any
}

type deferredRef struct {
	channel   string
	targetID  string
	fields    map[string]any
	refFields []api.CustomField
}

var (
	maxRecordAttachmentBytes int64     = 32 << 20
	passwordRandReader       io.Reader = cryptorand.Reader
)

// CopyChannelEntries copies entries between channels, optionally recursing into dependencies.
func CopyChannelEntries(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef ChannelRef, opts RecordCopyOptions) (RecordCopyResult, error) {
	channels, err := api.ListChannelDetails(ctx, fromClient)
	if err != nil {
		return RecordCopyResult{From: fromRef.SiteRef, To: toRef.SiteRef, Resource: toRef.Channel}, err
	}
	channelMap := make(map[string]api.ChannelDetail, len(channels))
	for _, channel := range channels {
		channelMap[channel.Slug] = channel
	}
	if _, ok := channelMap[fromRef.Channel]; !ok {
		return RecordCopyResult{From: fromRef.SiteRef, To: toRef.SiteRef, Resource: toRef.Channel}, fmt.Errorf("unknown source channel %q", fromRef.Channel)
	}
	result := RecordCopyResult{From: fromRef.SiteRef, To: toRef.SiteRef, Resource: toRef.Channel}
	copier := &recordCopier{
		fromClient: fromClient,
		toClient:   toClient,
		options:    opts,
		result:     &result,
		mapping:    map[string]map[string]string{},
		queued:     map[string]map[string]struct{}{},
		channelMap: channelMap,
	}
	if err := copier.copyChannel(ctx, fromRef.Channel, toRef.Channel, nil, true); err != nil {
		return result, err
	}

	validationWarnings := ValidateEntries(ctx, fromClient, toClient, copier.mapping, channelMap)
	result.Warnings = append(result.Warnings, validationWarnings...)

	return result, nil
}

// CopyCustomers copies customers between sites.
func CopyCustomers(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, opts RecordCopyOptions) (RecordCopyResult, error) {
	if opts.PasswordLength <= 0 {
		opts.PasswordLength = 12
	}
	result := RecordCopyResult{From: fromRef, To: toRef, Resource: "customers"}
	copier := &recordCopier{
		fromClient: fromClient,
		toClient:   toClient,
		options:    opts,
		result:     &result,
		mapping:    map[string]map[string]string{},
		queued:     map[string]map[string]struct{}{},
	}
	fields, err := api.GetCustomerCustomizations(ctx, fromClient)
	if err != nil {
		return result, err
	}
	info := buildSchemaInfo("customers", fields)
	records, err := copier.listRecords(ctx, "customers", nil)
	if err != nil {
		return result, err
	}
	_, warnings, err := copier.copyRecords(ctx, "customers", "customers", info, records)
	result.Warnings = append(result.Warnings, warnings...)
	return result, err
}

func (c *recordCopier) copyChannel(ctx context.Context, sourceChannel string, targetChannel string, ids map[string]struct{}, root bool) error {
	if !c.queueRequest(targetChannel, ids) {
		return nil
	}
	detail := c.channelMap[sourceChannel]
	info := buildSchemaInfo(targetChannel, detail.Customizations)
	records, err := c.listRecords(ctx, sourceChannel, ids)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}

	if c.options.CopyCustomers {
		customerIDs := collectCustomerIDs(records, info)
		if len(customerIDs) > 0 {
			if err := c.copyCustomersByID(ctx, customerIDs); err != nil {
				return err
			}
		}
	}

	if c.options.Recursive {
		dependencies := collectDependencyIDs(records, info)
		keys := make([]string, 0, len(dependencies))
		for channel := range dependencies {
			if !root && channel == sourceChannel {
				continue
			}
			if len(c.options.Only) > 0 && !contains(c.options.Only, channel) {
				continue
			}
			keys = append(keys, channel)
		}
		sort.Strings(keys)
		for _, dependency := range keys {
			if err := c.copyChannel(ctx, dependency, dependency, dependencies[dependency], false); err != nil {
				return err
			}
		}
	}

	c.preMatchEntries(ctx, sourceChannel, targetChannel, records, info)
	_, warnings, err := c.copyRecords(ctx, sourceChannel, targetChannel, info, records)
	c.result.Warnings = append(c.result.Warnings, warnings...)
	return err
}

func (c *recordCopier) copyCustomersByID(ctx context.Context, ids map[string]struct{}) error {
	fields, err := api.GetCustomerCustomizations(ctx, c.fromClient)
	if err != nil {
		return err
	}
	info := buildSchemaInfo("customers", fields)
	records, err := c.listRecords(ctx, "customers", ids)
	if err != nil {
		return err
	}
	_, warnings, err := c.copyRecords(ctx, "customers", "customers", info, records)
	c.result.Warnings = append(c.result.Warnings, warnings...)
	return err
}

func (c *recordCopier) listRecords(ctx context.Context, sourceChannel string, ids map[string]struct{}) ([]map[string]any, error) {
	params, err := buildRecordQuery(c.options, ids)
	if err != nil {
		return nil, err
	}
	var opts []api.RequestOption
	if len(params) > 0 {
		opts = append(opts, api.WithQuery(params))
	}
	opts = append(opts, api.WithParam("x-cdn-expires", "600"))
	path := "/customers"
	if sourceChannel != "customers" {
		path = "/channels/" + url.PathEscape(sourceChannel) + "/entries"
	}
	if c.options.PerPage > 0 {
		var all []map[string]any
		page := 1
		for {
			paged, err := api.ListPage[map[string]any](ctx, c.fromClient, path, page, c.options.PerPage, opts...)
			if err != nil {
				return nil, err
			}
			all = append(all, paged.Data...)
			if !paged.Links.HasNext() || len(paged.Data) == 0 {
				break
			}
			page++
		}
		return all, nil
	}
	return api.List[map[string]any](ctx, c.fromClient, path, opts...)
}

// preMatchEntries fetches existing target entries and matches them against source
// records by slug (+ title_field_value fallback). Content-identical matches are
// added to the mapping and emitted as "skip" items. Content-different matches are
// stored in preMatched for fast lookup during upsert.
func (c *recordCopier) preMatchEntries(ctx context.Context, sourceChannel, targetChannel string, sourceRecords []map[string]any, info schemaInfo) {
	if targetChannel == "customers" {
		return // customers use email-based matching via findExistingID
	}
	path := "/channels/" + url.PathEscape(targetChannel) + "/entries"
	targetRecords, err := api.List[map[string]any](ctx, c.toClient, path)
	if err != nil {
		return // non-fatal: fall back to per-record matching
	}
	if len(targetRecords) == 0 {
		return
	}

	// Build target indexes: slug → entry, title_field_value → entry
	bySlug := make(map[string]map[string]any, len(targetRecords))
	byTitle := make(map[string]map[string]any, len(targetRecords))
	for _, entry := range targetRecords {
		if slug := stringValue(entry["slug"]); slug != "" {
			bySlug[slug] = entry
		}
		if tfv := stringValue(entry["title_field_value"]); tfv != "" {
			byTitle[tfv] = entry
		}
	}

	mapped := c.ensureMapping(targetChannel)
	if c.preMatched == nil {
		c.preMatched = map[string]map[string]string{}
	}
	if c.preMatched[targetChannel] == nil {
		c.preMatched[targetChannel] = map[string]string{}
	}

	for _, source := range sourceRecords {
		sourceID := stringValue(source["id"])
		if sourceID == "" {
			continue
		}
		if _, ok := mapped[sourceID]; ok {
			continue // already matched
		}

		// Match by slug, then title_field_value
		var target map[string]any
		if slug := stringValue(source["slug"]); slug != "" {
			target = bySlug[slug]
		}
		if target == nil {
			if tfv := stringValue(source["title_field_value"]); tfv != "" {
				target = byTitle[tfv]
			}
		}
		if target == nil {
			continue
		}

		targetID := stringValue(target["id"])
		if targetID == "" {
			continue
		}

		if contentEqual(source, target, info) {
			// Content identical — skip
			mapped[sourceID] = targetID
			c.result.Items = append(c.result.Items, RecordCopyItem{
				Action:     "skip",
				Identifier: recordIdentifier(source),
				Resource:   targetChannel,
				SourceID:   sourceID,
				TargetID:   targetID,
			})
		} else {
			// Content differs — pre-match for fast upsert
			c.preMatched[targetChannel][sourceID] = targetID
		}
	}
}

// lookupPreMatched returns a pre-matched target ID for a source record.
func (c *recordCopier) lookupPreMatched(channel, sourceID string) (string, bool) {
	if c.preMatched == nil {
		return "", false
	}
	if m, ok := c.preMatched[channel]; ok {
		if targetID, ok := m[sourceID]; ok {
			return targetID, true
		}
	}
	return "", false
}

// contentEqual compares scalar fields between source and target, ignoring
// system fields, files, galleries, and references (which have different IDs).
func contentEqual(source, target map[string]any, info schemaInfo) bool {
	skipCompare := map[string]bool{
		"id": true, "_id": true, "created_at": true, "updated_at": true,
		"url": true, "entries_url": true, "short_id": true,
	}
	// Skip complex fields (files, galleries) — their IDs differ
	for _, f := range info.fileFields {
		skipCompare[f.Name] = true
	}
	for _, f := range info.galleryFields {
		skipCompare[f.Name] = true
	}
	for _, f := range info.customerFields {
		skipCompare[f.Name] = true
	}

	// Build set of reference field names for targeted comparison.
	refFields := map[string]bool{}
	for _, f := range info.referenceFields {
		refFields[f.Name] = true
	}

	for key, sourceVal := range source {
		if skipCompare[key] {
			continue
		}
		// Reference fields: only check if source has data but target is nil/empty.
		// This detects entries broken by a previous run that sent plain IDs.
		if refFields[key] {
			if refFieldEmpty(target[key]) && !refFieldEmpty(sourceVal) {
				return false
			}
			continue
		}
		targetVal := target[key]
		if w := compareScalarField("", key, sourceVal, targetVal); w != "" {
			return false
		}
	}
	return true
}

// refFieldEmpty returns true if a reference field value is nil or has no
// meaningful content. Handles both plain format (string/[]any) and rich format
// (Reference/Relation objects).
func refFieldEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case []any:
		return len(val) == 0
	case map[string]any:
		// Relation wrapper: check objects array
		if objs, ok := val["objects"].([]any); ok {
			return len(objs) == 0
		}
		// Reference object: check id
		return stringValue(val["id"]) == ""
	}
	return false
}

func (c *recordCopier) copyRecords(ctx context.Context, sourceChannel string, targetChannel string, info schemaInfo, records []map[string]any) (map[string]string, []string, error) {
	mapped := c.ensureMapping(targetChannel)
	var warnings []string
	var pending []pendingSelfRef
	for i, record := range records {
		emitStageItem(ctx, "Channel Entries", recordIdentifier(record), int64(i+1), int64(len(records)))
		sourceID := stringValue(record["id"])
		if sourceID != "" {
			if _, ok := mapped[sourceID]; ok {
				continue
			}
		}
		payload := deepCopyMap(record)
		identifier := recordIdentifier(payload)
		stripSystemFields(payload)
		if err := c.prepareAttachments(ctx, payload, info); err != nil {
			return mapped, warnings, err
		}
		flattenSelectFields(payload, info)
		deferredFields := c.extractDeferredRefs(payload, info)
		selfFields := extractSelfRefs(payload, info.selfRefs)
		c.remapReferences(payload, info, identifier)
		if c.options.Media != nil {
			c.options.Media.RewriteValue(info.resource, payload)
		}
		targetID, action, err := c.upsertRecord(ctx, targetChannel, sourceID, payload)
		if err != nil {
			if isRecoverableRecordError(err) {
				warnings = append(warnings, fmt.Sprintf("%s %s: %v", info.resource, identifier, err))
				continue
			}
			if c.options.AllowErrors {
				warnings = append(warnings, fmt.Sprintf("%s %s: %v", info.resource, identifier, err))
				continue
			}
			return mapped, warnings, err
		}
		if sourceID != "" && targetID != "" {
			mapped[sourceID] = targetID
		}
		pending = append(pending, pendingSelfRef{channel: targetChannel, targetID: targetID, fields: selfFields})
		if len(deferredFields) > 0 && targetID != "" {
			c.deferredRefs = append(c.deferredRefs, deferredRef{
				channel:   targetChannel,
				targetID:  targetID,
				fields:    deferredFields,
				refFields: info.referenceFields,
			})
		}
		c.result.Items = append(c.result.Items, RecordCopyItem{
			Action:     action,
			Identifier: recordIdentifier(payload),
			Resource:   info.resource,
			SourceID:   sourceID,
			TargetID:   targetID,
		})
	}
	for _, item := range pending {
		if len(item.fields) == 0 || item.targetID == "" {
			continue
		}
		c.remapPendingSelfRefs(item.fields, info.selfRefs)
		if err := c.updateRecord(ctx, item.channel, item.targetID, item.fields); err != nil {
			if isRecoverableRecordError(err) {
				warnings = append(warnings, fmt.Sprintf("%s self-ref update %s: %v", item.channel, item.targetID, err))
				continue
			}
			if c.options.AllowErrors {
				warnings = append(warnings, fmt.Sprintf("%s self-ref update %s: %v", item.channel, item.targetID, err))
				continue
			}
			return mapped, warnings, err
		}
	}
	return mapped, warnings, nil
}

func (c *recordCopier) upsertRecord(ctx context.Context, targetChannel string, sourceID string, payload map[string]any) (string, string, error) {
	// Check pre-matched entries first (avoids per-record API query)
	existingID := ""
	if sourceID != "" {
		if id, ok := c.lookupPreMatched(targetChannel, sourceID); ok {
			existingID = id
		}
	}
	if existingID == "" {
		var err error
		existingID, err = c.findExistingID(ctx, targetChannel, payload)
		if err != nil {
			return "", "", err
		}
	}
	if c.options.DryRun {
		action := "create"
		if existingID != "" {
			action = "update"
		}
		return existingID, "dry-run:" + action, nil
	}
	if targetChannel == "customers" {
		if existingID == "" {
			if err := ensureCustomerPassword(payload, c.options.PasswordLength); err != nil {
				return "", "", err
			}
			var created map[string]any
			if err := c.toClient.Post(ctx, "/customers", payload, &created); err != nil {
				return "", "", err
			}
			return stringValue(created["id"]), "create", nil
		}
		delete(payload, "password")
		delete(payload, "password_confirmation")
		var updated map[string]any
		if err := c.toClient.Put(ctx, "/customers/"+url.PathEscape(existingID), payload, &updated); err != nil {
			return "", "", err
		}
		return stringValue(updated["id"]), "update", nil
	}

	if existingID == "" {
		var created map[string]any
		if err := c.toClient.Post(ctx, "/channels/"+url.PathEscape(targetChannel)+"/entries", payload, &created); err != nil {
			return "", "", err
		}
		return stringValue(created["id"]), "create", nil
	}
	var updated map[string]any
	if err := c.toClient.Put(ctx, "/channels/"+url.PathEscape(targetChannel)+"/entries/"+url.PathEscape(existingID), payload, &updated); err != nil {
		return "", "", err
	}
	return stringValue(updated["id"]), "update", nil
}

func (c *recordCopier) updateRecord(ctx context.Context, targetChannel string, targetID string, payload map[string]any) error {
	if len(payload) == 0 {
		return nil
	}
	if c.options.DryRun {
		return nil
	}
	if targetChannel == "customers" {
		return c.toClient.Put(ctx, "/customers/"+url.PathEscape(targetID), payload, &map[string]any{})
	}
	return c.toClient.Put(ctx, "/channels/"+url.PathEscape(targetChannel)+"/entries/"+url.PathEscape(targetID), payload, &map[string]any{})
}

func (c *recordCopier) findExistingID(ctx context.Context, targetChannel string, payload map[string]any) (string, error) {
	fields := parseUpsertFields(c.options.Upsert, targetChannel)
	if len(fields) == 0 {
		if targetChannel == "customers" {
			fields = []string{"email"}
		}
	}
	if len(fields) == 0 {
		return "", nil
	}
	clauses := make([]string, 0, len(fields))
	for _, field := range fields {
		value := lookupValue(payload, field)
		if strings.TrimSpace(value) == "" {
			continue
		}
		key := field
		if targetChannel != "customers" && field == "slug" {
			key = "_slug"
		}
		clauses = append(clauses, fmt.Sprintf(`%s:"%s"`, key, escapeWhereValue(value)))
	}
	if len(clauses) == 0 {
		return "", nil
	}
	where := strings.Join(clauses, " OR ")
	var matches []map[string]any
	path := "/customers"
	if targetChannel != "customers" {
		path = "/channels/" + url.PathEscape(targetChannel) + "/entries"
	}
	if err := c.toClient.Get(ctx, path, &matches, api.WithParam("where", where)); err != nil {
		if api.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	if len(matches) == 0 {
		return "", nil
	}
	return stringValue(matches[0]["id"]), nil
}

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
	return nil
}

func (c *recordCopier) remapReferences(payload map[string]any, info schemaInfo, identifier string) {
	for _, field := range info.referenceFields {
		value := payload[field.Name]
		if value == nil {
			continue
		}
		switch field.Type {
		case "belongs_to", "customer":
			sourceID := refSourceID(value)
			if sourceID == "" {
				continue
			}
			if targetID, ok := c.lookupMappedID(referenceClass(field), sourceID); ok {
				payload[field.Name] = targetID
			} else {
				delete(payload, field.Name)
				c.trackUnresolved(info.resource, identifier, "", field, sourceID)
			}
		case "belongs_to_many":
			sourceIDs := refSourceIDs(value)
			if len(sourceIDs) == 0 {
				continue
			}
			var kept []string
			var unresolved bool
			for _, sid := range sourceIDs {
				if targetID, ok := c.lookupMappedID(referenceClass(field), sid); ok {
					kept = append(kept, targetID)
				} else if sid != "" {
					unresolved = true
				}
			}
			if len(kept) > 0 {
				payload[field.Name] = kept
			} else {
				delete(payload, field.Name)
			}
			if unresolved {
				c.trackUnresolved(info.resource, identifier, "", field, "")
			}
		}
	}
	if ownerID := stringValue(payload["_owner"]); ownerID != "" {
		if targetID, ok := c.lookupMappedID("customers", ownerID); ok {
			payload["_owner"] = targetID
		} else if !c.options.CopyCustomers {
			delete(payload, "_owner")
		}
	}
}

// refSourceID extracts the source ID from a belongs_to value.
// Handles plain string ID (API default) and rich Reference object (client-version 2).
func refSourceID(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		return stringValue(v["id"])
	}
	return ""
}

// refSourceIDs extracts source IDs from a belongs_to_many value.
// Handles plain []string/[]any (API default) and rich Relation object (client-version 2).
func refSourceIDs(value any) []string {
	switch v := value.(type) {
	case []any:
		ids := make([]string, 0, len(v))
		for _, item := range v {
			switch it := item.(type) {
			case string:
				ids = append(ids, it)
			case map[string]any:
				if id := stringValue(it["id"]); id != "" {
					ids = append(ids, id)
				}
			}
		}
		return ids
	case map[string]any:
		// Relation wrapper: {__type: "Relation", objects: [...]}
		if objs, ok := v["objects"].([]any); ok {
			return refSourceIDs(objs)
		}
	}
	return nil
}

func (c *recordCopier) trackUnresolved(channel, identifier, targetID string, field api.CustomField, sourceRefID string) {
	c.unresolvedRefs = append(c.unresolvedRefs, unresolvedRef{
		Channel:    channel,
		Entry:      identifier,
		TargetID:   targetID,
		Field:      field.Name,
		FieldType:  field.Type,
		RefChannel: referenceClass(field),
	})
}

// UnresolvedWarnings formats unresolved references as human-readable warnings.
func (c *recordCopier) UnresolvedWarnings() []string {
	if len(c.unresolvedRefs) == 0 {
		return nil
	}
	var warnings []string
	for _, ref := range c.unresolvedRefs {
		w := fmt.Sprintf("channel=%s entry=%s field=%s: unresolved %s reference to %s",
			ref.Channel, ref.Entry, ref.Field, ref.FieldType, ref.RefChannel)
		if ref.TargetID != "" {
			w += fmt.Sprintf(" (targetID=%s)", ref.TargetID)
		}
		warnings = append(warnings, w)
	}
	return warnings
}

func (c *recordCopier) remapPendingSelfRefs(payload map[string]any, selfRefs []api.CustomField) {
	for _, field := range selfRefs {
		value := payload[field.Name]
		if value == nil {
			continue
		}
		switch field.Type {
		case "belongs_to", "customer":
			sourceID := refSourceID(value)
			if sourceID == "" {
				continue
			}
			if targetID, ok := c.lookupMappedID(referenceClass(field), sourceID); ok {
				payload[field.Name] = targetID
			} else {
				delete(payload, field.Name)
			}
		case "belongs_to_many":
			sourceIDs := refSourceIDs(value)
			if len(sourceIDs) == 0 {
				continue
			}
			var kept []string
			for _, sid := range sourceIDs {
				if targetID, ok := c.lookupMappedID(referenceClass(field), sid); ok {
					kept = append(kept, targetID)
				}
			}
			if len(kept) > 0 {
				payload[field.Name] = kept
			} else {
				delete(payload, field.Name)
			}
		}
	}
}

// extractDeferredRefs removes reference fields pointing to channels not yet in the mapping.
// These will be resolved after all channels are copied.
func (c *recordCopier) extractDeferredRefs(payload map[string]any, info schemaInfo) map[string]any {
	deferred := map[string]any{}
	for _, field := range info.referenceFields {
		refChannel := referenceClass(field)
		if refChannel == info.resource || refChannel == "customers" {
			continue
		}
		if _, hasMappings := c.mapping[refChannel]; hasMappings {
			continue
		}
		if v, ok := payload[field.Name]; ok && v != nil {
			deferred[field.Name] = v
			delete(payload, field.Name)
		}
	}
	return deferred
}

// resolveDeferredRefs remaps and updates all deferred cross-channel references.
func (c *recordCopier) resolveDeferredRefs(ctx context.Context) ([]string, error) {
	var warnings []string
	for _, d := range c.deferredRefs {
		if d.targetID == "" || len(d.fields) == 0 {
			continue
		}
		c.remapDeferredFields(d.channel, d.targetID, d.fields, d.refFields)
		if len(d.fields) == 0 {
			continue // all fields were unresolved and removed
		}
		if err := c.updateRecord(ctx, d.channel, d.targetID, d.fields); err != nil {
			if isRecoverableRecordError(err) {
				warnings = append(warnings, fmt.Sprintf("%s deferred-ref update %s: %v", d.channel, d.targetID, err))
				continue
			}
			if c.options.AllowErrors {
				warnings = append(warnings, fmt.Sprintf("%s deferred-ref update %s: %v", d.channel, d.targetID, err))
				continue
			}
			return warnings, err
		}
	}
	return warnings, nil
}

// remapDeferredFields remaps reference IDs in a deferred payload using the now-complete mapping.
func (c *recordCopier) remapDeferredFields(channel, targetID string, payload map[string]any, refFields []api.CustomField) {
	for _, field := range refFields {
		value := payload[field.Name]
		if value == nil {
			continue
		}
		switch field.Type {
		case "belongs_to", "customer":
			sourceID := refSourceID(value)
			if sourceID == "" {
				continue
			}
			if mappedID, ok := c.lookupMappedID(referenceClass(field), sourceID); ok {
				payload[field.Name] = mappedID
			} else {
				delete(payload, field.Name)
				c.trackUnresolved(channel, targetID, targetID, field, sourceID)
			}
		case "belongs_to_many":
			sourceIDs := refSourceIDs(value)
			if len(sourceIDs) == 0 {
				continue
			}
			var kept []string
			var unresolved bool
			for _, sid := range sourceIDs {
				if mappedID, ok := c.lookupMappedID(referenceClass(field), sid); ok {
					kept = append(kept, mappedID)
				} else if sid != "" {
					unresolved = true
				}
			}
			if len(kept) > 0 {
				payload[field.Name] = kept
			} else {
				delete(payload, field.Name)
			}
			if unresolved {
				c.trackUnresolved(channel, targetID, targetID, field, "")
			}
		}
	}
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

func collectDependencyIDs(records []map[string]any, info schemaInfo) map[string]map[string]struct{} {
	out := map[string]map[string]struct{}{}
	for _, field := range info.referenceFields {
		target := referenceClass(field)
		if target == "" || target == "customers" || target == info.resource {
			continue
		}
		addReferenceIDs(out, target, records, field)
	}
	return out
}

func collectCustomerIDs(records []map[string]any, info schemaInfo) map[string]struct{} {
	out := map[string]struct{}{}
	for _, field := range info.customerFields {
		addReferenceIDs(map[string]map[string]struct{}{"customers": out}, "customers", records, field)
	}
	for _, record := range records {
		if owner := stringValue(record["_owner"]); owner != "" {
			out[owner] = struct{}{}
		}
	}
	return out
}

func addReferenceIDs(target map[string]map[string]struct{}, class string, records []map[string]any, field api.CustomField) {
	if target[class] == nil {
		target[class] = map[string]struct{}{}
	}
	for _, record := range records {
		value := record[field.Name]
		switch field.Type {
		case "belongs_to", "customer":
			if ref, ok := value.(map[string]any); ok {
				if id := stringValue(ref["id"]); id != "" {
					target[class][id] = struct{}{}
				}
			}
		case "belongs_to_many":
			if relation, ok := value.(map[string]any); ok {
				if objects, ok := relation["objects"].([]any); ok {
					for _, rawObject := range objects {
						if ref, ok := rawObject.(map[string]any); ok {
							if id := stringValue(ref["id"]); id != "" {
								target[class][id] = struct{}{}
							}
						}
					}
				}
			}
		}
	}
}

func extractSelfRefs(payload map[string]any, selfRefs []api.CustomField) map[string]any {
	values := map[string]any{}
	for _, field := range selfRefs {
		if value, ok := payload[field.Name]; ok {
			values[field.Name] = value
			delete(payload, field.Name)
		}
	}
	return values
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
