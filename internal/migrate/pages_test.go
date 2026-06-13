package migrate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func pageDoc(fullpath, parentPath string) api.PageDocument {
	doc := api.PageDocument{
		"fullpath": fullpath,
		"slug":     fullpath[max(0, lastSlash(fullpath)+1):],
	}
	if parentPath != "" {
		doc["parent_path"] = parentPath
	}
	return doc
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

func fullpaths(docs []api.PageDocument) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = api.PageDocumentFullpath(d)
	}
	return out
}

func TestTopoSortPages(t *testing.T) {
	tests := []struct {
		name  string
		input []api.PageDocument
		want  []string
	}{
		{
			name:  "empty",
			input: nil,
			want:  []string{},
		},
		{
			name: "single root page",
			input: []api.PageDocument{
				pageDoc("home", ""),
			},
			want: []string{"home"},
		},
		{
			name: "parent before child regardless of input order",
			input: []api.PageDocument{
				pageDoc("archive/cookies", "archive"),
				pageDoc("archive", ""),
			},
			want: []string{"archive", "archive/cookies"},
		},
		{
			name: "multi-level nesting",
			input: []api.PageDocument{
				pageDoc("a/b/c", "a/b"),
				pageDoc("a", ""),
				pageDoc("a/b", "a"),
			},
			want: []string{"a", "a/b", "a/b/c"},
		},
		{
			name: "siblings sorted alphabetically",
			input: []api.PageDocument{
				pageDoc("a/y", "a"),
				pageDoc("a/x", "a"),
				pageDoc("a", ""),
			},
			want: []string{"a", "a/x", "a/y"},
		},
		{
			name: "parent outside copy set",
			input: []api.PageDocument{
				pageDoc("archive/page1", "archive"),
				pageDoc("archive/page2", "archive"),
			},
			// archive is not in the set, so children just appear in alphabetical order
			want: []string{"archive/page1", "archive/page2"},
		},
		{
			name: "mixed roots and nested",
			input: []api.PageDocument{
				pageDoc("blog/post1", "blog"),
				pageDoc("about", ""),
				pageDoc("blog", ""),
				pageDoc("archive/old", "archive"),
				pageDoc("archive", ""),
			},
			want: []string{"about", "archive", "archive/old", "blog", "blog/post1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fullpaths(topoSortPages(tt.input))
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %q, want %q\n  full result: %v", i, got[i], tt.want[i], got)
					break
				}
			}
		})
	}
}

// TestCopyPagesAllowErrorsSkipsInvalidPage verifies a page the target API
// rejects (e.g. "invalid editable") is skipped with a warning under
// AllowErrors instead of aborting the stage.
func TestCopyPagesAllowErrorsSkipsInvalidPage(t *testing.T) {
	created := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`[{"fullpath":"bad-page"},{"fullpath":"good-page"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/pages/bad-page" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`{"fullpath":"bad-page","title":"Bad","items":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/pages/good-page" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`{"fullpath":"good-page","title":"Good","items":[]}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/pages/") && r.Header.Get("X-Nimbu-Site") == "target":
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/pages" && r.Header.Get("X-Nimbu-Site") == "target":
			var doc map[string]any
			_ = json.NewDecoder(r.Body).Decode(&doc)
			fullpath, _ := doc["fullpath"].(string)
			if fullpath == "bad-page" {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"message":"invalid editable Intro ehbo"}`))
				return
			}
			created[fullpath] = true
			_, _ = w.Write([]byte(`{"fullpath":"` + fullpath + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fromClient := api.New(srv.URL, "").WithSite("source")
	toClient := api.New(srv.URL, "").WithSite("target")

	// Without AllowErrors the stage aborts.
	_, err := CopyPages(context.Background(), fromClient, toClient, SiteRef{Site: "source"}, SiteRef{Site: "target"}, PageCopyOptions{Query: "*"})
	if err == nil || !strings.Contains(err.Error(), "invalid editable") {
		t.Fatalf("expected invalid editable error, got %v", err)
	}

	result, err := CopyPages(context.Background(), fromClient, toClient, SiteRef{Site: "source"}, SiteRef{Site: "target"}, PageCopyOptions{Query: "*", AllowErrors: true})
	if err != nil {
		t.Fatalf("CopyPages with AllowErrors error = %v", err)
	}
	if !created["good-page"] {
		t.Fatal("good-page should have been created")
	}
	var skips int
	for _, item := range result.Items {
		if item.Action == "skip" {
			skips++
		}
	}
	if skips != 1 {
		t.Fatalf("skips = %d, want 1 (%v)", skips, result.Items)
	}
	if len(result.Warnings) == 0 || !strings.Contains(result.Warnings[len(result.Warnings)-1], "invalid editable") {
		t.Fatalf("expected invalid editable warning, got %v", result.Warnings)
	}
}
