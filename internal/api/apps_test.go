package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListAppLogsSendsQueryAndDecodesEntries(t *testing.T) {
	var seen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = true
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/apps/storefront/logs" {
			t.Fatalf("path = %s, want /apps/storefront/logs", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("X-Nimbu-Site"); got != "demo" {
			t.Fatalf("site header = %q", got)
		}
		query := r.URL.Query()
		for key, want := range map[string]string{
			"since": "1751700000.123",
			"level": "error",
			"q":     "level:error checkout",
			"job":   "sync_products",
			"limit": "25",
		} {
			if got := query.Get(key); got != want {
				t.Fatalf("query %s = %q, want %q; raw query %q", key, got, want, r.URL.RawQuery)
			}
		}
		_, _ = w.Write([]byte(`[
			{
				"id":"abc123",
				"time":1751700000.123,
				"level":"ERROR",
				"data":{"message":"checkout failed"},
				"created_at":"2026-07-05T12:00:00.123Z",
				"context":null
			}
		]`))
	}))
	defer server.Close()

	client := New(server.URL, "test-token").WithSite("demo")
	logs, err := ListAppLogs(context.Background(), client, "storefront", AppLogOptions{
		Since: "1751700000.123",
		Level: "error",
		Query: "level:error checkout",
		Job:   "sync_products",
		Limit: 25,
	})
	if err != nil {
		t.Fatalf("ListAppLogs: %v", err)
	}
	if !seen {
		t.Fatal("server was not called")
	}
	if len(logs) != 1 {
		t.Fatalf("log count = %d", len(logs))
	}
	if logs[0].ID != "abc123" || logs[0].Time != 1751700000.123 || logs[0].Level != "ERROR" {
		t.Fatalf("unexpected log entry: %#v", logs[0])
	}
	if got := strings.TrimSpace(string(logs[0].Data)); got != `{"message":"checkout failed"}` {
		t.Fatalf("data = %s", got)
	}
}
