package cmd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestProductsCountForwardsContentLocaleAndFilters(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"count":3}`))
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	command := ProductsCountCmd{CountQueryFlags: CountQueryFlags{
		Locale: "nl", Filters: []string{"status=published"},
	}}
	if err := command.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("count products: %v", err)
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if values.Get("content_locale") != "nl" || values.Get("locale") != "" || values.Get("status") != "published" {
		t.Fatalf("query = %q", query)
	}
}
