package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

func TestPhase1ListFooterSnapshots(t *testing.T) {
	resources := []string{
		"products", "orders", "customers", "collections", "coupons",
		"pages", "menus", "notifications", "blogs", "articles",
		"channels", "entries", "uploads", "webhooks", "sites",
	}

	var sb strings.Builder

	for _, mode := range []struct {
		name string
		cfg  output.Mode
	}{
		{name: "human", cfg: output.Mode{}},
		{name: "json", cfg: output.Mode{JSON: true}},
		{name: "plain", cfg: output.Mode{Plain: true}},
	} {
		fmt.Fprintf(&sb, "## mode=%s\n", mode.name)
		for _, resource := range resources {
			buf := &bytes.Buffer{}
			ctx := output.WithWriter(context.Background(), &output.Writer{Out: buf, Err: buf, Mode: mode.cfg})
			ctx = output.WithMode(ctx, mode.cfg)
			meta := listFooterMeta{Page: 1, PerPage: 25, Returned: 25, Total: 100, TotalKnown: true}
			if err := writeListFooter(ctx, resource, meta); err != nil {
				t.Fatalf("write footer: %v", err)
			}
			line := strings.TrimSpace(buf.String())
			if line == "" {
				line = "<empty>"
			}
			fmt.Fprintf(&sb, "%s => %s\n", resource, line)
		}
		sb.WriteString("\n")
	}

	assertGolden(t, "phase1_list_footer.golden", sb.String())
}

func TestPhase1ErrorContractSnapshots(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "api_not_found", err: &api.Error{StatusCode: 404, Message: "not found", Code: "object_not_found"}},
		{name: "api_rate_limited", err: &api.Error{StatusCode: 429, Message: "rate limited"}},
		{name: "scope_missing", err: &scopeMissingError{Required: []string{"read_products"}, Sample: "Example: nimbu auth scopes"}},
		{name: "validation_local", err: fmt.Errorf("--file required")},
		{name: "site_required", err: siteRequiredError(errProjectHasNoSite)},
		{name: "confirmation_required", err: requireForce(&RootFlags{}, "channel news")},
		{name: "network_refused", err: &api.Error{Message: "dial tcp: connection refused", Err: &url.Error{Op: "Get", URL: "https://api.test/sites", Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}}}},
		{name: "network_timeout", err: &api.Error{Message: "deadline", Err: &url.Error{Op: "Get", URL: "https://api.test/sites", Err: context.DeadlineExceeded}}},
		{name: "local_text_mentions_timeout", err: fmt.Errorf("parse timeout: connection setting unknown")},
		{name: "interrupted", err: &interruptedError{err: fmt.Errorf("push theme: upload failed")}},
	}

	var sb strings.Builder
	for _, tc := range tests {
		fmt.Fprintf(&sb, "## %s\n", tc.name)
		desc := classifyError(tc.err)
		data, err := json.MarshalIndent(desc, "", "  ")
		if err != nil {
			t.Fatalf("marshal descriptor: %v", err)
		}
		sb.WriteString(string(data))
		sb.WriteString("\n\n")
	}

	assertGolden(t, "phase1_error_contract.golden", sb.String())
}
