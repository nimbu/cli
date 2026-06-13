package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestChannelsCreatePostsBodyAndPrintsChannel(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/channels":
			gotMethod, gotPath = r.Method, r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode post body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"c1","slug":"testimonials","name":"Testimonials"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &ChannelsCreateCmd{
		Assignments: []string{"name=Testimonials", "slug=testimonials", "title_field=author"},
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run channels create: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/channels" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["name"] != "Testimonials" || gotBody["slug"] != "testimonials" || gotBody["title_field"] != "author" {
		t.Fatalf("unexpected create body: %#v", gotBody)
	}

	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if resp["slug"] != "testimonials" {
		t.Fatalf("unexpected output: %#v", resp)
	}
}

func TestChannelsCreateAcceptsFileWithCustomizations(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/channels" {
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode post body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"c1","slug":"cases","name":"Cases"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	file := writeFieldsFile(t, `{"name":"Cases","slug":"cases","customizations":[{"name":"title","type":"string","localized":true}]}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &ChannelsCreateCmd{File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run channels create from file: %v", err)
	}

	fields, ok := gotBody["customizations"].([]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("expected one customization, got %#v", gotBody["customizations"])
	}
	field := fields[0].(map[string]any)
	if field["name"] != "title" || field["localized"] != true {
		t.Fatalf("unexpected customization payload: %#v", field)
	}
}

func TestChannelsCreateRejectsFileAndInlineTogether(t *testing.T) {
	ctx, _, _ := newContractTestContext(t, "https://api.example.test", output.Mode{})
	cmd := &ChannelsCreateCmd{File: "channel.json", Assignments: []string{"slug=foo"}}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "either --file or inline") {
		t.Fatalf("expected file/inline mutual-exclusion error, got %v", err)
	}
}

func TestChannelsCreateBlockedInReadonly(t *testing.T) {
	ctx, _, _ := newContractTestContext(t, "https://api.example.test", output.Mode{})
	cmd := &ChannelsCreateCmd{Assignments: []string{"slug=foo"}}
	err := cmd.Run(ctx, &RootFlags{Site: "demo", Readonly: true})
	if err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("expected readonly error, got %v", err)
	}
}

func TestChannelsDeleteRequiresForce(t *testing.T) {
	ctx, _, _ := newContractTestContext(t, "https://api.example.test", output.Mode{})
	err := (&ChannelsDeleteCmd{Channel: "testimonials"}).Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected --force error, got %v", err)
	}
}

func TestChannelsDeleteSendsDeleteRequest(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/channels/testimonials" {
			gotMethod, gotPath = r.Method, r.URL.Path
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &ChannelsDeleteCmd{Channel: "testimonials"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", Force: true}); err != nil {
		t.Fatalf("run channels delete: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/channels/testimonials" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}

	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if resp["status"] != "success" {
		t.Fatalf("unexpected delete output: %#v", resp)
	}
}

func TestChannelsDeleteRejectsBlankSlug(t *testing.T) {
	ctx, _, _ := newContractTestContext(t, "https://api.example.test", output.Mode{})
	cmd := &ChannelsDeleteCmd{Channel: "   "}
	err := cmd.Run(ctx, &RootFlags{Site: "demo", Force: true})
	if err == nil || !strings.Contains(err.Error(), "channel slug is required") {
		t.Fatalf("expected blank-slug rejection, got %v", err)
	}
}
