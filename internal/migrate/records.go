package migrate

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	cryptorand "crypto/rand"

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
	Action     string                    `json:"action"`
	Identifier string                    `json:"identifier"`
	Resource   string                    `json:"resource"`
	SourceID   string                    `json:"source_id,omitempty"`
	TargetID   string                    `json:"target_id,omitempty"`
	Localized  []RecordLocalizedCopyItem `json:"localized,omitempty"`
}

// RecordLocalizedCopyItem captures a localized entry-field copy action.
type RecordLocalizedCopyItem struct {
	Locale string   `json:"locale"`
	Action string   `json:"action"`
	Fields []string `json:"fields,omitempty"`
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
	fromClient              *api.Client
	toClient                *api.Client
	options                 RecordCopyOptions
	result                  *RecordCopyResult
	mapping                 map[string]map[string]string
	preMatched              map[string]map[string]string // channel → sourceID → targetID (matched but content differs)
	preMatchedLocalizedOnly map[string]map[string]string // channel → sourceID → targetID (only localized fields differ)
	queued                  map[string]map[string]struct{}
	channelMap              map[string]api.ChannelDetail
	locales                 []string
	deferredRefs            []deferredRef
	unresolvedRefs          []unresolvedRef
	localizedTargetRecords  map[string]map[string]map[string]map[string]any
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
	localizedFields []api.CustomField
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
		fromClient:             fromClient,
		toClient:               toClient,
		options:                opts,
		result:                 &result,
		mapping:                map[string]map[string]string{},
		queued:                 map[string]map[string]struct{}{},
		channelMap:             channelMap,
		localizedTargetRecords: map[string]map[string]map[string]map[string]any{},
	}
	localizedChannels := []string{fromRef.Channel}
	if opts.Recursive {
		localizedChannels = make([]string, 0, len(channelMap))
		for channel := range channelMap {
			localizedChannels = append(localizedChannels, channel)
		}
	}
	if channelsHaveLocalizedFields(channelMap, localizedChannels) {
		locales, localeWarnings := sharedNonDefaultContentLocales(ctx, fromClient, toClient, fromRef.SiteRef, toRef.SiteRef)
		copier.locales = locales
		result.Warnings = append(result.Warnings, localeWarnings...)
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
	_, warnings, err := copier.copyRecords(ctx, "customers", "customers", info, records, nil)
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
	localizedRecords, err := c.listLocalizedRecords(ctx, sourceChannel, ids, info)
	if err != nil {
		c.result.Warnings = append(c.result.Warnings, fmt.Sprintf("channel=%s: localized source fetch failed: %v", sourceChannel, err))
		localizedRecords = nil
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

	c.preMatchEntries(ctx, sourceChannel, targetChannel, records, localizedRecords, info)
	_, warnings, err := c.copyRecords(ctx, sourceChannel, targetChannel, info, records, localizedRecords)
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
	_, warnings, err := c.copyRecords(ctx, "customers", "customers", info, records, nil)
	c.result.Warnings = append(c.result.Warnings, warnings...)
	return err
}

func (c *recordCopier) listRecords(ctx context.Context, sourceChannel string, ids map[string]struct{}) ([]map[string]any, error) {
	return c.listRecordsWithOptions(ctx, sourceChannel, ids)
}

func (c *recordCopier) listRecordsWithOptions(ctx context.Context, sourceChannel string, ids map[string]struct{}, extra ...api.RequestOption) ([]map[string]any, error) {
	params, err := buildRecordQuery(c.options, ids)
	if err != nil {
		return nil, err
	}
	opts := make([]api.RequestOption, 0, len(extra)+2)
	if len(params) > 0 {
		opts = append(opts, api.WithQuery(params))
	}
	opts = append(opts, api.WithParam("x-cdn-expires", "600"))
	opts = append(opts, extra...)
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

func (c *recordCopier) copyRecords(ctx context.Context, sourceChannel string, targetChannel string, info schemaInfo, records []map[string]any, localizedRecords map[string]map[string]map[string]any) (map[string]string, []string, error) {
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
		identifier := recordIdentifier(record)
		if targetID, ok := c.lookupPreMatchedLocalizedOnly(targetChannel, sourceID); ok {
			localizedItems, err := c.updateLocalizedRecords(ctx, targetChannel, targetID, sourceID, identifier, info, localizedRecords)
			if err != nil {
				if isRecoverableRecordError(err) || c.options.AllowErrors {
					warnings = append(warnings, fmt.Sprintf("%s localized update %s: %v", targetChannel, identifier, err))
				} else {
					return mapped, warnings, err
				}
			}
			if sourceID != "" && targetID != "" {
				mapped[sourceID] = targetID
			}
			c.result.Items = append(c.result.Items, RecordCopyItem{
				Action:     "update",
				Identifier: identifier,
				Resource:   info.resource,
				SourceID:   sourceID,
				TargetID:   targetID,
				Localized:  localizedItems,
			})
			continue
		}

		payload := deepCopyMap(record)
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
		localizedItems, err := c.updateLocalizedRecords(ctx, targetChannel, targetID, sourceID, identifier, info, localizedRecords)
		if err != nil {
			if isRecoverableRecordError(err) || c.options.AllowErrors {
				warnings = append(warnings, fmt.Sprintf("%s localized update %s: %v", targetChannel, identifier, err))
			} else {
				return mapped, warnings, err
			}
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
			Localized:  localizedItems,
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
	return c.updateRecordWithOptions(ctx, targetChannel, targetID, payload)
}

func (c *recordCopier) updateRecordWithOptions(ctx context.Context, targetChannel string, targetID string, payload map[string]any, opts ...api.RequestOption) error {
	if len(payload) == 0 {
		return nil
	}
	if c.options.DryRun {
		return nil
	}
	if targetChannel == "customers" {
		return c.toClient.Put(ctx, "/customers/"+url.PathEscape(targetID), payload, &map[string]any{}, opts...)
	}
	return c.toClient.Put(ctx, "/channels/"+url.PathEscape(targetChannel)+"/entries/"+url.PathEscape(targetID), payload, &map[string]any{}, opts...)
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
