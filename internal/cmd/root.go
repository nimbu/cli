package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/alecthomas/kong"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// Build info (injected via ldflags)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Context keys for dependency injection.
type (
	configKey    struct{}
	rootFlagsKey struct{}
)

// RootFlags contains global flags available to all commands.
type RootFlags struct {
	Site           string        `help:"Site ID or subdomain" env:"NIMBU_SITE"`
	APIURL         string        `help:"API base URL" default:"https://api.nimbu.io" env:"NIMBU_API_URL"`
	Timeout        time.Duration `help:"Request timeout" default:"30s" env:"NIMBU_TIMEOUT"`
	Fields         string        `help:"Comma-separated fields to return"`
	Locale         string        `help:"Filter by locale"`
	Include        string        `help:"Include related resources"`
	Sort           string        `help:"Sort by field, e.g. field or field:desc"`
	Filters        []string      `help:"Filter by key=value, repeatable"`
	Color          string        `help:"Color output: auto|always|never" default:"${color}" env:"NIMBU_COLOR"`
	JSON           bool          `help:"Output JSON to stdout" default:"${json}" env:"NIMBU_JSON"`
	Plain          bool          `help:"Output stable TSV to stdout" default:"${plain}" env:"NIMBU_PLAIN"`
	Force          bool          `help:"Skip confirmations for destructive operations"`
	NoInput        bool          `help:"Never prompt; fail instead (for CI)" env:"NIMBU_NO_INPUT"`
	Readonly       bool          `help:"Disable all write operations" env:"NIMBU_READONLY"`
	EnableCommands string        `help:"Comma-separated allowlist of commands" env:"NIMBU_ENABLE_COMMANDS"`
	Verbose        bool          `help:"Enable verbose logging"`
	Debug          bool          `help:"Enable debug logging (HTTP traces)"`
}

// CLI is the root command structure.
type CLI struct {
	RootFlags `embed:""`

	Version       kong.VersionFlag `help:"Print version and exit"`
	Auth          AuthCmd          `cmd:"" help:"Authentication and credentials"`
	Sites         SitesCmd         `cmd:"" help:"Manage sites"`
	Channels      ChannelsCmd      `cmd:"" help:"Manage channels and entries"`
	Pages         PagesCmd         `cmd:"" help:"Manage pages"`
	Menus         MenusCmd         `cmd:"" help:"Manage navigation menus"`
	Products      ProductsCmd      `cmd:"" help:"Manage products"`
	Collections   CollectionsCmd   `cmd:"" help:"Manage collections"`
	Coupons       CouponsCmd       `cmd:"" help:"Manage coupons"`
	Orders        OrdersCmd        `cmd:"" help:"Manage orders"`
	Customers     CustomersCmd     `cmd:"" help:"Manage customers"`
	Accounts      AccountsCmd      `cmd:"" help:"Manage accounts"`
	Notifications NotificationsCmd `cmd:"" help:"Manage notifications"`
	Roles         RolesCmd         `cmd:"" help:"Manage roles"`
	Redirects     RedirectsCmd     `cmd:"" help:"Manage redirects"`
	Functions     FunctionsCmd     `cmd:"" help:"Execute cloud functions"`
	Jobs          JobsCmd          `cmd:"" help:"Execute cloud jobs"`
	Apps          AppsCmd          `cmd:"" help:"Manage OAuth apps"`
	Themes        ThemesCmd        `cmd:"" help:"Manage themes"`
	Uploads       UploadsCmd       `cmd:"" help:"Manage uploads"`
	Blogs         BlogsCmd         `cmd:"" help:"Manage blogs"`
	Webhooks      WebhooksCmd      `cmd:"" help:"Manage webhooks"`
	Tokens        TokensCmd        `cmd:"" help:"Manage API tokens"`
	Translations  TranslationsCmd  `cmd:"" help:"Manage translations"`
	Config        ConfigCmd        `cmd:"" help:"Manage configuration"`
	API           APICmd           `cmd:"" help:"Raw API access"`
	Completion    CompletionCmd    `cmd:"" help:"Generate shell completions"`
}

// Note: ConfigCmd and CompletionCmd are implemented in their own files (config.go, completion.go)

type exitPanic struct{ code int }

// Execute runs the CLI with the given arguments and returns an exit code.
func Execute(args []string) int {
	err := execute(args)
	if err == nil {
		return 0
	}

	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}

func execute(args []string) (err error) {
	// Show help when no args provided
	if len(args) == 0 {
		args = []string{"--help"}
	}

	parser, cli, err := newParser()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			if ep, ok := r.(exitPanic); ok {
				if ep.code == 0 {
					err = nil
					return
				}
				err = &ExitError{Code: ep.code, Err: errors.New("exited")}
				return
			}
			panic(r)
		}
	}()

	kctx, err := parser.Parse(args)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return wrapParseError(err)
	}

	// Set up logging
	logLevel := slog.LevelWarn
	if cli.Verbose {
		logLevel = slog.LevelInfo
	}
	if cli.Debug {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	// Create context with values
	ctx := context.Background()

	// Set up output mode
	outMode, err := output.FromFlags(cli.JSON, cli.Plain)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return &ExitError{Code: ExitUsage, Err: err}
	}
	ctx = output.WithMode(ctx, outMode)

	// Set up output writer
	writer := output.DefaultWriter()
	writer.Mode = outMode
	writer.Color = cli.Color
	writer.NoTTY = cli.NoInput
	ctx = output.WithWriter(ctx, writer)

	// Load config
	cfg, _ := config.Read() // Ignore error, use defaults

	// Resolve site from flags, config, or project file
	site := cli.Site
	if site == "" {
		site = cfg.DefaultSite
	}
	if site == "" {
		if proj, err := config.ReadProjectConfig(); err == nil {
			site = proj.Site
		}
	}

	// Store values in context
	ctx = context.WithValue(ctx, rootFlagsKey{}, &cli.RootFlags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)

	kctx.BindTo(ctx, (*context.Context)(nil))
	kctx.Bind(&cli.RootFlags)
	kctx.Bind(site) // Bind resolved site

	if err = kctx.Run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}

func newParser() (*kong.Kong, *CLI, error) {
	vars := kong.Vars{
		"color":   envOr("NIMBU_COLOR", "auto"),
		"json":    boolString(os.Getenv("NIMBU_JSON") == "1" || os.Getenv("NIMBU_JSON") == "true"),
		"plain":   boolString(os.Getenv("NIMBU_PLAIN") == "1" || os.Getenv("NIMBU_PLAIN") == "true"),
		"version": VersionString(),
	}

	cli := &CLI{}
	parser, err := kong.New(
		cli,
		kong.Name("nimbu-cli"),
		kong.Description("CLI for the Nimbu API - AI-agent first, human-friendly second"),
		kong.UsageOnError(),
		kong.Help(helpPrinter()),
		kong.ConfigureHelp(helpOptions()),
		kong.Vars(vars),
		kong.Writers(os.Stdout, os.Stderr),
		kong.Exit(func(code int) { panic(exitPanic{code: code}) }),
	)
	if err != nil {
		return nil, nil, err
	}
	return parser, cli, nil
}

func wrapParseError(err error) error {
	if err == nil {
		return nil
	}
	var parseErr *kong.ParseError
	if errors.As(err, &parseErr) {
		return &ExitError{Code: ExitUsage, Err: parseErr}
	}
	return err
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// VersionString returns the version string.
func VersionString() string {
	return fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)
}

// ExitError represents an error with a specific exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit code %d", e.Code)
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

// Exit codes
const (
	ExitSuccess    = 0
	ExitGeneral    = 1
	ExitUsage      = 2
	ExitAuth       = 3
	ExitAuthz      = 4
	ExitNotFound   = 5
	ExitValidation = 6
	ExitRateLimit  = 7
	ExitNetwork    = 8
)

// GetAPIClient creates an API client from context.
func GetAPIClient(ctx context.Context) (*api.Client, error) {
	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)

	// Get token from keyring
	store, err := auth.OpenDefault()
	if err != nil {
		return nil, fmt.Errorf("open keyring: %w", err)
	}

	token, err := store.GetToken()
	if err != nil {
		if errors.Is(err, auth.ErrNoToken) {
			return nil, fmt.Errorf("not logged in; run 'nimbu-cli auth login' first")
		}
		return nil, fmt.Errorf("get token: %w", err)
	}

	client := api.New(flags.APIURL, token)
	client = client.WithTimeout(flags.Timeout)
	client = client.WithDebug(flags.Debug)

	// Get resolved site
	if site, ok := ctx.Value(string("")).(string); ok && site != "" {
		client = client.WithSite(site)
	}

	return client, nil
}

// GetAPIClientWithSite creates an API client with a specific site.
func GetAPIClientWithSite(ctx context.Context, site string) (*api.Client, error) {
	client, err := GetAPIClient(ctx)
	if err != nil {
		return nil, err
	}
	if site != "" {
		client = client.WithSite(site)
	}
	return client, nil
}

// RequireSite ensures a site is specified.
func RequireSite(ctx context.Context, site string) (string, error) {
	if site != "" {
		return site, nil
	}

	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	if flags.Site != "" {
		return flags.Site, nil
	}

	cfg := ctx.Value(configKey{}).(*config.Config)
	if cfg.DefaultSite != "" {
		return cfg.DefaultSite, nil
	}

	if proj, err := config.ReadProjectConfig(); err == nil && proj.Site != "" {
		return proj.Site, nil
	}

	return "", fmt.Errorf("site required; use --site flag, NIMBU_SITE env, or .nimbu.json")
}

// MapAPIError maps API errors to exit codes.
func MapAPIError(err error) *ExitError {
	if err == nil {
		return nil
	}

	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.IsUnauthorized():
			return &ExitError{Code: ExitAuth, Err: err}
		case apiErr.IsForbidden():
			return &ExitError{Code: ExitAuthz, Err: err}
		case apiErr.IsNotFound():
			return &ExitError{Code: ExitNotFound, Err: err}
		case apiErr.IsValidation():
			return &ExitError{Code: ExitValidation, Err: err}
		case apiErr.IsRateLimit():
			return &ExitError{Code: ExitRateLimit, Err: err}
		}
	}

	return &ExitError{Code: ExitGeneral, Err: err}
}
