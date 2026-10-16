package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"

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
	// Status lines go to stdout, so stdout decides color.
	st := newStatusStyler(output.WriterFromContext(ctx))

	// Structured output modes
	mode := output.FromContext(ctx)

	if !hasToken {
		return c.renderNotLoggedIn(ctx, mode, host, st, false)
	}

	// Verify token against API and fetch user info
	var user api.User
	var verifyErr error
	if client, err := GetAPIClient(ctx); err == nil {
		verifyErr = client.Get(ctx, "/user", &user, api.WithTokenRejection())
	} else {
		verifyErr = err
	}
	verified := verifyErr == nil

	// Verifying may have refreshed an OAuth session, or found it revoked and
	// deleted it; show what is stored now.
	current, err := ResolveAuthCredential(ctx)
	switch {
	case errors.Is(verifyErr, oauth.ErrSessionExpired), errors.Is(err, auth.ErrNoToken):
		return c.renderNotLoggedIn(ctx, mode, host, st, true)
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
		symbol := st.symbol("✓", "#22c55e")
		if name != "" {
			if _, err := output.Fprintf(ctx, "%s Logged in to %s as %s (%s)\n", symbol, st.bold(host), st.bold(name), email); err != nil {
				return err
			}
		} else {
			if _, err := output.Fprintf(ctx, "%s Logged in to %s as %s\n", symbol, st.bold(host), st.bold(email)); err != nil {
				return err
			}
		}
	} else {
		symbol := st.symbol("!", "#f59e0b")
		if email != "" {
			if _, err := output.Fprintf(ctx, "%s Logged in to %s as %s (unverified)\n", symbol, st.bold(host), st.bold(email)); err != nil {
				return err
			}
		} else {
			if _, err := output.Fprintf(ctx, "%s Logged in to %s (unverified)\n", symbol, st.bold(host)); err != nil {
				return err
			}
		}
	}

	return printSessionDetails(ctx, session, flags.Verbose)
}

// renderNotLoggedIn reports the logged-out state; expired means a stored
// session was just found revoked or expired on the server.
func (c *AuthStatusCmd) renderNotLoggedIn(ctx context.Context, mode output.Mode, host string, st statusStyler, expired bool) error {
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

	symbol := st.symbol("✗", "#ef4444")
	if expired {
		_, err := output.Fprintf(ctx, "%s Not logged in to %s: the login session expired or was revoked. Run `nimbu auth login`.\n", symbol, st.bold(host))
		return err
	}
	_, err := output.Fprintf(ctx, "%s Not logged in to %s\n", symbol, st.bold(host))
	return err
}

// statusStyler styles auth status lines for stdout. Lipgloss v2 styles are
// pure and always render truecolor, so colors are converted to the profile
// decided for stdout before rendering.
type statusStyler struct {
	on      bool
	profile colorprofile.Profile
}

func newStatusStyler(w *output.Writer) statusStyler {
	return statusStyler{on: w.UseColor(), profile: colorprofileFromTermenv(w.OutColorProfile())}
}

func (s statusStyler) symbol(symbol, hex string) string {
	if !s.on {
		return symbol
	}
	style := lipgloss.NewStyle().Bold(true)
	if c := s.profile.Convert(lipgloss.Color(hex)); c != nil {
		style = style.Foreground(c)
	}
	return style.Render(symbol)
}

func (s statusStyler) bold(text string) string {
	if !s.on {
		return text
	}
	return lipgloss.NewStyle().Bold(true).Render(text)
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

func printSessionDetails(ctx context.Context, session sessionInfo, verbose bool) error {
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
	for _, line := range scopeLines(session.Scopes, verbose, stdoutWidth(output.WriterFromContext(ctx))) {
		if _, err := output.Fprintln(ctx, line); err != nil {
			return err
		}
	}
	return nil
}

const scopesPrefix = "  Scopes: "

// scopeLines renders granted scopes for humans. A list that fits on one line
// is shown in full; a longer one is summarized unless verbose is set, in which
// case it is wrapped to width under a hanging indent. --json always carries
// the full list.
func scopeLines(scopes []string, verbose bool, width int) []string {
	if len(scopes) == 0 {
		return nil
	}
	full := scopesPrefix + strings.Join(scopes, ", ")
	if ansi.StringWidth(full) <= width {
		return []string{full}
	}
	if !verbose {
		return []string{fmt.Sprintf("%s%d granted (--verbose lists them)", scopesPrefix, len(scopes))}
	}

	indent := strings.Repeat(" ", ansi.StringWidth(scopesPrefix))
	var lines []string
	line := scopesPrefix
	empty := true
	for i, scope := range scopes {
		token := scope
		if i < len(scopes)-1 {
			token += ","
		}
		sep := " "
		if empty {
			sep = ""
		}
		if !empty && ansi.StringWidth(line+sep+token) > width {
			lines = append(lines, line)
			line, sep = indent, ""
		}
		line += sep + token
		empty = false
	}
	return append(lines, line)
}

// stdoutWidth is the terminal width of stdout, or 80 when it is not a
// terminal or the size is unknown.
func stdoutWidth(w *output.Writer) int {
	if f, ok := w.Out.(*os.File); ok && w.IsTTY() {
		if width, _, err := term.GetSize(int(f.Fd())); err == nil && width > 0 {
			return width
		}
	}
	return 80
}
