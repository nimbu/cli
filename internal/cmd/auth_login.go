package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/output"
)

// AuthLoginCmd logs in to Nimbu.
type AuthLoginCmd struct {
	Email    string `help:"Email address" short:"e"`
	Password string `help:"Password (use stdin for security)" short:"p"`
	Token    string `help:"Use existing token instead of login" short:"t" env:"NIMBU_TOKEN"`
}

// Run executes the login command.
func (c *AuthLoginCmd) Run(ctx context.Context, flags *RootFlags) error {
	// If token provided directly, store it
	if c.Token != "" {
		return c.storeToken(ctx, c.Token, "")
	}

	// Get email
	email := c.Email
	if email == "" {
		if flags.NoInput {
			return fmt.Errorf("--email required with --no-input")
		}
		var err error
		email, err = prompt("Email: ")
		if err != nil {
			return fmt.Errorf("read email: %w", err)
		}
	}

	// Get password
	password := c.Password
	if password == "" {
		if flags.NoInput {
			return fmt.Errorf("--password required with --no-input")
		}
		var err error
		password, err = promptPassword("Password: ")
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
	}

	// Call login API
	client := api.New(flags.APIURL, "")

	var resp api.AuthResponse
	err := client.Post(ctx, "/auth/login", map[string]string{
		"email":    email,
		"password": password,
	}, &resp)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Store credentials
	if err := c.storeToken(ctx, resp.Token, email); err != nil {
		return err
	}

	// Output result
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, map[string]any{
			"status": "logged_in",
			"email":  email,
			"user":   resp.User,
		})
	}

	fmt.Printf("Logged in as %s\n", email)
	return nil
}

func (c *AuthLoginCmd) storeToken(ctx context.Context, token, email string) error {
	store, err := auth.OpenDefault()
	if err != nil {
		return fmt.Errorf("open keyring: %w", err)
	}

	if err := store.SetToken(token); err != nil {
		return fmt.Errorf("store token: %w", err)
	}

	if email != "" {
		if err := store.SetEmail(email); err != nil {
			return fmt.Errorf("store email: %w", err)
		}
	}

	return nil
}

func prompt(message string) (string, error) {
	fmt.Fprint(os.Stderr, message)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptPassword(message string) (string, error) {
	fmt.Fprint(os.Stderr, message)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr) // newline after password
	if err != nil {
		return "", err
	}
	return string(password), nil
}
