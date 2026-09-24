package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

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
	writer := tabwriter.NewWriter(output.WriterFromContext(ctx).Out, 0, 0, 2, ' ', 0)
	if !output.FromContext(ctx).Plain {
		_, _ = fmt.Fprintln(writer, "NAME\tHOST\tSITE\tPROTECTED")
	}
	for _, row := range rows {
		if output.FromContext(ctx).Plain {
			if _, err := output.Fprintf(ctx, "%s\t%s\t%s\t%t\n", row.Name, row.Host, row.Site, row.Protected); err != nil {
				return err
			}
		} else {
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%t\n", row.Name, row.Host, row.Site, row.Protected)
		}
	}
	return writer.Flush()
}

type EnvAddCmd struct {
	Name      string `arg:"" required:"" help:"Environment name"`
	Host      string `required:"" help:"API host"`
	Protected bool   `help:"Require confirmation for schema apply and release"`
}

func (c *EnvAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("env add is disabled by --readonly")
	}
	if flags.Site == "" {
		return fmt.Errorf("env add requires --site")
	}
	env := config.EnvironmentConfig{Host: c.Host, Site: flags.Site, Protected: c.Protected}
	if _, err := migrate.ParseSiteRef(env.Site, env.Host, "", ""); err != nil {
		return err
	}
	path, err := config.FindProjectFile()
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
	if flags.Readonly {
		return fmt.Errorf("env remove is disabled by --readonly")
	}
	path, err := config.FindProjectFile()
	if err != nil {
		return err
	}
	if err := config.UpdateEnvironment(path, c.Name, nil); err != nil {
		return err
	}
	return environmentResult(ctx, c.Name, "Removed")
}

func applyEnvironment(flags *RootFlags) error {
	if flags.Environment == "" {
		return nil
	}
	path, err := config.FindProjectFile()
	if err != nil {
		return err
	}
	project, err := config.ReadProjectConfigFrom(path)
	if err != nil {
		return err
	}
	env, ok := project.Environments[flags.Environment]
	if !ok {
		return fmt.Errorf("unknown environment %q; run 'nimbu env list'", flags.Environment)
	}
	if err := env.Validate(); err != nil {
		return fmt.Errorf("environment %q: %w", flags.Environment, err)
	}
	ref, err := migrate.ParseSiteRef(env.Site, env.Host, "", "")
	if err != nil {
		return err
	}
	flags.APIURL, flags.Site = ref.BaseURL, ref.Site
	flags.SelectedEnvironmentProtected = env.Protected
	if warnings, err := config.WarnUnknownEnvironmentKeys(path); err == nil {
		for _, warning := range warnings {
			_, _ = fmt.Fprintf(output.DefaultWriter().Err, "warning: %s\n", warning)
		}
	}
	return nil
}

// IsProtectedEnvironment reports protection for the selected project environment.
func IsProtectedEnvironment(flags *RootFlags) bool {
	return flags != nil && flags.SelectedEnvironmentProtected
}

func confirmProtectedEnvironment(ctx context.Context, flags *RootFlags, yes bool, action string) error {
	if !IsProtectedEnvironment(flags) || yes {
		return nil
	}
	if flags.NoInput {
		return fmt.Errorf("%s on protected environment %q requires --yes with --no-input", action, flags.Environment)
	}
	promptFlags := *flags
	promptFlags.Force = false
	confirmed, err := confirmPrompt(&promptFlags, fmt.Sprintf("%s on protected environment %s", action, flags.Environment))
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
