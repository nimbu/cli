package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// TranslationsUpdateCmd updates a translation.
type TranslationsUpdateCmd struct {
	Key         string   `arg:"" help:"Translation key"`
	File        string   `help:"Read translation JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. nl=Achternaam, values.fr=Nom)"`
}

// Run executes the update command.
func (c *TranslationsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update translation"); err != nil {
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

	var t map[string]any
	if err := client.Put(ctx, "/translations/"+url.PathEscape(c.Key), body, &t); err != nil {
		return fmt.Errorf("update translation: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, t)
	}

	if mode.Plain {
		key, _ := t["key"].(string)
		locale, _ := t["locale"].(string)
		value, _ := t["value"].(string)
		return output.Plain(ctx, key, locale, value)
	}

	fmt.Printf("Updated translation: %s\n", c.Key)
	return nil
}
