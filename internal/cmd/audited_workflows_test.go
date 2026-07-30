package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestAppsCodeUpdateUsesPutAndEscapesFilename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.EscapedPath() != "/apps/store/code/folder%2Fmain.js" {
			t.Fatalf("path = %s", r.URL.EscapedPath())
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["code"] != "console.log(1)" {
			t.Fatalf("body = %#v", body)
		}
		_, _ = w.Write([]byte(`{"name":"folder/main.js"}`))
	}))
	defer server.Close()
	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := AppsCodeUpdateCmd{App: "store", Filename: "folder/main.js", Assignments: []string{"code=console.log(1)"}}
	if err := cmd.Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("apps code update: %v", err)
	}
}

func TestProductAttachmentDownloadWritesExactBytes(t *testing.T) {
	payload := []byte{0, 1, 2, '\n', 255}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	destination := filepath.Join(t.TempDir(), "attachment.bin")
	cmd := ProductAttachmentsDownloadCmd{Product: "p1", Attachment: "a1", Output: destination}
	if err := cmd.Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("download attachment: %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("downloaded bytes = %v", got)
	}
}

func TestProductAttachmentsListDoesNotPaginateEmbeddedArray(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		attachments := make([]map[string]any, 100)
		for i := range attachments {
			attachments[i] = map[string]any{"id": i}
		}
		_ = json.NewEncoder(w).Encode(attachments)
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	if err := (&ProductAttachmentsListCmd{Product: "p1"}).Run(ctx); err != nil {
		t.Fatalf("list attachments: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestPageVersionsListUsesLimitOffsetPagination(t *testing.T) {
	var offsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offsets = append(offsets, r.URL.Query().Get("offset"))
		offset := r.URL.Query().Get("offset")
		count := 100
		if offset == "100" {
			count = 1
		}
		versions := make([]map[string]any, count)
		for i := range versions {
			versions[i] = map[string]any{"id": offset + ":" + strconv.Itoa(i)}
		}
		_ = json.NewEncoder(w).Encode(versions)
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	if err := (&PageVersionsListCmd{Page: "about"}).Run(ctx); err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if !slices.Equal(offsets, []string{"0", "100"}) {
		t.Fatalf("offsets = %v", offsets)
	}
}

func TestPageVersionsListStopsWhenServerIgnoresOffset(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		versions := make([]map[string]any, 100)
		for i := range versions {
			versions[i] = map[string]any{"id": i}
		}
		_ = json.NewEncoder(w).Encode(versions)
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	err := (&PageVersionsListCmd{Page: "about"}).Run(ctx)
	if err == nil || !strings.Contains(err.Error(), "did not advance") {
		t.Fatalf("expected pagination error, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestAuditedDeleteRequiresForce(t *testing.T) {
	ctx, _, _ := newAdminWorkflowTestContext(t, "http://127.0.0.1:1", output.Mode{})
	err := (&AnnouncementsDeleteCmd{ID: "a1"}).Run(ctx, &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force error, got %v", err)
	}
}

func TestAnnouncementsDoNotRequireOrSendSiteContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if site := r.Header.Get("X-Nimbu-Site"); site != "" {
			t.Fatalf("unexpected site header %q", site)
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	ctx.Value(rootFlagsKey{}).(*RootFlags).Site = ""
	ctx.Value(configKey{}).(*config.Config).DefaultSite = ""
	if err := (&AnnouncementsListCmd{All: true}).Run(ctx); err != nil {
		t.Fatalf("list announcements: %v", err)
	}
}

func TestCustomerRolesAddPreservesDirectRoleMembership(t *testing.T) {
	var updated bool
	var submitted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/customers/c1":
			_, _ = w.Write([]byte(`{"id":"c1","roles":["parent","child"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/roles":
			roles := []map[string]any{
				{"id": "parent-id", "name": "parent", "customers": []string{"c1"}, "children": []string{"child-id"}},
				{"id": "child-id", "name": "child", "customers": []string{}},
				{"id": "other-id", "name": "other", "customers": []string{}},
			}
			if updated {
				roles[2]["customers"] = []string{"c1"}
			}
			_ = json.NewEncoder(w).Encode(roles)
		case r.Method == http.MethodPut && r.URL.Path == "/customers/c1":
			var body struct {
				Roles []string `json:"roles"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			submitted = body.Roles
			updated = true
			_, _ = w.Write([]byte(`{"id":"c1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := CustomerRolesAddCmd{Customer: "c1", Roles: []string{"other"}}
	if err := cmd.Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("add role: %v", err)
	}
	if !slices.Equal(submitted, []string{"other-id", "parent-id"}) {
		t.Fatalf("submitted roles = %v", submitted)
	}
}

func TestEventsIngestUsesCountlyCompatibleGetQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Query().Get("device_id") != "device-1" || r.URL.Query().Get("events") != `[{"key":"open"}]` {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"result":"Success"}`))
	}))
	defer server.Close()
	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := EventsIngestCmd{DeviceID: "device-1", Events: `[{"key":"open"}]`}
	if err := cmd.Run(ctx); err != nil {
		t.Fatalf("events ingest: %v", err)
	}
}
