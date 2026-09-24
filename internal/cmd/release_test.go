package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestReleaseStageNames(t *testing.T) {
	got, err := releaseStageNames([]string{"theme,schema", "code", "schema"})
	if err != nil || !reflect.DeepEqual(got, []string{"schema", "code", "theme"}) {
		t.Fatalf("stages=%v err=%v", got, err)
	}
	if _, err := releaseStageNames([]string{"content"}); err == nil {
		t.Fatal("unknown stage accepted")
	}
}

func TestReleaseStopsAtFailedStage(t *testing.T) {
	for _, failed := range []string{"schema", "code", "theme"} {
		t.Run(failed, func(t *testing.T) {
			ran := []string{}
			stages := []releaseStage{}
			for _, name := range []string{"schema", "code", "theme"} {
				stages = append(stages, releaseStage{name: name, run: func() (any, error) {
					ran = append(ran, name)
					if name == failed {
						return nil, errors.New("test failure")
					}
					return map[string]int{"updated": 1}, nil
				}})
			}
			recorded := false
			record := releaseRecord{Summary: map[string]any{}}
			err := executeRelease(context.Background(), stages, &record, false, func() error { recorded = true; return nil })
			if err == nil || !strings.Contains(err.Error(), "failed at "+failed) || !strings.Contains(err.Error(), "unattempted:") {
				t.Fatalf("error=%v", err)
			}
			if recorded {
				t.Fatal("recorded failed release")
			}
			if ran[len(ran)-1] != failed {
				t.Fatalf("ran beyond failure: %v", ran)
			}
			if _, ok := record.Summary[failed]; ok {
				t.Fatal("failed stage marked successful")
			}
		})
	}
}

func TestReleaseRecordingFailureDistinguished(t *testing.T) {
	record := releaseRecord{Summary: map[string]any{}}
	stages := []releaseStage{{name: "schema", run: func() (any, error) { return "applied", nil }}}
	err := executeRelease(context.Background(), stages, &record, false, func() error { return errors.New("offline") })
	if err == nil || !strings.Contains(err.Error(), "deployment completed (schema), but release recording failed") {
		t.Fatalf("error=%v", err)
	}
}

func TestReleaseDryRunDoesNotRecord(t *testing.T) {
	var out bytes.Buffer
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: &out, Err: &out, NoTTY: true})
	record := releaseRecord{Summary: map[string]any{}}
	planned := false
	stages := []releaseStage{{name: "schema", run: func() (any, error) { planned = true; return "planned", nil }}}
	err := executeRelease(ctx, stages, &record, true, func() error { t.Fatal("dry-run recorded release"); return nil })
	if err != nil || !planned || !strings.Contains(out.String(), "dry-run complete") {
		t.Fatalf("planned=%v output=%s error=%v", planned, out.String(), err)
	}
}

func TestReleaseGitRecordRejectsDirtyCheckout(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	git("init")
	git("-c", "user.name=Release Test", "-c", "user.email=release@example.test", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "fixture")
	clean, err := releaseGitRecord(context.Background(), root, false)
	if err != nil || clean.Commit == "" || clean.Summary["dirty"] != false {
		t.Fatalf("clean=%+v error=%v", clean, err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.yml"), []byte("fields: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := releaseGitRecord(context.Background(), root, false); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("dirty error=%v", err)
	}
	dirty, err := releaseGitRecord(context.Background(), root, true)
	if err != nil || dirty.Commit != clean.Commit || dirty.Summary["dirty"] != true {
		t.Fatalf("dirty=%+v error=%v", dirty, err)
	}
}

func TestReleaseProtectedNoOpSchemaRequiresYes(t *testing.T) {
	root := releaseProjectFixture(t)
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/releases" && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if r.URL.Path != "/products/customizations/plan" {
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte(`{"target":"products","exists":true,"fingerprint":"abc","ops":[]}`))
	}))
	defer server.Close()
	ctx, _, _ := newSitesListTestContext(t, server.URL, output.Mode{JSON: true})
	flags := &RootFlags{APIURL: server.URL, NoInput: true, Environment: "production", SelectedEnvironmentProtected: true}
	err := withWorkingDir(t, root, func() error { return (&ReleaseCmd{Only: []string{"schema,code"}, AllowDirty: true}).Run(ctx, flags) })
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("error=%v", err)
	}
	if !reflect.DeepEqual(requests, []string{"GET /releases", "POST /products/customizations/plan"}) {
		t.Fatalf("requests=%v", requests)
	}
}

func TestReleaseSchemaSuccessRecordsSingleJSONAndSiteFallback(t *testing.T) {
	root := releaseProjectFixture(t)
	var recorded releaseRecord
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/products/customizations/plan":
			_, _ = w.Write([]byte(`{"target":"products","exists":true,"fingerprint":"abc","ops":[]}`))
		case "/releases":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&recorded); err != nil {
				t.Error(err)
			}
			_, _ = w.Write([]byte(`{"id":"release-id","actor":"authenticated-user","created_at":"2026-09-22T10:00:00Z"}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	ctx, out, _ := newSitesListTestContext(t, server.URL, output.Mode{JSON: true})
	err := withWorkingDir(t, root, func() error {
		return (&ReleaseCmd{Only: []string{"schema"}, AllowDirty: true, Yes: true}).Run(ctx, &RootFlags{APIURL: server.URL, NoInput: true})
	})
	if err != nil {
		t.Fatal(err)
	}
	var result releaseRecord
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid single JSON document: %s: %v", out.String(), err)
	}
	if result.ID != "release-id" || result.Actor != "authenticated-user" || result.CreatedAt != "2026-09-22T10:00:00Z" {
		t.Fatalf("missing authoritative server metadata: %+v", result)
	}
	if recorded.Environment != "demo" || !recorded.Forced || recorded.Summary["dirty"] != true {
		t.Fatalf("record=%+v", recorded)
	}
	if _, ok := recorded.Summary["schema"]; !ok {
		t.Fatal("missing schema summary")
	}
}

func releaseProjectFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init"}, {"-c", "user.name=Release Test", "-c", "user.email=release@example.test", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "fixture"}} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %s: %v", out, err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "schema"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"nimbu.yml": "site: demo\n", "schema/products.yml": "fields: []\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
