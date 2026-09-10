package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/pagepath"
)

const maxPageBatchOps = 10

type surgicalWriteFlags struct {
	Locale string
	Diff   bool
	DryRun bool
}

type plannedOp struct {
	Human   string
	Op      api.BatchOperation
	rebuild func(*surgicalSession) (api.BatchOperation, error)
}

type surgicalSession struct {
	ctx         context.Context
	flags       *RootFlags
	client      *api.Client
	page        string
	locale      string
	pageID      string
	fullpath    string
	updatedAt   string
	etag        string
	doc         map[string]any
	schema      *pagepath.Schema
	schemaTried bool
}

func openSurgicalPage(ctx context.Context, flags *RootFlags, page, locale string) (*surgicalSession, error) {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return nil, err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return nil, err
	}
	session := &surgicalSession{ctx: ctx, flags: flags, client: client, page: page, locale: locale}
	if err := session.reload(); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *surgicalSession) requestOpts() []api.RequestOption {
	if s.locale == "" {
		return nil
	}
	return []api.RequestOption{api.WithContentLocale(s.locale)}
}

func (s *surgicalSession) reload() error {
	doc, err := api.GetPageDocument(s.ctx, s.client, s.page, s.requestOpts()...)
	if err != nil {
		return fmt.Errorf("get page: %w", err)
	}
	return s.applyDocument(doc)
}

func (s *surgicalSession) applyDocument(doc api.PageDocument) error {
	s.doc = map[string]any(doc)
	s.pageID = stringAny(doc["id"])
	if s.pageID == "" {
		return fmt.Errorf("page document is missing id")
	}
	s.fullpath = api.PageDocumentFullpath(doc)
	if s.fullpath == "" {
		s.fullpath = s.page
	}
	s.updatedAt = stringAny(doc["updated_at"])
	etag, err := api.PageETag(s.pageID, s.updatedAt)
	if err != nil {
		return fmt.Errorf("compute page etag: %w", err)
	}
	s.etag = etag
	s.schema = nil
	s.schemaTried = false
	return nil
}

func (s *surgicalSession) ensureSchema() (*pagepath.Schema, error) {
	if s.schema != nil {
		return s.schema, nil
	}
	if s.schemaTried {
		return nil, fmt.Errorf("page schema is unavailable")
	}
	s.schemaTried = true
	schema, err := s.client.GetPageSchema(s.ctx, s.pageID)
	if err != nil {
		return nil, fmt.Errorf("get page schema: %w", err)
	}
	s.schema = schema
	return schema, nil
}

func (s *surgicalSession) resolve(input string) (pagepath.Resolved, error) {
	parsed, err := pagepath.Parse(input)
	if err != nil {
		return pagepath.Resolved{}, err
	}
	resolved, err := pagepath.Resolve(parsed, s.doc, s.schema)
	if err != nil {
		var resolveErr *pagepath.ResolveError
		if errors.As(err, &resolveErr) && s.schema == nil {
			if _, schemaErr := s.ensureSchema(); schemaErr == nil {
				resolved, err = pagepath.Resolve(parsed, s.doc, s.schema)
			}
		}
	}
	if err != nil {
		return pagepath.Resolved{}, pathResolveError(err)
	}
	return enrichResolved(resolved, s.doc), nil
}

func (s *surgicalSession) runBatch(ops []plannedOp, write surgicalWriteFlags) (*api.BatchResult, error) {
	if len(ops) == 0 {
		return nil, fmt.Errorf("no operations to apply")
	}
	if len(ops) > maxPageBatchOps {
		return nil, fmt.Errorf("batch is limited to %d operations", maxPageBatchOps)
	}
	if write.DryRun {
		return nil, printDryRun(s.ctx, ops)
	}

	result, err := s.postBatch(ops)
	if err == nil {
		s.absorbResult(result)
		return result, nil
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 412 {
		return nil, err
	}

	if refetchErr := s.reload(); refetchErr != nil {
		return nil, refetchErr
	}
	if current := apiErr.CurrentETag(); current != "" {
		s.etag = strings.Trim(current, `"`)
	}
	retried := make([]plannedOp, len(ops))
	for i, op := range ops {
		next := op
		if op.rebuild != nil {
			rebuilt, rebuildErr := op.rebuild(s)
			if rebuildErr != nil {
				return nil, rebuildErr
			}
			next.Op = rebuilt
		}
		retried[i] = next
	}
	result, err = s.postBatch(retried)
	if err != nil {
		return nil, err
	}
	s.absorbResult(result)
	return result, nil
}

func (s *surgicalSession) postBatch(ops []plannedOp) (*api.BatchResult, error) {
	batch := make([]api.BatchOperation, len(ops))
	for i, op := range ops {
		batch[i] = op.Op
	}
	return s.client.PostPageBatch(s.ctx, s.pageID, batch, api.BatchOptions{
		Atomic:        true,
		IncludeResult: true,
		ContentLocale: s.locale,
		IfMatch:       s.etag,
	})
}

func (s *surgicalSession) absorbResult(result *api.BatchResult) {
	if result == nil {
		return
	}
	if result.ETag != "" {
		s.etag = strings.Trim(result.ETag, `"`)
	}
	if result.UpdatedAt != "" {
		s.updatedAt = result.UpdatedAt
	}
	if len(result.Page) == 0 {
		return
	}
	var page map[string]any
	if err := json.Unmarshal(result.Page, &page); err == nil {
		s.doc = page
		if id := stringAny(page["id"]); id != "" {
			s.pageID = id
		}
		if fullpath := api.PageDocumentFullpath(page); fullpath != "" {
			s.fullpath = fullpath
		}
	}
}

func batchAnchor(anchor pagepath.Anchor) *api.Anchor {
	switch anchor.Mode {
	case pagepath.AnchorNull:
		return &api.Anchor{Set: true}
	case pagepath.AnchorID:
		return &api.Anchor{Set: true, ID: anchor.ID}
	default:
		return nil
	}
}

func stringAny(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func canvasNameFromRaw(raw string) string {
	rest := strings.TrimPrefix(raw, "/items/")
	rest = strings.TrimSuffix(rest, "/repeatables")
	if i := strings.Index(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	name, err := pagepath.UnescapeName(rest)
	if err != nil {
		return rest
	}
	return name
}
