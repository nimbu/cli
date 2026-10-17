package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/migrate"
)

func TestPlainDiffChangesEscapesFields(t *testing.T) {
	got := plainDiffChanges("update", []migrate.DiffChange{
		{Kind: "updated", Path: "fields[title].hint", From: "old\thint", To: "new\nhint\\x"},
		{Kind: "added", Path: "fields[a\tb]"},
	})
	want := []string{
		"update\tfields[title].hint\told\\thint\tnew\\nhint\\\\x",
		"update\tfields[a\\tb]",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("plain diff lines:\n got %q\nwant %q", got, want)
	}
	// Human lines keep the raw values.
	if human := renderDiffChanges("~", []migrate.DiffChange{{Kind: "updated", Path: "p", From: "a\tb", To: 2}}); human[0] != "~\tp\ta\tb\t2" {
		t.Fatalf("human diff line = %q", human[0])
	}
}

// Regression: diff --plain built its rows with Sprintf("%s\t%s\t%v\t%v"), so
// a field hint holding a newline split one change over two records.
func TestChannelsFieldsDiffPlainEscapesValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/channels/articles/customizations" {
			_, _ = w.Write([]byte(`[{"id":"f1","name":"title","label":"Title","type":"string","hint":"one line"}]`))
			return
		}
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()

	file := filepath.Join(t.TempDir(), "fields.json")
	body := `[{"name":"title","label":"Title","type":"string","hint":"two\nlines\twith tab"}]`
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	ctx := plainCopyContext(t, srv.URL, &out)
	cmd := &ChannelsFieldsDiffCmd{Channel: "articles", File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "source", APIURL: srv.URL, Timeout: 2 * time.Second}); err != nil {
		t.Fatalf("run fields diff: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one plain record, got %q", out.String())
	}
	fields := strings.Split(lines[0], "\t")
	if len(fields) != 4 || fields[0] != "update" || fields[3] != `two\nlines\twith tab` {
		t.Fatalf("expected 4 fields with the escaped hint, got %q", fields)
	}
}
