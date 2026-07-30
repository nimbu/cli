package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestSettingsConsentConfigCommandsAreInContract(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}
	paths := map[string]bool{}
	for _, command := range buildCommandContract(parser.Model).Commands {
		paths[command.Path] = true
	}
	for _, path := range []string{
		"nimbu settings consent config get",
		"nimbu settings consent config update",
		"nimbu settings consent config replace",
		"nimbu settings consent config copy",
	} {
		if !paths[path] {
			t.Errorf("command contract missing %q", path)
		}
	}
}

func TestSettingsConsentConfigReplaceContractRequiresFileAndHasNoAssignments(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}
	for _, command := range buildCommandContract(parser.Model).Commands {
		if command.Path != "nimbu settings consent config replace" {
			continue
		}
		if len(command.Arguments) != 0 {
			t.Fatalf("replace arguments = %#v, want none", command.Arguments)
		}
		for _, flag := range command.Flags {
			if flag.Name == "file" {
				if !flag.Required {
					t.Fatal("replace --file must be required in the command contract")
				}
				return
			}
		}
		t.Fatal("replace --file missing from command contract")
	}
	t.Fatal("replace command missing from contract")
}

func TestSettingsConsentConfigGetPreservesLocalizedFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/settings/consent" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"privacy_policy_url":{"en":"/privacy","nl":"/privacybeleid"},"purposes":[{"key":"analytics","description":{"en":"Analytics","nl":"Analyse"}}]}`))
	}))
	defer srv.Close()

	for name, mode := range map[string]output.Mode{
		"human": {},
		"plain": {Plain: true},
		"json":  {JSON: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, out, _ := newAdminWorkflowTestContext(t, srv.URL, mode)
			if err := (&SettingsConsentConfigGetCmd{}).Run(ctx); err != nil {
				t.Fatalf("get consent config: %v", err)
			}
			for _, value := range []string{`"nl": "/privacybeleid"`, `"nl": "Analyse"`} {
				if !strings.Contains(out.String(), value) {
					t.Fatalf("output %q does not contain %q", out.String(), value)
				}
			}
		})
	}
}

func TestSettingsConsentConfigReplaceAcceptsStdinAndRejectsEmptyStdin(t *testing.T) {
	originalStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = originalStdin })

	for _, tt := range []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "JSON object", content: `{"enabled":true}`},
		{name: "empty", wantErr: "no JSON input"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stdin, err := os.CreateTemp(t.TempDir(), "stdin")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = stdin.Close() }()
			if _, err := stdin.WriteString(tt.content); err != nil {
				t.Fatal(err)
			}
			if _, err := stdin.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			os.Stdin = stdin

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
			err = (&SettingsConsentConfigReplaceCmd{File: "-"}).Run(ctx, &RootFlags{Site: "demo", Force: true, NoInput: true})
			if tt.wantErr == "" && err != nil {
				t.Fatalf("replace from stdin: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSettingsConsentConfigCopyHonorsReadonly(t *testing.T) {
	ctx, _, _ := newAdminWorkflowTestContext(t, "https://api.example.test", output.Mode{})
	err := (&SettingsConsentConfigCopyCmd{From: "source", To: "target"}).Run(ctx, &RootFlags{Readonly: true, NoInput: true})
	if err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("error = %v, want readonly guard", err)
	}
}

func TestSettingsConsentConfigCopyDryRunReadsWithoutWriting(t *testing.T) {
	var puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/settings/consent":
			_, _ = w.Write([]byte(`{"enabled":true}`))
		case r.Method == http.MethodPut:
			puts++
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &SettingsConsentConfigCopyCmd{From: "source", To: "target", FromHost: srv.URL, ToHost: srv.URL, DryRun: true}
	if err := cmd.Run(ctx, &RootFlags{Readonly: true, NoInput: true}); err != nil {
		t.Fatalf("dry-run consent config copy: %v", err)
	}
	if puts != 0 {
		t.Fatalf("PUT calls = %d, want 0", puts)
	}
	if !strings.Contains(out.String(), `"action": "dry-run:update"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestSettingsConsentConfigUpdatePatchesInlineAssignments(t *testing.T) {
	var method string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"enabled":true}`))
	}))
	defer srv.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &SettingsConsentConfigUpdateCmd{Assignments: []string{"enabled:=true", "privacy_policy_url.nl=/privacybeleid"}}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("update consent config: %v", err)
	}
	if method != http.MethodPatch {
		t.Fatalf("method = %q, want PATCH", method)
	}
	urls, _ := body["privacy_policy_url"].(map[string]any)
	if body["enabled"] != true || urls["nl"] != "/privacybeleid" {
		t.Fatalf("body = %#v", body)
	}
}

func TestSettingsConsentConfigReplaceGuardsWholeConfigWrite(t *testing.T) {
	ctx, _, _ := newAdminWorkflowTestContext(t, "https://api.example.test", output.Mode{})
	tests := []struct {
		name string
		cmd  SettingsConsentConfigReplaceCmd
		flag RootFlags
		want string
	}{
		{name: "file required", cmd: SettingsConsentConfigReplaceCmd{}, flag: RootFlags{Force: true}, want: "--file is required"},
		{name: "force required", cmd: SettingsConsentConfigReplaceCmd{File: "config.json"}, want: "--force"},
		{name: "readonly", cmd: SettingsConsentConfigReplaceCmd{File: "config.json"}, flag: RootFlags{Force: true, Readonly: true}, want: "readonly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Run(ctx, &tt.flag)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestSettingsConsentConfigReplacePutsFileLosslessly(t *testing.T) {
	file := t.TempDir() + "/consent.json"
	const payload = `{"privacy_policy_url":{"en":"/privacy","nl":"/privacybeleid"},"cookies":[{"id":"c1","name":"session","metadata":{"same_site":"lax"}}],"future_counter":9007199254740993}`
	if err := os.WriteFile(file, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	var method string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		_ = decoder.Decode(&body)
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	if err := (&SettingsConsentConfigReplaceCmd{File: file}).Run(ctx, &RootFlags{Site: "demo", Force: true, NoInput: true}); err != nil {
		t.Fatalf("replace consent config: %v", err)
	}
	if method != http.MethodPut {
		t.Fatalf("method = %q, want PUT", method)
	}
	urls := body["privacy_policy_url"].(map[string]any)
	if urls["nl"] != "/privacybeleid" {
		t.Fatalf("localized URLs lost: %#v", body)
	}
	if body["future_counter"] != json.Number("9007199254740993") {
		t.Fatalf("large integer lost: %#v", body["future_counter"])
	}
}

func TestSettingsConsentConfigReplaceRejectsTrailingJSONWithoutPut(t *testing.T) {
	file := t.TempDir() + "/consent.json"
	if err := os.WriteFile(file, []byte(`{"enabled":true}{"enabled":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts++
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	err := (&SettingsConsentConfigReplaceCmd{File: file}).Run(ctx, &RootFlags{Site: "demo", Force: true, NoInput: true})
	if err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("error = %v, want trailing JSON error", err)
	}
	if puts != 0 {
		t.Fatalf("PUT calls = %d, want 0", puts)
	}
}

func TestSettingsConsentConfigUpdateReadonlyDoesNotConsumeStdin(t *testing.T) {
	originalStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = originalStdin })
	stdin, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close() }()
	if _, err := stdin.WriteString(`{"enabled":true}`); err != nil {
		t.Fatal(err)
	}
	if _, err := stdin.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	os.Stdin = stdin

	ctx, _, _ := newAdminWorkflowTestContext(t, "https://api.example.test", output.Mode{})
	err = (&SettingsConsentConfigUpdateCmd{File: "-"}).Run(ctx, &RootFlags{Readonly: true, NoInput: true})
	if err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("error = %v, want readonly guard", err)
	}
	offset, err := stdin.Seek(0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 0 {
		t.Fatalf("stdin offset = %d, want 0 (input must not be consumed)", offset)
	}
}
