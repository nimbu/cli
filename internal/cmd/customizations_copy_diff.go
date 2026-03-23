package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// CustomersConfigCopyCmd copies customer customizations.
type CustomersConfigCopyCmd struct {
	From     string `help:"Source site" required:"" name:"from"`
	To       string `help:"Target site" required:"" name:"to"`
	FromHost string `help:"Source API base URL or host" name:"from-host"`
	ToHost   string `help:"Target API base URL or host" name:"to-host"`
}

// CustomersConfigDiffCmd diffs customer customizations.
type CustomersConfigDiffCmd struct {
	From     string `help:"Source site" required:"" name:"from"`
	To       string `help:"Target site" required:"" name:"to"`
	FromHost string `help:"Source API base URL or host" name:"from-host"`
	ToHost   string `help:"Target API base URL or host" name:"to-host"`
}

// ProductsConfigCopyCmd copies product customizations.
type ProductsConfigCopyCmd struct {
	From     string `help:"Source site" required:"" name:"from"`
	To       string `help:"Target site" required:"" name:"to"`
	FromHost string `help:"Source API base URL or host" name:"from-host"`
	ToHost   string `help:"Target API base URL or host" name:"to-host"`
}

// ProductsConfigDiffCmd diffs product customizations.
type ProductsConfigDiffCmd struct {
	From     string `help:"Source site" required:"" name:"from"`
	To       string `help:"Target site" required:"" name:"to"`
	FromHost string `help:"Source API base URL or host" name:"from-host"`
	ToHost   string `help:"Target API base URL or host" name:"to-host"`
}

func (c *CustomersConfigCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCustomizationCopy(ctx, flags, migrate.CustomizationService{Kind: migrate.CustomizationCustomers}, c.From, c.To, c.FromHost, c.ToHost)
}

func (c *CustomersConfigDiffCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCustomizationDiff(ctx, migrate.CustomizationService{Kind: migrate.CustomizationCustomers}, c.From, c.To, c.FromHost, c.ToHost)
}

func (c *ProductsConfigCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCustomizationCopy(ctx, flags, migrate.CustomizationService{Kind: migrate.CustomizationProducts}, c.From, c.To, c.FromHost, c.ToHost)
}

func (c *ProductsConfigDiffCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCustomizationDiff(ctx, migrate.CustomizationService{Kind: migrate.CustomizationProducts}, c.From, c.To, c.FromHost, c.ToHost)
}

func runCustomizationCopy(ctx context.Context, flags *RootFlags, service migrate.CustomizationService, from, to, fromHost, toHost string) error {
	if err := requireWrite(flags, "copy customizations"); err != nil {
		return err
	}
	fromRef, err := parseSiteRefForCommand(ctx, from, fromHost)
	if err != nil {
		return err
	}
	toRef, err := parseSiteRefForCommand(ctx, to, toHost)
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
	targetFields, err := service.Load(ctx, toClient)
	if err != nil && !apiIsNotFound(err) {
		return err
	}
	if len(targetFields) > 0 && flags != nil && !flags.Force {
		ok, err := confirmPrompt(flags, fmt.Sprintf("replace existing %s customizations on %s", service.Kind, toRef.Site))
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("aborted")
		}
	}
	ctx, tl := copyWithTimeline(ctx, "Customizations", fromRef.Site, toRef.Site, false)
	if tl != nil {
		defer tl.Close()
	}
	result, err := migrate.CopyCustomizations(ctx, service, fromClient, toClient, fromRef, toRef, false)
	if err != nil {
		return finishCopyTimelineError(tl, err)
	}
	finishCopyTimeline(tl, "Customizations", fmt.Sprintf("%s %d fields", result.Action, result.FieldCount))
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		_, err := output.Fprintf(ctx, "%s\t%s\t%d\n", result.Action, result.Kind, result.FieldCount)
		return err
	}
	if tl != nil {
		return nil
	}
	_, err = output.Fprintf(ctx, "%s %s customizations (%d fields)\n", result.Action, result.Kind, result.FieldCount)
	return err
}

func runCustomizationDiff(ctx context.Context, service migrate.CustomizationService, from, to, fromHost, toHost string) error {
	fromRef, err := parseSiteRefForCommand(ctx, from, fromHost)
	if err != nil {
		return err
	}
	toRef, err := parseSiteRefForCommand(ctx, to, toHost)
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
	result, err := migrate.DiffCustomizations(ctx, service, fromClient, toClient, fromRef, toRef)
	if err != nil {
		return err
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	plainLines := append(renderDiffChanges("add", result.Diff.Added), renderDiffChanges("remove", result.Diff.Removed)...)
	plainLines = append(plainLines, renderDiffChanges("update", result.Diff.Updated)...)
	humanLines := append([]string{}, renderDiffChanges("+", result.Diff.Added)...)
	humanLines = append(humanLines, renderDiffChanges("-", result.Diff.Removed)...)
	humanLines = append(humanLines, renderDiffChanges("~", result.Diff.Updated)...)
	if len(plainLines) == 0 {
		plainLines = []string{"equal\t$"}
		humanLines = []string{"There are no differences."}
	}
	return writeDiffSet(ctx, result, plainLines, humanLines)
}

func apiIsNotFound(err error) bool {
	return api.IsNotFound(err)
}
