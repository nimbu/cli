package cmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestAuthLogoutBypassesReadonlyForSessionRevocation(t *testing.T) {
	var logoutRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/logout" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		logoutRequests++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	store := &fakeAuthStore{credential: auth.Credential{Token: "test-token"}}
	withFakeAuthStore(t, store)

	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, &RootFlags{
		APIURL:   srv.URL,
		Timeout:  30 * time.Second,
		Readonly: true,
	})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})
	ctx = context.WithValue(ctx, authResolverKey{}, newAuthCredentialResolver("api.example.test"))
	ctx = output.WithMode(ctx, output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: io.Discard, Err: io.Discard})

	if err := (&AuthLogoutCmd{}).Run(ctx); err != nil {
		t.Fatalf("auth logout: %v", err)
	}
	if logoutRequests != 1 {
		t.Fatalf("logout requests = %d, want 1", logoutRequests)
	}
	if store.deleteCredentialCalls != 1 {
		t.Fatalf("delete credential calls = %d, want 1", store.deleteCredentialCalls)
	}
}
