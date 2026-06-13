package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// pagedServer serves `total` items in pages of 100. Pages listed in
// degradedPages omit the Link header (and totals when withTotals is false),
// simulating rate-limited or CDN-degraded responses.
func pagedServer(t *testing.T, total int, withTotals bool, degradedPages map[int]bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		perPage := 100
		start := (page - 1) * perPage
		end := start + perPage
		if end > total {
			end = total
		}
		degraded := degradedPages[page]
		if !degraded {
			if withTotals {
				w.Header().Set("X-Total-Count", strconv.Itoa(total))
			}
			if end < total {
				w.Header().Set("Link", fmt.Sprintf(`<http://x/items?page=%d>; rel="next"`, page+1))
			} else {
				w.Header().Set("Link", `<http://x/items?page=1>; rel="first"`)
			}
		} else if withTotals {
			w.Header().Set("X-Total-Count", strconv.Itoa(total))
		}
		w.Header().Set("Content-Type", "application/json")
		items := ""
		for i := start; i < end; i++ {
			if items != "" {
				items += ","
			}
			items += fmt.Sprintf(`{"id":"item-%d"}`, i)
		}
		_, _ = w.Write([]byte("[" + items + "]"))
	}))
}

// A mid-stream response missing the Link header must not silently truncate
// the listing (regression: site copy stopped at 1303 of 2662 entries).
func TestListSurvivesMissingLinkHeaderWithTotals(t *testing.T) {
	srv := pagedServer(t, 350, true, map[int]bool{2: true})
	defer srv.Close()

	items, err := List[map[string]any](context.Background(), New(srv.URL, ""), "/items")
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(items) != 350 {
		t.Fatalf("items = %d, want 350", len(items))
	}
}

func TestListSurvivesMissingLinkHeaderWithoutTotals(t *testing.T) {
	srv := pagedServer(t, 350, false, map[int]bool{2: true})
	defer srv.Close()

	items, err := List[map[string]any](context.Background(), New(srv.URL, ""), "/items")
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(items) != 350 {
		t.Fatalf("items = %d, want 350", len(items))
	}
}

func TestListStopsOnLastPage(t *testing.T) {
	srv := pagedServer(t, 250, true, nil)
	defer srv.Close()

	items, err := List[map[string]any](context.Background(), New(srv.URL, ""), "/items")
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(items) != 250 {
		t.Fatalf("items = %d, want 250", len(items))
	}
}

// An exact-multiple total with totals known must not fetch forever.
func TestListStopsOnExactMultipleWithDegradedLastPage(t *testing.T) {
	srv := pagedServer(t, 200, true, map[int]bool{2: true})
	defer srv.Close()

	items, err := List[map[string]any](context.Background(), New(srv.URL, ""), "/items")
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(items) != 200 {
		t.Fatalf("items = %d, want 200", len(items))
	}
}
