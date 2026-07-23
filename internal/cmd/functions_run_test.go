package cmd

import (
	"net/http"
	"strings"
	"testing"
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
