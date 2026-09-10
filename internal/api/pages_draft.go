package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
)

// DraftOptions configure draft GET/POST query params.
type DraftOptions struct {
	ContentLocale string
}

// DraftBatchOptions configure POST /pages/{id}/draft/batch.
type DraftBatchOptions struct {
	ContentLocale string
}

// DraftBatchResult is the successful draft batch response.
type DraftBatchResult struct {
	Results []BatchOpResult `json:"results"`
	Draft   json.RawMessage `json:"draft"`
}

// PageDraftPreviewToken is POST /pages/{id}/draft/preview_token.
type PageDraftPreviewToken struct {
	Token      string `json:"token"`
	PreviewURL string `json:"preview_url"`
}

// PageDraft is GET/POST /pages/{id}/draft with a lossless raw body.
type PageDraft struct {
	ID               string          `json:"id"`
	PageID           string          `json:"page_id"`
	FuturePageID     string          `json:"future_page_id,omitempty"`
	ReservedFullpath string          `json:"reserved_fullpath,omitempty"`
	Content          json.RawMessage `json:"content"`
	UpdatedAt        string          `json:"updated_at"`
	raw              json.RawMessage
}

func (d *PageDraft) UnmarshalJSON(data []byte) error {
	type alias PageDraft
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*d = PageDraft(decoded)
	d.raw = bytes.Clone(data)
	return nil
}

func (d PageDraft) MarshalJSON() ([]byte, error) {
	if len(d.raw) > 0 {
		return bytes.Clone(d.raw), nil
	}
	type alias PageDraft
	return json.Marshal(alias(d))
}

// ContentMap decodes draft.content as a generic object.
func (d PageDraft) ContentMap() map[string]any {
	if len(d.Content) == 0 {
		return nil
	}
	var content map[string]any
	if err := json.Unmarshal(d.Content, &content); err != nil {
		return nil
	}
	return content
}

func draftQueryOpts(locale string) []RequestOption {
	if locale == "" {
		return nil
	}
	return []RequestOption{WithContentLocale(locale)}
}

func pageDraftPath(pageID, suffix string) string {
	path := "/pages/" + url.PathEscape(pageID) + "/draft"
	if suffix != "" {
		path += suffix
	}
	return path
}

// GetPageDraft fetches GET /pages/{pageID}/draft.
func (c *Client) GetPageDraft(ctx context.Context, pageID string, opts DraftOptions) (*PageDraft, error) {
	var draft PageDraft
	if err := c.Get(ctx, pageDraftPath(pageID, ""), &draft, draftQueryOpts(opts.ContentLocale)...); err != nil {
		return nil, err
	}
	return &draft, nil
}

// PostPageDraft saves POST /pages/{pageID}/draft with a page-like body.
func (c *Client) PostPageDraft(ctx context.Context, pageID string, body any, opts DraftOptions) (*PageDraft, error) {
	var draft PageDraft
	if err := c.Post(ctx, pageDraftPath(pageID, ""), body, &draft, draftQueryOpts(opts.ContentLocale)...); err != nil {
		return nil, err
	}
	return &draft, nil
}

// PostPageDraftBatch sends operations to POST /pages/{pageID}/draft/batch.
func (c *Client) PostPageDraftBatch(ctx context.Context, pageID string, ops []BatchOperation, opts DraftBatchOptions) (*DraftBatchResult, error) {
	var result DraftBatchResult
	if err := c.Post(ctx, pageDraftPath(pageID, "/batch"), map[string]any{"operations": ops}, &result, draftQueryOpts(opts.ContentLocale)...); err != nil {
		return nil, err
	}
	return &result, nil
}

// PublishPageDraft posts POST /pages/{pageID}/draft/publish.
func (c *Client) PublishPageDraft(ctx context.Context, pageID string, confirm bool) (PageDocument, error) {
	var page PageDocument
	if err := c.Post(ctx, pageDraftPath(pageID, "/publish"), map[string]any{"confirm": confirm}, &page); err != nil {
		return nil, err
	}
	return page, nil
}

// DeletePageDraft discards DELETE /pages/{pageID}/draft.
func (c *Client) DeletePageDraft(ctx context.Context, pageID string) error {
	return c.Delete(ctx, pageDraftPath(pageID, ""), nil)
}

// CreatePageDraftPreviewToken posts POST /pages/{pageID}/draft/preview_token.
func (c *Client) CreatePageDraftPreviewToken(ctx context.Context, pageID string) (*PageDraftPreviewToken, error) {
	var token PageDraftPreviewToken
	if err := c.Post(ctx, pageDraftPath(pageID, "/preview_token"), map[string]any{}, &token); err != nil {
		return nil, err
	}
	return &token, nil
}
