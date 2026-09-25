package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/oauth"
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
		return c.renderNotLoggedIn(ctx, mode, host, useColor, false)
	}

	// Verify token against API and fetch user info
	var user api.User
	var verifyErr error
	if client, err := GetAPIClient(ctx); err == nil {
		verifyErr = client.Get(ctx, "/user", &user)
	} else {
		verifyErr = err
	}
	verified := verifyErr == nil

	// Verifying may have refreshed an OAuth session, or found it revoked and
	// deleted it; show what is stored now.
	current, err := ResolveAuthCredential(ctx)
	switch {
	case errors.Is(verifyErr, oauth.ErrSessionExpired), errors.Is(err, auth.ErrNoToken):
		return c.renderNotLoggedIn(ctx, mode, host, useColor, true)
	case err == nil:
		cred = current
	}

	email := cred.Email
	if verified && user.Email != "" {
		email = user.Email
	}
	name := user.Name
	session := describeSession(cred)

	if mode.JSON {
		payload := map[string]any{
			"logged_in":   true,
			"verified":    verified,
			"email":       email,
			"name":        name,
			"host":        host,
			"auth_method": session.Method,
		}
		if session.ExpiresAt != nil {
			payload["expires_at"] = session.ExpiresAt
		}
		if len(session.Scopes) > 0 {
			payload["scopes"] = session.Scopes
		}
		return output.JSON(ctx, payload)
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
			if _, err := output.Fprintf(ctx, "%s Logged in to %s as %s (%s)\n", symbol, bold(host, useColor), bold(name, useColor), email); err != nil {
				return err
			}
		} else {
			if _, err := output.Fprintf(ctx, "%s Logged in to %s as %s\n", symbol, bold(host, useColor), bold(email, useColor)); err != nil {
				return err
			}
		}
	} else {
		symbol := colorSymbol("!", "#f59e0b", useColor)
		if email != "" {
			if _, err := output.Fprintf(ctx, "%s Logged in to %s as %s (unverified)\n", symbol, bold(host, useColor), bold(email, useColor)); err != nil {
				return err
			}
		} else {
			if _, err := output.Fprintf(ctx, "%s Logged in to %s (unverified)\n", symbol, bold(host, useColor)); err != nil {
				return err
			}
		}
	}

	return printSessionDetails(ctx, session)
}

// renderNotLoggedIn reports the logged-out state; expired means a stored
// session was just found revoked or expired on the server.
func (c *AuthStatusCmd) renderNotLoggedIn(ctx context.Context, mode output.Mode, host string, useColor, expired bool) error {
	if mode.JSON {
		payload := map[string]any{
			"logged_in": false,
			"verified":  false,
			"email":     "",
			"name":      "",
			"host":      host,
		}
		if expired {
			payload["reason"] = "session_expired"
		}
		return output.JSON(ctx, payload)
	}
	if mode.Plain {
		return output.Plain(ctx, "logged_out", "", host)
	}

	symbol := colorSymbol("✗", "#ef4444", useColor)
	if expired {
		_, err := output.Fprintf(ctx, "%s Not logged in to %s: the login session expired or was revoked. Run `nimbu auth login`.\n", symbol, bold(host, useColor))
		return err
	}
	_, err := output.Fprintf(ctx, "%s Not logged in to %s\n", symbol, bold(host, useColor))
	return err
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

// sessionInfo describes how the CLI is authenticated.
type sessionInfo struct {
	// Method is "oauth" for a browser/device login session, "token" for a
	// stored API token and "env" for NIMBU_TOKEN.
	Method    string
	ExpiresAt *time.Time
	Scopes    []string
}

func describeSession(cred auth.Credential) sessionInfo {
	switch {
	case envToken() != "":
		return sessionInfo{Method: "env"}
	case cred.IsOAuth():
		info := sessionInfo{Method: auth.AuthMethodOAuth, Scopes: cred.Scopes}
		if !cred.ExpiresAt.IsZero() {
			expires := cred.ExpiresAt.UTC()
			info.ExpiresAt = &expires
		}
		return info
	default:
		return sessionInfo{Method: "token"}
	}
}

func printSessionDetails(ctx context.Context, session sessionInfo) error {
	if session.Method != auth.AuthMethodOAuth {
		return nil
	}
	line := "  Session: OAuth login (refreshes automatically)"
	if session.ExpiresAt != nil {
		line += fmt.Sprintf("; access token valid until %s", session.ExpiresAt.Local().Format(time.RFC3339))
	}
	if _, err := output.Fprintln(ctx, line); err != nil {
		return err
	}
	if len(session.Scopes) > 0 {
		if _, err := output.Fprintf(ctx, "  Scopes: %s\n", strings.Join(session.Scopes, ", ")); err != nil {
			return err
		}
	}
	return nil
}
