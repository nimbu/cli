package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestReadThemeContentTreatsDashAsStdin(t *testing.T) {
	content, err := readThemeContentFromReader("", "-", strings.NewReader("theme code"))
	if err != nil {
		t.Fatalf("read theme content: %v", err)
	}
	if string(content) != "theme code" {
		t.Fatalf("content = %q", content)
	}
}

func TestReadThemeContentRejectsFileAndInlineCode(t *testing.T) {
	_, err := readThemeContentFromReader("template.liquid", "-", strings.NewReader("theme code"))
	if err == nil || !strings.Contains(err.Error(), "either") {
		t.Fatalf("expected mutually exclusive input error, got %v", err)
	}
}

func TestVerifyThemeCodeRefetchesWhenWriteResponseOmitsCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"page.liquid","code":"hello"}`))
	}))
	defer server.Close()

	client := api.New(server.URL, "token")
	if err := verifyThemeCodeResponse(context.Background(), client, "theme", "templates", "page.liquid", []byte("hello"), api.ThemeResource{}); err != nil {
		t.Fatalf("verify theme code: %v", err)
	}
}
