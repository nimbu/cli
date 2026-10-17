package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

type EnvCmd struct {
	List   EnvListCmd   `cmd:"" help:"List project environments"`
	Add    EnvAddCmd    `cmd:"" help:"Add a project environment"`
	Remove EnvRemoveCmd `cmd:"" help:"Remove a project environment"`
}

type EnvListCmd struct{}

func (c *EnvListCmd) Run(ctx context.Context) error {
	project, err := config.ReadProjectConfig()
	if err != nil {
		return err
	}
	type row struct {
		Name string `json:"name"`
		config.EnvironmentConfig
	}
	rows := make([]row, 0, len(project.Environments))
	for name, env := range project.Environments {
		rows = append(rows, row{name, env})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, rows)
	}
	table := make([]envListRow, 0, len(rows))
	for _, row := range rows {
		table = append(table, envListRow{Name: row.Name, Host: row.Host, Site: row.Site, Protected: row.Protected})
	}
	fields := []string{"name", "host", "site", "protected"}
	if output.FromContext(ctx).Plain {
		return output.PlainFromSlice(ctx, table, fields)
	}
	return output.WriteTable(ctx, table, fields, []string{"NAME", "HOST", "SITE", "PROTECTED"})
}

// envListRow is one `env list` row for the plain and table renderers.
type envListRow struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Site      string `json:"site"`
	Protected bool   `json:"protected"`
}

type EnvAddCmd struct {
	Name      string `arg:"" required:"" help:"Environment name"`
	Host      string `required:"" help:"API host"`
	Protected bool   `help:"Require confirmation for schema apply and release"`
}

func (c *EnvAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "add an environment"); err != nil {
		return err
	}
	if flags.Site == "" {
		return newHintedError(errors.New("env add requires --site"), errorSiteRequired, ExitUsage,
			"pass --site <site>, e.g. `nimbu env add "+c.Name+" --host "+c.Host+" --site <site>`")
	}
	env := config.EnvironmentConfig{Host: c.Host, Site: flags.Site, Protected: c.Protected}
	if _, err := migrate.ParseSiteRef(env.Site, env.Host, "", ""); err != nil {
		return newHintedError(err, errorUsageInvalid, ExitUsage, "check --host and --site")
	}
	path, err := environmentProjectFile("env add")
	if err != nil {
		return err
	}
	if err := config.UpdateEnvironment(path, c.Name, &env); err != nil {
		return err
	}
	return environmentResult(ctx, c.Name, "Added")
}

type EnvRemoveCmd struct {
	Name string `arg:"" required:"" help:"Environment name"`
}

func (c *EnvRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "remove an environment"); err != nil {
		return err
	}
	path, err := environmentProjectFile("env remove")
	if err != nil {
		return err
	}
	if err := config.UpdateEnvironment(path, c.Name, nil); err != nil {
		return err
	}
	return environmentResult(ctx, c.Name, "Removed")
}

// applyEnvironment resolves --env into the API URL and site. Warnings about
// the environment entry go through warn, the root warning sink, so they get
// the same --json envelope and completion silence as config warnings.
func applyEnvironment(flags *RootFlags, warn func(string)) error {
	if flags.Environment == "" {
		return nil
	}
	path, env, ref, err := resolveEnvironment(flags.Environment)
	if err != nil {
		return err
	}
	flags.APIURL, flags.Site = ref.BaseURL, ref.Site
	flags.SelectedEnvironmentProtected = env.Protected
	if warnings, err := config.WarnUnknownEnvironmentKeys(path); err == nil && warn != nil {
		for _, warning := range warnings {
			warn(warning)
		}
	}
	return nil
}

// resolveEnvironment reads the named environment from nimbu.yml and resolves
// it into an API base URL and site. It returns the project file path so the
// caller can report unknown keys in it.
func resolveEnvironment(name string) (string, config.EnvironmentConfig, migrate.SiteRef, error) {
	path, err := environmentProjectFile("--env " + name)
	if err != nil {
		return "", config.EnvironmentConfig{}, migrate.SiteRef{}, err
	}
	project, err := config.ReadProjectConfigFrom(path)
	if err != nil {
		return "", config.EnvironmentConfig{}, migrate.SiteRef{}, err
	}
	env, ok := project.Environments[name]
	if !ok {
		return "", config.EnvironmentConfig{}, migrate.SiteRef{}, newHintedError(fmt.Errorf("unknown environment %q", name), errorUsageInvalid, ExitUsage,
			"run `nimbu env list` to see the configured environments")
	}
	if err := env.Validate(); err != nil {
		return "", config.EnvironmentConfig{}, migrate.SiteRef{}, newHintedError(fmt.Errorf("environment %q: %w", name, err), errorUsageInvalid, ExitUsage,
			"fix the environment in "+config.ProjectFileName+", or re-add it with `nimbu env add`")
	}
	ref, err := migrate.ParseSiteRef(env.Site, env.Host, "", "")
	if err != nil {
		return "", config.EnvironmentConfig{}, migrate.SiteRef{}, newHintedError(fmt.Errorf("environment %q: %w", name, err), errorUsageInvalid, ExitUsage,
			"fix the environment in "+config.ProjectFileName+", or re-add it with `nimbu env add`")
	}
	return path, env, ref, nil
}

// environmentProjectFile finds nimbu.yml for an environment command. A missing
// project is a usage error: environments live in nimbu.yml.
func environmentProjectFile(action string) (string, error) {
	path, err := config.FindProjectFile()
	if errors.Is(err, config.ErrNotFound) {
		hint := "run it from a project directory (`nimbu init` creates one)"
		if strings.HasPrefix(action, "--env") {
			hint += ", or pass --site instead of --env"
		}
		return "", newHintedError(fmt.Errorf("%s needs a project: %w", action, err), errorUsageInvalid, ExitUsage, hint)
	}
	return path, err
}

// IsProtectedEnvironment reports protection for the selected project environment.
func IsProtectedEnvironment(flags *RootFlags) bool {
	return flags != nil && flags.SelectedEnvironmentProtected
}

func confirmProtectedEnvironment(ctx context.Context, flags *RootFlags, yes bool, action string) error {
	if !IsProtectedEnvironment(flags) || yes {
		return nil
	}
	target := fmt.Sprintf("%s on protected environment %s", action, flags.Environment)
	if reason := promptBlocker(flags); reason != "" {
		return protectedConfirmationError(fmt.Sprintf("%s requires --yes (cannot prompt: %s)", target, reason))
	}
	// --force deliberately does not confirm a protected environment.
	promptFlags := *flags
	promptFlags.Force, promptFlags.Yes = false, false
	confirmed, err := confirmPromptFlag(&promptFlags, target, "--yes")
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("%s cancelled", action)
	}
	return nil
}

// resolveEnvironmentReference substitutes an environment in the site position.
func resolveEnvironmentReference(raw, hostOverride string) (string, string, error) {
	project, err := config.ReadProjectConfig()
	if errors.Is(err, config.ErrNotFound) {
		return raw, hostOverride, nil
	}
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(strings.Trim(strings.TrimSpace(raw), "/"), "/", 2)
	env, ok := project.Environments[parts[0]]
	if !ok {
		return raw, hostOverride, nil
	}
	if err := env.Validate(); err != nil {
		return "", "", fmt.Errorf("environment %q: %w", parts[0], err)
	}
	resolved := env.Site
	if len(parts) == 2 {
		resolved += "/" + parts[1]
	}
	return resolved, env.Host, nil
}

func environmentResult(ctx context.Context, name, action string) error {
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, map[string]string{"status": "ok", "name": name})
	}
	_, err := output.Fprintf(ctx, "%s environment %s\n", action, name)
	return err
}
