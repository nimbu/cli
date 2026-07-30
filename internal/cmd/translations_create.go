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

	body, batch, err := readTranslationCreateInput(c.File, assignments)
	if err != nil {
		return err
	}

	if batch {
		var translations []api.Translation
		if err := client.Post(ctx, "/translations", body, &translations); err != nil {
			return fmt.Errorf("create translations: %w", err)
		}
		if output.FromContext(ctx).JSON {
			return output.JSON(ctx, translations)
		}
		rows := expandTranslationsListRows(translations, "")
		return output.PrintSlice(ctx, rows, []string{"key", "locale", "value"}, []string{"Key", "Locale", "Value"})
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

func readTranslationCreateInput(file string, assignments []string) (any, bool, error) {
	if file != "" && len(assignments) > 0 {
		return nil, false, fmt.Errorf("use either --file or inline assignments, not both")
	}
	if len(assignments) > 0 {
		body, err := parseInlineAssignments(assignments)
		return body, false, err
	}

	value, err := readJSONAnyInput(file)
	if err != nil {
		return nil, false, err
	}
	switch body := value.(type) {
	case map[string]any:
		return body, false, nil
	case []any:
		for index, item := range body {
			if _, ok := item.(map[string]any); !ok {
				return nil, false, fmt.Errorf("parse JSON: translation array item %d must be an object", index)
			}
		}
		return body, true, nil
	default:
		return nil, false, fmt.Errorf("parse JSON: expected an object or array of objects")
	}
}
