package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/output"
)

// Without a default macOS keychain the session could not be stored, so
// login must fail before it asks the server for one.
func TestLoginFailsBeforeServerWithoutKeychain(t *testing.T) {
	old := openAuthStore
	openAuthStore = func(string) (auth.Store, error) {
		return nil, fmt.Errorf("%w for HOME=/tmp/scratch", auth.ErrNoDefaultKeychain)
	}
	t.Cleanup(func() { openAuthStore = old })

	for _, tc := range []struct {
		name string
		cmd  AuthLoginCmd
	}{
		{"browser", AuthLoginCmd{}},
		{"device", AuthLoginCmd{Device: true}},
		{"password", AuthLoginCmd{Email: "me@example.com", Password: "sekret"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()

			var stderr bytes.Buffer
			ctx := output.WithWriter(context.Background(), &output.Writer{Out: &bytes.Buffer{}, Err: &stderr, Color: "never"})
			err := tc.cmd.Run(ctx, &RootFlags{APIURL: srv.URL, NoInput: true})
			if !errors.Is(err, auth.ErrNoDefaultKeychain) {
				t.Fatalf("Run() = %v, want ErrNoDefaultKeychain", err)
			}
			if n := hits.Load(); n != 0 {
				t.Fatalf("server got %d requests, want 0", n)
			}
		})
	}
}

func TestClassifyErrorNoDefaultKeychain(t *testing.T) {
	desc := classifyError(fmt.Errorf("open keyring: %w", auth.ErrNoDefaultKeychain))
	if desc.Code != errorKeyringUnavailable {
		t.Fatalf("code = %s, want %s", desc.Code, errorKeyringUnavailable)
	}
	if desc.ExitCode != ExitAuth {
		t.Fatalf("exit = %d, want %d", desc.ExitCode, ExitAuth)
	}
	if !strings.Contains(desc.Hint, "NIMBU_KEYRING_BACKEND=file") {
		t.Fatalf("hint %q does not name the file backend", desc.Hint)
	}
}
