package migrate

import (
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
