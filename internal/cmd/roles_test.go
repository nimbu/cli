package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func relationRoleJSON(id, name string, customerIDs []string) []byte {
	objects := make([]map[string]any, len(customerIDs))
	for i, customerID := range customerIDs {
		objects[i] = map[string]any{
			"__type":    "Reference",
			"className": "customer",
			"id":        customerID,
		}
	}
	body, err := json.Marshal(map[string]any{
		"id":   id,
		"name": name,
		"customers": map[string]any{
			"__type":    "Relation",
			"className": "customer",
			"objects":   objects,
		},
		"children": []string{},
		"parents":  []string{},
	})
	if err != nil {
		panic(err)
	}
	return body
}

func TestRolesGetDecodesRelationMembers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/roles/bingo" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write(relationRoleJSON("bingo", "bingo", []string{"c1", "c2"}))
	}))
	t.Cleanup(server.Close)

	ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	if err := (&RolesGetCmd{Role: "bingo"}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("roles get: %v", err)
	}

	var role struct {
		Customers []string `json:"customers"`
	}
	if err := json.Unmarshal(out.Bytes(), &role); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	if !slices.Equal(role.Customers, []string{"c1", "c2"}) {
		t.Fatalf("customers = %#v", role.Customers)
	}
}

func TestRolesListDecodesRelationMembers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/roles" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte("[" + string(relationRoleJSON("bingo", "bingo", []string{"c1"})) + "]"))
	}))
	t.Cleanup(server.Close)

	ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	if err := (&RolesListCmd{}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("roles list: %v", err)
	}

	var roles []struct {
		Customers []string `json:"customers"`
	}
	if err := json.Unmarshal(out.Bytes(), &roles); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	if len(roles) != 1 || !slices.Equal(roles[0].Customers, []string{"c1"}) {
		t.Fatalf("roles = %#v", roles)
	}
}

func TestRolesUpdateDecodesRelationMembers(t *testing.T) {
	members := []string{"c1", "c2"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/roles/bingo":
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", members))
		case r.Method == http.MethodPut && r.URL.Path == "/roles/bingo":
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", members))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	file := filepath.Join(t.TempDir(), "patch.json")
	if err := os.WriteFile(file, []byte(`{"description":"ok"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&RolesUpdateCmd{Role: "bingo", File: file}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("roles update: %v", err)
	}
	if !strings.Contains(out.String(), `"c1"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestRolesCustomersAddRemoveSet(t *testing.T) {
	var mu sync.Mutex
	members := []string{"c1", "c2"}
	var lastPut []string
	var puts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/roles/bingo":
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", members))
		case r.Method == http.MethodPut && r.URL.Path == "/roles/bingo":
			var body struct {
				Customers []string `json:"customers"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			lastPut = slices.Clone(body.Customers)
			slices.Sort(lastPut)
			members = slices.Clone(body.Customers)
			puts++
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", members))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	t.Run("add", func(t *testing.T) {
		ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
		err := (&RolesCustomersAddCmd{Role: "bingo", Customers: []string{"c3"}}).Run(ctx, &RootFlags{})
		if err != nil {
			t.Fatalf("add: %v", err)
		}
		if !slices.Equal(lastPut, []string{"c1", "c2", "c3"}) {
			t.Fatalf("put = %#v", lastPut)
		}
		if !strings.Contains(out.String(), "2 → 3") {
			t.Fatalf("output = %s", out.String())
		}
	})

	t.Run("remove", func(t *testing.T) {
		ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
		err := (&RolesCustomersRemoveCmd{Role: "bingo", Customers: []string{"c3"}}).Run(ctx, &RootFlags{})
		if err != nil {
			t.Fatalf("remove: %v", err)
		}
		if !slices.Equal(lastPut, []string{"c1", "c2"}) {
			t.Fatalf("put = %#v", lastPut)
		}
		if !strings.Contains(out.String(), "3 → 2") {
			t.Fatalf("output = %s", out.String())
		}
	})

	t.Run("set", func(t *testing.T) {
		ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
		err := (&RolesCustomersSetCmd{Role: "bingo", Customers: []string{"c9", "c8"}}).Run(ctx, &RootFlags{})
		if err != nil {
			t.Fatalf("set: %v", err)
		}
		if !slices.Equal(lastPut, []string{"c8", "c9"}) {
			t.Fatalf("put = %#v", lastPut)
		}
		if !strings.Contains(out.String(), "2 → 2") {
			t.Fatalf("output = %s", out.String())
		}
	})

	if puts != 3 {
		t.Fatalf("puts = %d", puts)
	}
}

func TestRolesUpdateRefusesToShrinkRelationOverHalfWithoutForce(t *testing.T) {
	members := []string{"c1", "c2", "c3", "c4"}
	var put bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/roles/bingo":
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", members))
		case r.Method == http.MethodPut:
			put = true
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", []string{"c1"}))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	file := filepath.Join(t.TempDir(), "patch.json")
	if err := os.WriteFile(file, []byte(`{"customers":["c1"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	err := (&RolesUpdateCmd{Role: "bingo", File: file}).Run(ctx, &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected --force error, got %v", err)
	}
	if put {
		t.Fatal("write should not have been sent")
	}

	ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	if err := (&RolesUpdateCmd{Role: "bingo", File: file}).Run(ctx, &RootFlags{Force: true}); err != nil {
		t.Fatalf("forced update: %v", err)
	}
	if !put {
		t.Fatal("expected write with --force")
	}
	if !strings.Contains(out.String(), "4 → 1") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestRolesUpdateDryRunPrintsBodyWithoutWriting(t *testing.T) {
	var put bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/roles/bingo":
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", []string{"c1", "c2"}))
		case r.Method == http.MethodPut:
			put = true
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	file := filepath.Join(t.TempDir(), "patch.json")
	if err := os.WriteFile(file, []byte(`{"customers":["c1","c2","c3"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	err := (&RolesUpdateCmd{Role: "bingo", File: file, DryRun: true}).Run(ctx, &RootFlags{})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if put {
		t.Fatal("dry-run must not write")
	}
	if !strings.Contains(out.String(), `"customers"`) || !strings.Contains(out.String(), "2 → 3") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestRolesUpdateWarnsOnSuccessfulDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/roles/bingo":
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", []string{"c1"}))
		case r.Method == http.MethodPut && r.URL.Path == "/roles/bingo":
			_, _ = w.Write([]byte(`{"id":"bingo","name":"bingo","customers":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	file := filepath.Join(t.TempDir(), "patch.json")
	if err := os.WriteFile(file, []byte(`{"description":"ok"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, out, errOut := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	err := (&RolesUpdateCmd{Role: "bingo", File: file}).Run(ctx, &RootFlags{})
	if err != nil {
		t.Fatalf("roles update: %v", err)
	}
	if !strings.Contains(errOut.String(), "request succeeded (HTTP 200) but the response could not be decoded:") {
		t.Fatalf("stderr = %s", errOut.String())
	}
	if !strings.Contains(out.String(), "Updated role") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestRolesCustomersAddWarnsOnSuccessfulDecodeError(t *testing.T) {
	var gets int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/roles/bingo":
			gets++
			if gets == 1 {
				_, _ = w.Write(relationRoleJSON("bingo", "bingo", []string{"c1"}))
				return
			}
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", []string{"c1", "c2"}))
		case r.Method == http.MethodPut && r.URL.Path == "/roles/bingo":
			_, _ = w.Write([]byte(`{"id":"bingo","name":"bingo","customers":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx, out, errOut := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	err := (&RolesCustomersAddCmd{Role: "bingo", Customers: []string{"c2"}}).Run(ctx, &RootFlags{})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(errOut.String(), "request succeeded (HTTP 200) but the response could not be decoded:") {
		t.Fatalf("stderr = %s", errOut.String())
	}
	if !strings.Contains(out.String(), "Updated role customers") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestRolesUpdateRefusesNullRelationWithoutForce(t *testing.T) {
	members := []string{"c1", "c2", "c3", "c4"}
	var put bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/roles/bingo":
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", members))
		case r.Method == http.MethodPut:
			put = true
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", nil))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	file := filepath.Join(t.TempDir(), "patch.json")
	if err := os.WriteFile(file, []byte(`{"customers":null}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	err := (&RolesUpdateCmd{Role: "bingo", File: file}).Run(ctx, &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected --force error, got %v", err)
	}
	if put {
		t.Fatal("write should not have been sent")
	}

	ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	if err := (&RolesUpdateCmd{Role: "bingo", File: file, DryRun: true}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if put {
		t.Fatal("dry-run must not write")
	}
	if !strings.Contains(out.String(), "4 → 0") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestRolesUpdateDryRunShowsUnknownBeforeForUnexpandedRelation(t *testing.T) {
	var put bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/roles/bingo":
			_, _ = w.Write([]byte(`{"id":"bingo","name":"bingo","customers":{"__type":"Relation","className":"customer"}}`))
		case r.Method == http.MethodPut:
			put = true
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	file := filepath.Join(t.TempDir(), "patch.json")
	if err := os.WriteFile(file, []byte(`{"customers":["c1","c2"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	err := (&RolesUpdateCmd{Role: "bingo", File: file, DryRun: true}).Run(ctx, &RootFlags{})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if put {
		t.Fatal("dry-run must not write")
	}
	if !strings.Contains(out.String(), "customers: ? → 2") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestRolesUpdateDryRunAllowsShrinkPreviewWithoutForce(t *testing.T) {
	var put bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/roles/bingo":
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", []string{"c1", "c2", "c3", "c4"}))
		case r.Method == http.MethodPut:
			put = true
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	file := filepath.Join(t.TempDir(), "patch.json")
	if err := os.WriteFile(file, []byte(`{"customers":["c1"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	err := (&RolesUpdateCmd{Role: "bingo", File: file, DryRun: true}).Run(ctx, &RootFlags{})
	if err != nil {
		t.Fatalf("dry-run shrink preview: %v", err)
	}
	if put {
		t.Fatal("dry-run must not write")
	}
	if !strings.Contains(out.String(), "4 → 1") || !strings.Contains(out.String(), `"customers"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestRolesUpdatePassesUnknownRelationOpThrough(t *testing.T) {
	var putBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/roles/bingo":
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", []string{"c1", "c2"}))
		case r.Method == http.MethodPut && r.URL.Path == "/roles/bingo":
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", []string{"c1", "c2"}))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	file := filepath.Join(t.TempDir(), "patch.json")
	if err := os.WriteFile(file, []byte(`{"customers":{"__op":"AddUnique","objects":[{"id":"c3"}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	if err := (&RolesUpdateCmd{Role: "bingo", File: file}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("unknown op update: %v", err)
	}
	op, _ := putBody["customers"].(map[string]any)
	if op["__op"] != "AddUnique" {
		t.Fatalf("put body = %#v", putBody)
	}
}

func TestRolesCustomersAddRefusesUnexpandedRelation(t *testing.T) {
	var put bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/roles/bingo":
			_, _ = w.Write([]byte(`{"id":"bingo","name":"bingo","customers":{"__type":"Relation","className":"customer"}}`))
		case r.Method == http.MethodPut:
			put = true
			_, _ = w.Write([]byte(`{"id":"bingo"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	err := (&RolesCustomersAddCmd{Role: "bingo", Customers: []string{"c1"}}).Run(ctx, &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "not expanded") {
		t.Fatalf("expected unexpanded error, got %v", err)
	}
	if put {
		t.Fatal("must not replace an unexpanded relation")
	}
}

func TestRolesCustomersSetRefusesOverHalfShrinkWithoutForce(t *testing.T) {
	members := []string{"c1", "c2", "c3", "c4"}
	var put bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/roles/bingo":
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", members))
		case r.Method == http.MethodPut:
			put = true
			_, _ = w.Write(relationRoleJSON("bingo", "bingo", []string{"c1"}))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{})
	err := (&RolesCustomersSetCmd{Role: "bingo", Customers: []string{"c1"}}).Run(ctx, &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected --force error, got %v", err)
	}
	if put {
		t.Fatal("write should not have been sent")
	}
}
