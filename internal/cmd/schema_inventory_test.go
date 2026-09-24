package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestSchemaTargetDriftReportsOnlyUndeclaredTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/channels":
			_, _ = w.Write([]byte(`[{"slug":"events"},{"slug":"legacy"},{"slug":"legacy"}]`))
		case "/blogs":
			_, _ = w.Write([]byte(`[{"handle":"news"}]`))
		case "/products/checkout_profiles":
			_, _ = w.Write([]byte(`[{"slug":"gift"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	docs := []schemaDocument{{Target: "channel:events"}, {Target: "checkout_profile:gift"}, {Target: "blog:declared"}}
	plans, err := schemaTargetDrift(context.Background(), api.New(server.URL, "test"), docs)
	if err != nil {
		t.Fatal(err)
	}
	targets := []string{}
	for _, plan := range plans {
		targets = append(targets, plan.Target)
		if len(plan.Ops) != 0 || len(plan.Drift) != 1 || plan.Drift[0]["target"] != plan.Target || !plan.Exists {
			t.Fatalf("unexpected drift: %#v", plan)
		}
	}
	if !reflect.DeepEqual(targets, []string{"blog:news", "channel:legacy"}) {
		t.Fatalf("targets: %v", targets)
	}
	if code := schemaPlanExit(plans); code != 0 {
		t.Fatalf("informational drift changed exit code: %d", code)
	}
}

func TestSchemaTargetDriftPaginates(t *testing.T) {
	pages := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/channels" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		if page == "1" {
			items := make([]map[string]any, 100)
			for i := range items {
				items[i] = map[string]any{"slug": fmt.Sprintf("channel-%03d", i)}
			}
			_ = json.NewEncoder(w).Encode(items)
		} else {
			_, _ = w.Write([]byte(`[{"slug":"last"}]`))
		}
	}))
	defer server.Close()
	plans, err := schemaTargetDrift(context.Background(), api.New(server.URL, "test"), []schemaDocument{{Target: "channel:declared"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 101 || !reflect.DeepEqual(pages, []string{"1", "2"}) {
		t.Fatalf("plans=%d pages=%v", len(plans), pages)
	}
}

func TestSchemaTargetDriftReturnsInventoryErrors(t *testing.T) {
	for _, body := range []string{`[{"id":"missing-slug"}]`, `invalid-json`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			_, err := schemaTargetDrift(context.Background(), api.New(server.URL, "test"), []schemaDocument{{Target: "channel:declared"}})
			if err == nil || !strings.Contains(err.Error(), "inventory channel schemas") {
				t.Fatalf("error: %v", err)
			}
		})
	}
}

func TestSchemaTargetDriftSkipsUnmanagedKinds(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/channels" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`[{"slug":"events"}]`))
	}))
	defer server.Close()
	plans, err := schemaTargetDrift(context.Background(), api.New(server.URL, "test"), []schemaDocument{{Target: "channel:events"}, {Target: "products"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 0 || !reflect.DeepEqual(paths, []string{"/channels"}) {
		t.Fatalf("plans=%v paths=%v", plans, paths)
	}
}
