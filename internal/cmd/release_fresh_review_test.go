package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestReleaseChecksRecordingAccessBeforeDeployment(t *testing.T) {
	root := releaseProjectFixture(t)
	deployments := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/releases":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"release access denied"}`))
		case "/products/customizations/plan":
			_, _ = w.Write([]byte(`{"target":"products","exists":true,"fingerprint":"abc","ops":[{"kind":"add_field","risk":"safe"}]}`))
		case "/products/customizations/apply":
			deployments++
			_, _ = w.Write([]byte(`{"applied":true,"fingerprint":"updated"}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	ctx, _, _ := newSitesListTestContext(t, server.URL, output.Mode{JSON: true})

	err := withWorkingDir(t, root, func() error {
		return (&ReleaseCmd{Only: []string{"schema"}, AllowDirty: true, Yes: true}).Run(ctx, &RootFlags{APIURL: server.URL, NoInput: true})
	})

	if err == nil || !strings.Contains(err.Error(), "before deployment") || deployments != 0 {
		t.Fatalf("err=%v deployments=%d", err, deployments)
	}
}

func TestReleaseDryRunPreservesBlockedPlanErrorDetails(t *testing.T) {
	root := releaseProjectFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products/customizations/plan" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"target":"products","exists":true,"fingerprint":"abc","ops":[{"kind":"retype_field","risk":"blocked","reason":"incompatible"}]}`))
	}))
	defer server.Close()
	ctx, _, _ := newSitesListTestContext(t, server.URL, output.Mode{JSON: true})

	err := withWorkingDir(t, root, func() error {
		return (&ReleaseCmd{Only: []string{"schema"}, AllowDirty: true, DryRun: true}).Run(ctx, &RootFlags{APIURL: server.URL, NoInput: true})
	})

	var displayed *displayedError
	if err == nil || errors.As(err, &displayed) {
		t.Fatalf("blocked plan diagnostic suppressed: %v", err)
	}
	details := classifyError(err).Details
	data, marshalErr := json.Marshal(details)
	if marshalErr != nil || !strings.Contains(string(data), "incompatible") {
		t.Fatalf("missing plan details: %s %v", data, marshalErr)
	}
}
