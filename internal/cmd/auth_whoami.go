package cmd

import (
	"context"
	"fmt"

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

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, user)
	}

	if mode.Plain {
		return output.Plain(ctx, user.Email, user.Name)
	}

	admin := ""
	if user.Admin {
		admin = "yes"
	}
	return output.Detail(ctx, user,
		[]any{user.Email, user.Name},
		[]output.Field{
			output.FAlways("Email", user.Email),
			output.F("Name", user.Name),
			output.F("Admin", admin),
		},
	)
}
