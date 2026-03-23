package cmd

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/output"
)

// AuthLoginCmd logs in to Nimbu.
type AuthLoginCmd struct {
	Email     string `help:"Email address" short:"e"`
	Password  string `help:"Password (use stdin for security)" short:"p"`
	Token     string `help:"Use existing token instead of login" short:"t" env:"NIMBU_TOKEN"`
	ExpiresIn int    `help:"Token lifetime in seconds" short:"x" default:"31536000"`
}

// Run executes the login command.
func (c *AuthLoginCmd) Run(ctx context.Context, flags *RootFlags) error {
	host := apps.NormalizeHost(flags.APIURL)

	// If token provided directly, store it
	if c.Token != "" {
		return c.storeToken(ctx, c.Token, "", host)
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
	client := api.New(flags.APIURL, "").WithVersion(version)

	resp, err := loginWithCredentials(ctx, client, email, password, c.ExpiresIn, flags.NoInput, prompt)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Store credentials
	if err := c.storeToken(ctx, resp.Token, email, host); err != nil {
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

func loginWithCredentials(ctx context.Context, client *api.Client, email, password string, expiresIn int, noInput bool, promptTwoFactor func(string) (string, error)) (api.AuthResponse, error) {
	if expiresIn <= 0 {
		expiresIn = 60 * 60 * 24 * 365
	}

	resp, err := performLogin(ctx, client, email, password, expiresIn, "")
	if err == nil {
		return resp, nil
	}

	if !isTwoFactorRequired(err) {
		return api.AuthResponse{}, err
	}

	if noInput {
		return api.AuthResponse{}, fmt.Errorf("two-factor code required with --no-input")
	}

	secondFactor, err := promptTwoFactor("Two-factor code: ")
	if err != nil {
		return api.AuthResponse{}, fmt.Errorf("read two-factor code: %w", err)
	}

	return performLogin(ctx, client, email, password, expiresIn, secondFactor)
}

func performLogin(ctx context.Context, client *api.Client, email, password string, expiresIn int, secondFactor string) (api.AuthResponse, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "nimbu"
	}

	var resp api.AuthResponse
	opts := []api.RequestOption{api.WithHeader("Authorization", "Basic "+basicAuthHeader(email, password))}
	if secondFactor != "" {
		opts = append(opts, api.WithHeader("X-Nimbu-Two-Factor", secondFactor))
	}

	err = client.Post(ctx, "/auth/login", api.LoginRequest{
		Description: "Nimbu login from " + hostname,
		ExpiresIn:   expiresIn,
	}, &resp, opts...)
	return resp, err
}

func isTwoFactorRequired(err error) bool {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		return strings.TrimSpace(apiErr.Code) == "210"
	}
	return false
}

func basicAuthHeader(email, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(email + ":" + password))
}

func (c *AuthLoginCmd) storeToken(ctx context.Context, token, email, host string) error {
	store, err := openAuthStore(host)
	if err != nil {
		return fmt.Errorf("open keyring: %w", err)
	}

	if err := store.SetCredential(auth.Credential{Token: token, Email: email}); err != nil {
		return fmt.Errorf("store credential: %w", err)
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
