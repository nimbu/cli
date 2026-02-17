package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/alecthomas/kong"
)

// Build info (injected via ldflags)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const (
	colorAuto   = "auto"
	colorAlways = "always"
	colorNever  = "never"
)

// RootFlags contains global flags available to all commands.
type RootFlags struct {
	Site           string        `help:"Site ID or subdomain" env:"NIMBU_SITE"`
	APIURL         string        `help:"API base URL" default:"https://api.nimbu.io" env:"NIMBU_API_URL"`
	Timeout        time.Duration `help:"Request timeout" default:"30s" env:"NIMBU_TIMEOUT"`
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

	Version    kong.VersionFlag `help:"Print version and exit"`
	Auth       AuthCmd          `cmd:"" help:"Authentication and credentials"`
	Sites      SitesCmd         `cmd:"" help:"Manage sites"`
	Channels   ChannelsCmd      `cmd:"" help:"Manage channels and entries"`
	Pages      PagesCmd         `cmd:"" help:"Manage pages"`
	Menus      MenusCmd         `cmd:"" help:"Manage navigation menus"`
	Products   ProductsCmd      `cmd:"" help:"Manage products"`
	Orders     OrdersCmd        `cmd:"" help:"Manage orders"`
	Customers  CustomersCmd     `cmd:"" help:"Manage customers"`
	Themes     ThemesCmd        `cmd:"" help:"Manage themes"`
	Uploads    UploadsCmd       `cmd:"" help:"Manage uploads"`
	Blogs      BlogsCmd         `cmd:"" help:"Manage blogs"`
	Webhooks   WebhooksCmd      `cmd:"" help:"Manage webhooks"`
	Tokens     TokensCmd        `cmd:"" help:"Manage API tokens"`
	Config     ConfigCmd        `cmd:"" help:"Manage configuration"`
	API        APICmd           `cmd:"" help:"Raw API access"`
	Completion CompletionCmd    `cmd:"" help:"Generate shell completions"`
}

// Placeholder command structs (will be implemented in separate files)
type AuthCmd struct{}
type SitesCmd struct{}
type ChannelsCmd struct{}
type PagesCmd struct{}
type MenusCmd struct{}
type ProductsCmd struct{}
type OrdersCmd struct{}
type CustomersCmd struct{}
type ThemesCmd struct{}
type UploadsCmd struct{}
type BlogsCmd struct{}
type WebhooksCmd struct{}
type TokensCmd struct{}
type ConfigCmd struct{}
type APICmd struct{}
type CompletionCmd struct{}

// Placeholder Run methods
func (c *AuthCmd) Run(ctx context.Context) error       { return errors.New("not implemented") }
func (c *SitesCmd) Run(ctx context.Context) error      { return errors.New("not implemented") }
func (c *ChannelsCmd) Run(ctx context.Context) error   { return errors.New("not implemented") }
func (c *PagesCmd) Run(ctx context.Context) error      { return errors.New("not implemented") }
func (c *MenusCmd) Run(ctx context.Context) error      { return errors.New("not implemented") }
func (c *ProductsCmd) Run(ctx context.Context) error   { return errors.New("not implemented") }
func (c *OrdersCmd) Run(ctx context.Context) error     { return errors.New("not implemented") }
func (c *CustomersCmd) Run(ctx context.Context) error  { return errors.New("not implemented") }
func (c *ThemesCmd) Run(ctx context.Context) error     { return errors.New("not implemented") }
func (c *UploadsCmd) Run(ctx context.Context) error    { return errors.New("not implemented") }
func (c *BlogsCmd) Run(ctx context.Context) error      { return errors.New("not implemented") }
func (c *WebhooksCmd) Run(ctx context.Context) error   { return errors.New("not implemented") }
func (c *TokensCmd) Run(ctx context.Context) error     { return errors.New("not implemented") }
func (c *ConfigCmd) Run(ctx context.Context) error     { return errors.New("not implemented") }
func (c *APICmd) Run(ctx context.Context) error        { return errors.New("not implemented") }
func (c *CompletionCmd) Run(ctx context.Context) error { return errors.New("not implemented") }

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
	// TODO: Add output mode, site, API client to context

	kctx.BindTo(ctx, (*context.Context)(nil))
	kctx.Bind(&cli.RootFlags)

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
