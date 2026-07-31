package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestShippingRatesCommandGrammar(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	valid := [][]string{
		{"shipping-rates", "list", "--locale", "nl"},
		{"shipping-rates", "get", "--rate", "rate-1", "--locale", "nl"},
		{"shipping-rates", "create", "--locale", "nl", "name=Express", "price:=12.5"},
		{"shipping-rates", "create", "--file", "-"},
		{"shipping-rates", "update", "--rate", "rate-1", "--locale", "nl", "name=Express"},
		{"shipping-rates", "update", "--rate", "rate-1", "--file", "-"},
		{"shipping-rates", "delete", "--rate", "rate-1"},
	}
	for _, args := range valid {
		if _, err := parser.Parse(args); err != nil {
			t.Errorf("parse %v: %v", args, err)
		}
	}

	contract := buildCommandContract(parser.Model)
	paths := map[string]bool{}
	for _, command := range contract.Commands {
		paths[command.Path] = true
	}
	for _, path := range []string{
		"nimbu shipping-rates list",
		"nimbu shipping-rates get",
		"nimbu shipping-rates create",
		"nimbu shipping-rates update",
		"nimbu shipping-rates delete",
	} {
		if !paths[path] {
			t.Errorf("command contract missing %q", path)
		}
	}
	for _, path := range []string{"nimbu shipping-rates count", "nimbu shipping-rates copy"} {
		if paths[path] {
			t.Errorf("command contract unexpectedly exposes %q", path)
		}
	}
}

func TestShippingRatesWireContractAndContentLocale(t *testing.T) {
	tests := []struct {
		name       string
		wantMethod string
		wantPath   string
		response   string
		run        func(context.Context, *RootFlags) error
	}{
		{
			name: "list", wantMethod: http.MethodGet, wantPath: "/shipping_rates", response: `[]`,
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&ShippingRatesListCmd{QueryFlags: QueryFlags{Locale: "nl"}}).Run(ctx, flags)
			},
		},
		{
			name: "get", wantMethod: http.MethodGet, wantPath: "/shipping_rates/rate%2F1", response: `{"id":"rate/1"}`,
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&ShippingRatesGetCmd{Rate: "rate/1", Locale: "nl"}).Run(ctx, flags)
			},
		},
		{
			name: "create", wantMethod: http.MethodPost, wantPath: "/shipping_rates", response: `{"id":"rate-1","name":"Express"}`,
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&ShippingRatesCreateCmd{Locale: "nl", Assignments: []string{"name=Express"}}).Run(ctx, flags)
			},
		},
		{
			name: "update", wantMethod: http.MethodPatch, wantPath: "/shipping_rates/rate%2F1", response: `{"id":"rate/1","name":"Express"}`,
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&ShippingRatesUpdateCmd{Rate: "rate/1", Locale: "nl", Assignments: []string{"name=Express"}}).Run(ctx, flags)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != tc.wantMethod {
					t.Errorf("method = %s, want %s", r.Method, tc.wantMethod)
				}
				if r.URL.EscapedPath() != tc.wantPath {
					t.Errorf("path = %q, want %q", r.URL.EscapedPath(), tc.wantPath)
				}
				if got := r.URL.Query().Get("content_locale"); got != "nl" {
					t.Errorf("content_locale = %q, want nl", got)
				}
				if got := r.URL.Query().Get("locale"); got != "" {
					t.Errorf("legacy locale = %q, want empty", got)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()

			ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
			if err := tc.run(ctx, &RootFlags{Site: "demo"}); err != nil {
				t.Fatalf("run: %v", err)
			}
			if requests != 1 {
				t.Fatalf("requests = %d, want 1", requests)
			}
		})
	}
}

func TestShippingRatesJSONIsLossless(t *testing.T) {
	const raw = `[{"id":"rate-1","name":"Fallback","criteria":{"minimum":"10.00"},"price":"7.50","mystery":{"large":9007199254740993},"translations":{"nl":{"name":"Spoed"}}}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(raw))
	}))
	defer server.Close()

	ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	if err := (&ShippingRatesListCmd{}).Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.String(), "9007199254740993") {
		t.Fatalf("output changed large JSON integer: %s", out.String())
	}

	var got any
	var want any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if err := json.Unmarshal([]byte(raw), &want); err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("output lost response fields:\n got %s\nwant %s", gotJSON, wantJSON)
	}
}

func TestShippingRatesHumanAndPlainProjectLocaleWithFallback(t *testing.T) {
	const response = `[
		{"id":"localized","name":"Fallback","criteria":"subtotal","price":"7.50","default":true,"pickup":false,"region_id":"r1","translations":{"nl":{"name":"Spoed","price":"8.25"}}},
		{"id":"fallback","name":"Standard","criteria":"weight","price":"4.00","default":false,"pickup":true,"region_id":"r2","translations":{"fr":{"name":"Normal"}}}
	]`
	for _, tc := range []struct {
		name string
		mode output.Mode
	}{
		{name: "human", mode: output.Mode{}},
		{name: "plain", mode: output.Mode{Plain: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()

			ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, tc.mode)
			command := ShippingRatesListCmd{QueryFlags: QueryFlags{Locale: "nl"}}
			if err := command.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
				t.Fatalf("list: %v", err)
			}
			text := out.String()
			for _, want := range []string{"Spoed", "8.25", "Standard", "4.00"} {
				if !strings.Contains(text, want) {
					t.Errorf("output %q missing %q", text, want)
				}
			}
			if strings.Contains(text, "Fallback") {
				t.Errorf("output retained untranslated localized name: %q", text)
			}
		})
	}
}

func TestShippingRatesRejectTranslationsBeforeRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	file := filepath.Join(t.TempDir(), "rate.json")
	if err := os.WriteFile(file, []byte(`{"name":"Express","translations":{"nl":{"name":"Spoed"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	flags := &RootFlags{Site: "demo"}
	commands := []struct {
		name string
		run  func() error
	}{
		{
			name: "create inline",
			run: func() error {
				return (&ShippingRatesCreateCmd{Assignments: []string{`translations.nl.name=Spoed`}}).Run(ctx, flags)
			},
		},
		{
			name: "update file",
			run: func() error {
				return (&ShippingRatesUpdateCmd{Rate: "rate-1", File: file}).Run(ctx, flags)
			},
		},
	}
	for _, tc := range commands {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), "translations") || !strings.Contains(err.Error(), "--locale") {
				t.Fatalf("error = %v, want translations and --locale hint", err)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
}

func TestShippingRatesDeleteGuardsAndWireContract(t *testing.T) {
	ctx, _, _ := newAdminWorkflowTestContext(t, "http://127.0.0.1:1", output.Mode{})
	command := ShippingRatesDeleteCmd{Rate: "rate/1"}

	if err := command.Run(ctx, &RootFlags{Site: "demo", Readonly: true, Force: true}); err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("readonly error = %v", err)
	}
	if err := command.Run(ctx, &RootFlags{Site: "demo"}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("force error = %v", err)
	}

	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.EscapedPath()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	ctx, _, _ = newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	if err := command.Run(ctx, &RootFlags{Site: "demo", Force: true}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/shipping_rates/rate%2F1" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
}

func TestShippingRatesRejectFileAndAssignments(t *testing.T) {
	ctx, _, _ := newAdminWorkflowTestContext(t, "http://127.0.0.1:1", output.Mode{})
	err := (&ShippingRatesCreateCmd{File: "rate.json", Assignments: []string{"name=Express"}}).
		Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "either --file or inline assignments") {
		t.Fatalf("error = %v", err)
	}
}

func TestShippingRatesCreateFilePreservesExactNumericTokens(t *testing.T) {
	file := filepath.Join(t.TempDir(), "rate.json")
	const payload = `{"order_min":9007199254740993,"price":0.12345678901234567890}`
	if err := os.WriteFile(file, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}

	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = string(body)
		_, _ = w.Write([]byte(`{"id":"rate-1"}`))
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	command := ShippingRatesCreateCmd{File: file}
	if err := command.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if requestBody != payload {
		t.Fatalf("request body changed numeric tokens:\n got %s\nwant %s", requestBody, payload)
	}
}

func TestShippingRatesUpdateTypedAssignmentsPreserveExactNumericTokens(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = string(body)
		_, _ = w.Write([]byte(`{"id":"rate-1"}`))
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	command := ShippingRatesUpdateCmd{
		Rate: "rate-1",
		Assignments: []string{
			"order_max:=9007199254740993",
			"price:=0.12345678901234567890",
		},
	}
	if err := command.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	const want = `{"order_max":9007199254740993,"price":0.12345678901234567890}`
	if requestBody != want {
		t.Fatalf("request body changed numeric tokens:\n got %s\nwant %s", requestBody, want)
	}
}

func TestShippingRatesListAndDetailIncludeDocumentedConstraintFields(t *testing.T) {
	const response = `{"id":"rate-1","name":"Express","criteria":"weight","price":"7.50","default":true,"pickup":false,"region_id":"r1","weight_min":0.12345678901234567890,"weight_max":9007199254740993,"order_min":"10.00","order_max":"20.00","restricted":true,"zipcodes":["1000","2000"]}`

	tests := []struct {
		name     string
		mode     output.Mode
		response string
		run      func(context.Context) error
		wants    []string
	}{
		{
			name:     "list human",
			response: "[" + response + "]",
			run: func(ctx context.Context) error {
				return (&ShippingRatesListCmd{}).Run(ctx, &RootFlags{Site: "demo"})
			},
			wants: []string{"WEIGHT MIN", "WEIGHT MAX", "ORDER MIN", "ORDER MAX", "RESTRICTED", "ZIPCODES", "0.12345678901234567890", "9007199254740993", "1000"},
		},
		{
			name:     "list plain",
			mode:     output.Mode{Plain: true},
			response: "[" + response + "]",
			run: func(ctx context.Context) error {
				return (&ShippingRatesListCmd{}).Run(ctx, &RootFlags{Site: "demo"})
			},
			wants: []string{"0.12345678901234567890", "9007199254740993", "10.00", "20.00", "true", "1000"},
		},
		{
			name:     "detail human",
			response: response,
			run: func(ctx context.Context) error {
				return (&ShippingRatesGetCmd{Rate: "rate-1"}).Run(ctx, &RootFlags{Site: "demo"})
			},
			wants: []string{"Weight Min:", "Weight Max:", "Order Min:", "Order Max:", "Restricted:", "Zipcodes:", "0.12345678901234567890", "9007199254740993", "1000"},
		},
		{
			name:     "detail plain",
			mode:     output.Mode{Plain: true},
			response: response,
			run: func(ctx context.Context) error {
				return (&ShippingRatesGetCmd{Rate: "rate-1"}).Run(ctx, &RootFlags{Site: "demo"})
			},
			wants: []string{"0.12345678901234567890", "9007199254740993", "10.00", "20.00", "true", "1000"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()

			ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, tc.mode)
			if err := tc.run(ctx); err != nil {
				t.Fatalf("run: %v", err)
			}
			for _, want := range tc.wants {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output %q missing %q", out.String(), want)
				}
			}
		})
	}
}
