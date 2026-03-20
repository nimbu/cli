package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/output"
)

// AuthStatusCmd shows authentication status.
type AuthStatusCmd struct{}

// Run executes the status command.
func (c *AuthStatusCmd) Run(ctx context.Context, flags *RootFlags) error {
	cred, err := ResolveAuthCredential(ctx)
	hasToken := true
	if errors.Is(err, auth.ErrNoToken) {
		hasToken = false
	} else if err != nil {
		return err
	}

	host := apps.NormalizeHost(flags.APIURL)
	useColor := output.WriterFromContext(ctx).UseColor()

	// Structured output modes
	mode := output.FromContext(ctx)

	if !hasToken {
		return c.renderNotLoggedIn(ctx, mode, host, useColor)
	}

	// Verify token against API and fetch user info
	var user api.User
	verified := false
	if client, err := GetAPIClient(ctx); err == nil {
		if err := client.Get(ctx, "/user", &user); err == nil {
			verified = true
		}
	}

	email := cred.Email
	if verified && user.Email != "" {
		email = user.Email
	}
	name := user.Name

	if mode.JSON {
		return output.JSON(ctx, map[string]any{
			"logged_in": true,
			"verified":  verified,
			"email":     email,
			"name":      name,
			"host":      host,
		})
	}

	if mode.Plain {
		if verified {
			return output.Plain(ctx, "logged_in", email, name, host)
		}
		return output.Plain(ctx, "logged_in_unverified", email, host)
	}

	if verified {
		symbol := colorSymbol("✓", "#22c55e", useColor)
		if name != "" {
			fmt.Printf("%s Logged in to %s as %s (%s)\n", symbol, bold(host, useColor), bold(name, useColor), email)
		} else {
			fmt.Printf("%s Logged in to %s as %s\n", symbol, bold(host, useColor), bold(email, useColor))
		}
	} else {
		symbol := colorSymbol("!", "#f59e0b", useColor)
		if email != "" {
			fmt.Printf("%s Logged in to %s as %s (unverified)\n", symbol, bold(host, useColor), bold(email, useColor))
		} else {
			fmt.Printf("%s Logged in to %s (unverified)\n", symbol, bold(host, useColor))
		}
	}

	return nil
}

func (c *AuthStatusCmd) renderNotLoggedIn(ctx context.Context, mode output.Mode, host string, useColor bool) error {
	if mode.JSON {
		return output.JSON(ctx, map[string]any{
			"logged_in": false,
			"verified":  false,
			"email":     "",
			"name":      "",
			"host":      host,
		})
	}
	if mode.Plain {
		return output.Plain(ctx, "logged_out", "", host)
	}

	symbol := colorSymbol("✗", "#ef4444", useColor)
	fmt.Printf("%s Not logged in to %s\n", symbol, bold(host, useColor))
	return nil
}

func colorSymbol(symbol, color string, useColor bool) string {
	if !useColor {
		return symbol
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(symbol)
}

func bold(s string, useColor bool) string {
	if !useColor {
		return s
	}
	return lipgloss.NewStyle().Bold(true).Render(s)
}
