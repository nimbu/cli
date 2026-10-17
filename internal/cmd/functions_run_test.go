package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestFunctionsRunPositionalNameSuggestsFlag(t *testing.T) {
	ctx, _ := newAppsTestContextWithOut(t, "https://api.example.test")

	cmd := &FunctionsRunCmd{Assignments: []string{"calculateShipping", "qty:=2"}}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "--function calculateShipping") {
		t.Fatalf("expected flag suggestion, got %v", err)
	}
}

func TestFunctionsRunMissingFunctionErrors(t *testing.T) {
	ctx, _ := newAppsTestContextWithOut(t, "https://api.example.test")

	cmd := &FunctionsRunCmd{Assignments: []string{"foo=bar"}}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "--function <function>") {
		t.Fatalf("expected function-name usage error, got %v", err)
	}
}

func TestFunctionsRunSendsFlatBody(t *testing.T) {
	srv, captured := jobsTestServer(t, http.StatusOK, `{"ok":true}`)
	ctx, _ := newAppsTestContextWithOut(t, srv.URL)

	cmd := &FunctionsRunCmd{Function: "calculateShipping", Assignments: []string{"qty:=2"}}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL}); err != nil {
		t.Fatalf("run function: %v", err)
	}

	if captured.method != http.MethodPost || captured.path != "/functions/calculateShipping" {
		t.Fatalf("request = %s %s, want POST /functions/calculateShipping", captured.method, captured.path)
	}
	if captured.body["qty"] != float64(2) {
		t.Fatalf("qty = %v, want 2", captured.body["qty"])
	}
	if _, exists := captured.body["params"]; exists {
		t.Fatalf("body should not wrap params in an envelope: %#v", captured.body)
	}
}

func TestFunctionsRunPlainPrintsValidCompactJSON(t *testing.T) {
	srv, _ := jobsTestServer(t, http.StatusOK, `{"message":"He said \"hi\"\nC:\\tmp <b>"}`)
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{Plain: true})

	cmd := &FunctionsRunCmd{Function: "hello", Assignments: []string{"x=1"}}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL}); err != nil {
		t.Fatalf("run function: %v", err)
	}

	got := out.String()
	if strings.Count(got, "\n") != 1 || !strings.HasSuffix(got, "\n") {
		t.Fatalf("want one line, got %q", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("--plain output is not valid JSON: %v\n%s", err, got)
	}
	if want := "He said \"hi\"\nC:\\tmp <b>"; decoded["message"] != want {
		t.Fatalf("message = %q, want %q", decoded["message"], want)
	}
}
