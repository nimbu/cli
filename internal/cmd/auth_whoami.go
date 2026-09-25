package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// AuthWhoamiCmd shows the current authenticated user.
type AuthWhoamiCmd struct{}

// Run executes the whoami command.
func (c *AuthWhoamiCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	var user api.User
	if err := client.Get(ctx, "/user", &user); err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	session := sessionInfo{Method: "token"}
	if cred, err := ResolveAuthCredential(ctx); err == nil {
		session = describeSession(cred)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, whoamiPayload{
			User:       user,
			AuthMethod: session.Method,
			ExpiresAt:  session.ExpiresAt,
			Scopes:     session.Scopes,
		})
	}

	if mode.Plain {
		return output.Plain(ctx, user.Email, user.Name)
	}

	admin := ""
	if user.Admin {
		admin = "yes"
	}
	expires := ""
	if session.ExpiresAt != nil {
		expires = session.ExpiresAt.Local().Format(time.RFC3339)
	}
	return output.Detail(ctx, user,
		[]any{user.Email, user.Name},
		[]output.Field{
			output.FAlways("Email", user.Email),
			output.F("Name", user.Name),
			output.F("Admin", admin),
			output.F("Auth", session.Method),
			output.F("Token expires", expires),
			output.F("Scopes", strings.Join(session.Scopes, ", ")),
		},
	)
}

// whoamiPayload is the user plus how the CLI is authenticated.
type whoamiPayload struct {
	api.User
	AuthMethod string     `json:"auth_method"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Scopes     []string   `json:"scopes,omitempty"`
}
