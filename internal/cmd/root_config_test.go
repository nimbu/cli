package cmd

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestApplyRootConfigDefaultsUsesConfigWhenNoOverrides(t *testing.T) {
	flags := RootFlags{
		APIURL:  "https://api.nimbu.io",
		Timeout: 30 * time.Second,
	}
	cfg := config.Config{
		APIURL:  "https://api.example.test",
		Timeout: "12s",
	}

	got := applyRootConfigDefaults(flags, cfg, nil)
	if got.APIURL != cfg.APIURL {
		t.Fatalf("api_url mismatch: %q", got.APIURL)
	}
	if got.Timeout != 12*time.Second {
		t.Fatalf("timeout mismatch: %s", got.Timeout)
	}
}

func TestApplyRootConfigDefaultsRespectsCLIAndEnvOverrides(t *testing.T) {
	flags := RootFlags{
		APIURL:  "https://api.flag.test",
		Timeout: 7 * time.Second,
	}
	cfg := config.Config{
		APIURL:  "https://api.config.test",
		Timeout: "42s",
	}

	t.Setenv("NIMBU_API_URL", "https://api.env.test")
	t.Setenv("NIMBU_TIMEOUT", "9s")

	got := applyRootConfigDefaults(flags, cfg, []string{"--apiurl=https://api.flag.test", "--timeout=7s"})
	if got.APIURL != flags.APIURL {
		t.Fatalf("api_url should keep CLI/env value, got %q", got.APIURL)
	}
	if got.Timeout != flags.Timeout {
		t.Fatalf("timeout should keep CLI/env value, got %s", got.Timeout)
	}
}

func TestApplyRootConfigDefaultsIgnoresInvalidConfigTimeout(t *testing.T) {
	flags := RootFlags{Timeout: 30 * time.Second}
	cfg := config.Config{Timeout: "nope"}

	got := applyRootConfigDefaults(flags, cfg, nil)
	if got.Timeout != flags.Timeout {
		t.Fatalf("timeout should stay unchanged on invalid config value, got %s", got.Timeout)
	}
}

func TestHasCLIFlag(t *testing.T) {
	args := []string{"channels", "list", "--apiurl=https://api.example.test", "--timeout", "10s"}
	if !hasCLIFlag(args, "--apiurl") {
		t.Fatal("expected --apiurl to be detected")
	}
	if !hasCLIFlag(args, "--timeout") {
		t.Fatal("expected --timeout to be detected")
	}
	if hasCLIFlag(args, "--site") {
		t.Fatal("did not expect --site to be detected")
	}
}

func TestApplyRootConfigDefaultsWithoutEnvUsesConfig(t *testing.T) {
	flags := RootFlags{APIURL: "https://api.nimbu.io"}
	cfg := config.Config{APIURL: "https://api.config.test"}

	_ = os.Unsetenv("NIMBU_API_URL")
	got := applyRootConfigDefaults(flags, cfg, nil)
	if got.APIURL != "https://api.config.test" {
		t.Fatalf("expected config api_url, got %q", got.APIURL)
	}
}

func TestAPIClientFactoriesPropagateReadonly(t *testing.T) {
	store := &fakeAuthStore{credential: auth.Credential{Token: "test-token"}}
	withFakeAuthStore(t, store)

	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, &RootFlags{
		APIURL:   "https://api.example.test",
		Timeout:  30 * time.Second,
		Readonly: true,
	})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})
	ctx = context.WithValue(ctx, authResolverKey{}, newAuthCredentialResolver("api.example.test"))
	ctx = output.WithMode(ctx, output.Mode{})

	clients := map[string]func() error{
		"default": func() error {
			client, err := GetAPIClient(ctx)
			if err != nil {
				return err
			}
			return client.Post(ctx, "/channels", nil, nil)
		},
		"site": func() error {
			client, err := GetAPIClientWithSite(ctx, "site-1")
			if err != nil {
				return err
			}
			return client.Post(ctx, "/channels", nil, nil)
		},
		"base_url": func() error {
			client, err := GetAPIClientWithBaseURL(ctx, "https://api.other.test", "site-1")
			if err != nil {
				return err
			}
			return client.Post(ctx, "/channels", nil, nil)
		},
		"theme_copy_helper": func() error {
			client, err := newAPIClientForBase(ctx, "https://api.other.test", "site-1")
			if err != nil {
				return err
			}
			return client.Post(ctx, "/channels", nil, nil)
		},
	}

	for name, run := range clients {
		t.Run(name, func(t *testing.T) {
			err := run()
			if err == nil {
				t.Fatal("expected readonly error")
			}
			var readonlyErr *api.ReadonlyError
			if !errors.As(err, &readonlyErr) {
				t.Fatalf("error = %T %v, want *api.ReadonlyError", err, err)
			}
		})
	}
}
