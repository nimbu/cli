package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// AuthKeyringCmd manages keyring backend.
type AuthKeyringCmd struct {
	Show AuthKeyringShowCmd `cmd:"" default:"1" help:"Show current keyring backend"`
	Set  AuthKeyringSetCmd  `cmd:"" help:"Set keyring backend"`
}

// AuthKeyringShowCmd shows the current keyring backend.
type AuthKeyringShowCmd struct{}

// Run shows the current keyring backend.
func (c *AuthKeyringShowCmd) Run(ctx context.Context) error {
	backend, source, err := auth.ResolveBackend()
	if err != nil {
		return err
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, map[string]string{
			"backend": backend,
			"source":  source,
		})
	}

	if mode.Plain {
		return output.Plain(ctx, backend, source)
	}

	fmt.Printf("Backend: %s\n", backend)
	fmt.Printf("Source:  %s\n", source)
	return nil
}

// AuthKeyringSetCmd sets the keyring backend.
type AuthKeyringSetCmd struct {
	Backend string `arg:"" help:"Backend to use: auto, keychain, file, secret-service, kwallet, wincred"`
}

// Run sets the keyring backend.
func (c *AuthKeyringSetCmd) Run(ctx context.Context) error {
	// Validate backend
	valid := map[string]bool{
		"auto":           true,
		"keychain":       true,
		"file":           true,
		"secret-service": true,
		"kwallet":        true,
		"wincred":        true,
	}
	if !valid[c.Backend] {
		return fmt.Errorf("invalid backend %q; valid options: auto, keychain, file, secret-service, kwallet, wincred", c.Backend)
	}

	// Update config
	cfg, err := config.Read()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	cfg.KeyringBackend = c.Backend

	if err := config.Write(cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload(fmt.Sprintf("keyring backend set to %s", c.Backend)))
	}

	fmt.Printf("Keyring backend set to %s\n", c.Backend)
	return nil
}
