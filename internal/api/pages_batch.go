package api

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nimbu/cli/internal/pagepath"
)

// Anchor is a tri-state batch `after` value: omitted, JSON null, or an id.
type Anchor struct {
	Set bool
	ID  string
}

// MarshalJSON encodes an explicit null or a repeatable id.
func (a Anchor) MarshalJSON() ([]byte, error) {
	if !a.Set || a.ID == "" {
		return []byte("null"), nil
	}
	return json.Marshal(a.ID)
}

// BatchOperation is one POST /pages/{id}/batch operation.
type BatchOperation struct {
	Op    string  `json:"op"`
	Path  string  `json:"path"`
	Value any     `json:"value,omitempty"`
	After *Anchor `json:"after,omitempty"`
}

// BatchResult is the successful batch response.
type BatchResult struct {
	Results   []BatchOpResult `json:"results"`
	ETag      string          `json:"etag"`
	UpdatedAt string          `json:"updated_at"`
	Page      json.RawMessage `json:"page,omitempty"`
}

// BatchOpResult is one per-operation outcome.
type BatchOpResult struct {
	Index   int           `json:"index"`
	Status  string        `json:"status"`
	Path    string        `json:"path"`
	ID      string        `json:"id,omitempty"`
	Error   *BatchOpError `json:"error,omitempty"`
	Warning string        `json:"warning,omitempty"`
}

// BatchOpError is a per-operation failure.
type BatchOpError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// BatchOptions configure POST /pages/{id}/batch.
type BatchOptions struct {
	Atomic        bool
	IncludeResult bool
	ContentLocale string
	IfMatch       string
}

// PageSubtree is GET /pages/{id}/items/... with a lossless raw body.
type PageSubtree struct {
	Path          string          `json:"path"`
	ParentPath    string          `json:"parent_path"`
	Position      int             `json:"position"`
	SiblingsCount int             `json:"siblings_count"`
	Type          string          `json:"type"`
	Data          json.RawMessage `json:"data"`
	raw           json.RawMessage
}

func (s *PageSubtree) UnmarshalJSON(data []byte) error {
	type alias PageSubtree
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = PageSubtree(decoded)
	s.raw = bytes.Clone(data)
	return nil
}

func (s PageSubtree) MarshalJSON() ([]byte, error) {
	if len(s.raw) > 0 {
		return bytes.Clone(s.raw), nil
	}
	type alias PageSubtree
	return json.Marshal(alias(s))
}

// PostPageBatch sends operations to POST /pages/{pageID}/batch.
func (c *Client) PostPageBatch(ctx context.Context, pageID string, ops []BatchOperation, opts BatchOptions) (*BatchResult, error) {
	path := "/pages/" + url.PathEscape(pageID) + "/batch"
	query := map[string]string{}
	if opts.Atomic {
		query["atomic"] = "1"
	}
	if opts.IncludeResult {
		query["include"] = "result"
	}
	if opts.ContentLocale != "" {
		query["content_locale"] = opts.ContentLocale
	}
	var reqOpts []RequestOption
	if len(query) > 0 {
		reqOpts = append(reqOpts, WithQuery(query))
	}
	if etag := strings.TrimSpace(opts.IfMatch); etag != "" {
		reqOpts = append(reqOpts, WithHeader("If-Match", quoteETag(etag)))
	}

	var result BatchResult
	if err := c.Post(ctx, path, map[string]any{"operations": ops}, &result, reqOpts...); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPageSchema fetches GET /pages/{pageID}/schema.
func (c *Client) GetPageSchema(ctx context.Context, pageID string) (*pagepath.Schema, error) {
	var schema pagepath.Schema
	if err := c.Get(ctx, "/pages/"+url.PathEscape(pageID)+"/schema", &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}

// GetPageItems fetches GET /pages/{pageID}/items/<raw path without /items/>.
func (c *Client) GetPageItems(ctx context.Context, pageID, rawPath string) (*PageSubtree, error) {
	var subtree PageSubtree
	if err := c.Get(ctx, pageItemsPath(pageID, rawPath), &subtree); err != nil {
		return nil, err
	}
	return &subtree, nil
}

func pageItemsPath(pageID, rawPath string) string {
	rest := strings.TrimPrefix(strings.TrimSpace(rawPath), "/")
	rest = strings.TrimPrefix(rest, "items/")
	return "/pages/" + url.PathEscape(pageID) + "/items/" + rest
}

func quoteETag(etag string) string {
	if strings.HasPrefix(etag, `"`) && strings.HasSuffix(etag, `"`) {
		return etag
	}
	return `"` + etag + `"`
}

// PageETag is md5("<id>-<updated_at.to_f>") matching Ruby Time#to_f.to_s.
func PageETag(id, updatedAt string) (string, error) {
	stamp, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return "", fmt.Errorf("parse updated_at: %w", err)
	}
	seconds := float64(stamp.Unix()) + float64(stamp.Nanosecond())/1e9
	rendered := strconv.FormatFloat(seconds, 'f', -1, 64)
	if !strings.Contains(rendered, ".") {
		rendered += ".0"
	}
	sum := md5.Sum([]byte(id + "-" + rendered))
	return hex.EncodeToString(sum[:]), nil
}
