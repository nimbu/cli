package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestRealtimeGrantPrintsGrantAndSecrecyWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realtime/grants":
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"grant":"g-123","expires_at":"2126-09-21T10:02:00Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, errOut := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	if err := (&RealtimeGrantCmd{}).Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL}); err != nil {
		t.Fatalf("run realtime grant: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode stdout %q: %v", out.String(), err)
	}
	if payload["grant"] != "g-123" {
		t.Fatalf("grant = %#v", payload["grant"])
	}
	if payload["expires_at"] != "2126-09-21T10:02:00Z" {
		t.Fatalf("expires_at = %#v", payload["expires_at"])
	}

	if !strings.Contains(errOut.String(), "single-use") || !strings.Contains(errOut.String(), "secret") {
		t.Fatalf("stderr = %q, want the secrecy warning", errOut.String())
	}
	if strings.Contains(errOut.String(), "g-123") {
		t.Fatalf("the warning must not repeat the grant: %q", errOut.String())
	}
}

func TestRealtimeGrantHumanOutputUsesStableLabels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"grant":"g-456","expires_at":"2126-09-21T10:02:00Z"}`))
	}))
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	if err := (&RealtimeGrantCmd{}).Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL}); err != nil {
		t.Fatalf("run realtime grant: %v", err)
	}
	if !strings.Contains(out.String(), "grant:") || !strings.Contains(out.String(), "expires_at:") {
		t.Fatalf("stdout = %q", out.String())
	}
}
