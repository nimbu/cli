package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// TranslationsCreateCmd creates a translation.
type TranslationsCreateCmd struct {
	File        string   `help:"Read translation JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. key=home.title, nl=Welkom, values.fr=Bienvenue)"`
}

// Run executes the create command.
func (c *TranslationsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create translation"); err != nil {
		return err
	}
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	assignments, err := translationAssignmentsWithLocaleShorthand(c.Assignments)
	if err != nil {
		return err
	}

	body, err := readJSONBodyInput(c.File, assignments)
	if err != nil {
		return err
	}

	var t api.Translation
	if err := client.Post(ctx, "/translations", body, &t); err != nil {
		return fmt.Errorf("create translation: %w", err)
	}

	return output.Print(ctx, t, []any{t.Key, t.Locale, t.Value}, func() error {
		_, err := output.Fprintf(ctx, "Created translation: %s\n", t.Key)
		return err
	})
}
