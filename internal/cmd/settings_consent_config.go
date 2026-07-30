package cmd

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// SettingsConsentConfigCmd manages the whole consent configuration.
type SettingsConsentConfigCmd struct {
	Get     SettingsConsentConfigGetCmd     `cmd:"" help:"Get the complete consent configuration"`
	Update  SettingsConsentConfigUpdateCmd  `cmd:"" help:"Update consent configuration fields"`
	Replace SettingsConsentConfigReplaceCmd `cmd:"" help:"Replace the complete consent configuration (requires --force)"`
	Copy    SettingsConsentConfigCopyCmd    `cmd:"" help:"Copy the complete consent configuration between sites"`
}

type SettingsConsentConfigGetCmd struct{}

func (c *SettingsConsentConfigGetCmd) Run(ctx context.Context) error {
	return auditedGet(ctx, "/settings/consent")
}

type SettingsConsentConfigUpdateCmd struct {
	File        string   `help:"Read JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline consent configuration assignments"`
}

func (c *SettingsConsentConfigUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update consent configuration"); err != nil {
		return err
	}
	return auditedWrite(ctx, flags, http.MethodPatch, "/settings/consent", c.File, c.Assignments)
}

type SettingsConsentConfigReplaceCmd struct {
	File        string   `help:"Read complete JSON configuration from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments are not supported for replacement"`
}

func (c *SettingsConsentConfigReplaceCmd) Run(ctx context.Context, flags *RootFlags) error {
	if c.File == "" {
		return fmt.Errorf("--file is required for whole consent configuration replacement (use --file - for stdin)")
	}
	if len(c.Assignments) > 0 {
		return fmt.Errorf("inline assignments are not supported for whole consent configuration replacement; use --file")
	}
	if err := requireWrite(flags, "replace consent configuration"); err != nil {
		return err
	}
	if flags == nil || !flags.Force {
		return fmt.Errorf("use --force to replace the whole consent configuration")
	}
	body, err := readJSONInputUseNumber(c.File)
	if err != nil {
		return err
	}
	return auditedWriteBody(ctx, flags, http.MethodPut, "/settings/consent", body)
}

// SettingsConsentConfigCopyCmd copies the complete consent configuration.
type SettingsConsentConfigCopyCmd struct {
	From     string `help:"Source site" required:"" name:"from"`
	To       string `help:"Target site" required:"" name:"to"`
	FromHost string `help:"Source API base URL or host" name:"from-host"`
	ToHost   string `help:"Target API base URL or host" name:"to-host"`
	DryRun   bool   `name:"dry-run" help:"Resolve and report changes without writing the target"`
}

func (c *SettingsConsentConfigCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !c.DryRun {
		if err := requireWrite(flags, "copy consent configuration"); err != nil {
			return err
		}
	}
	fromRef, err := parseSiteRefForCommand(ctx, c.From, c.FromHost)
	if err != nil {
		return err
	}
	toRef, err := parseSiteRefForCommand(ctx, c.To, c.ToHost)
	if err != nil {
		return err
	}
	fromClient, err := GetAPIClientWithBaseURL(ctx, fromRef.BaseURL, fromRef.Site)
	if err != nil {
		return err
	}
	toClient, err := GetAPIClientWithBaseURL(ctx, toRef.BaseURL, toRef.Site)
	if err != nil {
		return err
	}
	result, err := migrate.CopyConsentConfig(ctx, fromClient, toClient, fromRef, toRef, migrate.ConsentCopyOptions{DryRun: c.DryRun})
	if err != nil {
		return err
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		_, err = output.Fprintf(ctx, "%s\t%s\t%s\n", result.Action, result.From.Site, result.To.Site)
		return err
	}
	_, err = output.Fprintf(ctx, "%s consent configuration from %s to %s\n", result.Action, result.From.Site, result.To.Site)
	return err
}
