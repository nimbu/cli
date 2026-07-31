package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestTranslationsCreateFileAcceptsArrayAndPreservesResponseShape(t *testing.T) {
	var gotBody []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"key":"home.title","values":{"nl-BE":"Welkom"}},{"key":"home.body","values":{"nl-BE":"Hallo"}}]`))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "translations.json")
	if err := os.WriteFile(path, []byte(`[{"key":"home.title","values":{"nl-BE":"Welkom"}},{"key":"home.body","values":{"nl-BE":"Hallo"}}]`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	command := TranslationsCreateCmd{File: path}
	if err := command.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("create translations: %v", err)
	}
	if len(gotBody) != 2 {
		t.Fatalf("request body length = %d", len(gotBody))
	}

	var response []map[string]any
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(response) != 2 {
		t.Fatalf("response length = %d", len(response))
	}
}

func TestTranslationsCreateFileRejectsNonObjectOrArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "translations.json")
	if err := os.WriteFile(path, []byte(`"not a translation"`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	ctx, _, _ := newAdminWorkflowTestContext(t, "https://api.example.test", output.Mode{})
	err := (&TranslationsCreateCmd{File: path}).Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected top-level type error")
	}
	if !strings.Contains(err.Error(), "object or array") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTranslationsCreateBatchExpandsValuesForHumanAndPlainOutput(t *testing.T) {
	tests := []struct {
		name string
		mode output.Mode
		want []string
	}{
		{
			name: "human",
			mode: output.Mode{},
			want: []string{"home.title", "nl-BE", "Welkom", "fr", "Bienvenue"},
		},
		{
			name: "plain",
			mode: output.Mode{Plain: true},
			want: []string{"home.title\tnl-BE\tWelkom", "home.title\tfr\tBienvenue"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"key":"home.title","values":{"nl-BE":"Welkom","fr":"Bienvenue"}}]`))
			}))
			defer server.Close()

			path := filepath.Join(t.TempDir(), "translations.json")
			if err := os.WriteFile(path, []byte(`[{"key":"home.title","values":{"nl-BE":"Welkom","fr":"Bienvenue"}}]`), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, tc.mode)
			if err := (&TranslationsCreateCmd{File: path}).Run(ctx, &RootFlags{Site: "demo"}); err != nil {
				t.Fatalf("create translations: %v", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output missing %q:\n%s", want, out.String())
				}
			}
		})
	}
}
