package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
)

// PagedResponse wraps paginated API responses.
type PagedResponse[T any] struct {
	Data       []T
	Pagination Pagination
	Links      Links
}

// Links holds pagination links from the Link header.
type Links struct {
	First string
	Prev  string
	Next  string
	Last  string
}

// linkRE matches Link header entries like: <url>; rel="next"
var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

// ParseLinks parses the Link header.
func ParseLinks(header string) Links {
	var links Links
	matches := linkRE.FindAllStringSubmatch(header, -1)
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		url, rel := m[1], m[2]
		switch rel {
		case "first":
			links.First = url
		case "prev":
			links.Prev = url
		case "next":
			links.Next = url
		case "last":
			links.Last = url
		}
	}
	return links
}

// HasNext returns true if there's a next page.
func (l Links) HasNext() bool {
	return l.Next != ""
}

// List fetches all items from a paginated endpoint.
func List[T any](ctx context.Context, c *Client, path string, opts ...RequestOption) ([]T, error) {
	var all []T
	page := 1

	for {
		paged, err := ListPage[T](ctx, c, path, page, 100, opts...)
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

// ListPage fetches a single page from a paginated endpoint.
func ListPage[T any](ctx context.Context, c *Client, path string, page, perPage int, opts ...RequestOption) (*PagedResponse[T], error) {
	opts = append(opts, WithPage(page, perPage))

	resp, err := c.RawRequest(ctx, http.MethodGet, path, nil, opts...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body := make([]byte, 4096)
		n, _ := resp.Body.Read(body)
		return nil, parseError(resp.StatusCode, body[:n])
	}

	var data []T
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	links := ParseLinks(resp.Header.Get("Link"))
	pagination := parsePagination(resp.Header, page, perPage, len(data))

	return &PagedResponse[T]{
		Data:       data,
		Pagination: pagination,
		Links:      links,
	}, nil
}

func parsePagination(header http.Header, page, perPage, count int) Pagination {
	p := Pagination{
		Page:    page,
		PerPage: perPage,
	}

	if total := header.Get("X-Total-Count"); total != "" {
		if n, err := strconv.Atoi(total); err == nil {
			p.Total = n
			p.TotalKnown = true
		}
	}

	if totalPages := header.Get("X-Total-Pages"); totalPages != "" {
		if n, err := strconv.Atoi(totalPages); err == nil {
			p.TotalPages = n
		}
	} else if p.Total > 0 && perPage > 0 {
		p.TotalPages = (p.Total + perPage - 1) / perPage
	}

	return p
}

// Count fetches the count from an endpoint.
func Count(ctx context.Context, c *Client, path string, opts ...RequestOption) (int, error) {
	var result struct {
		Count int `json:"count"`
	}

	if err := c.Get(ctx, path, &result, opts...); err != nil {
		return 0, err
	}

	return result.Count, nil
}
