package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// AppsConfigCmd adds an app to the local project config.
type AppsConfigCmd struct{}

// Run executes the config command.
func (c *AppsConfigCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "configure app"); err != nil {
		return err
	}
	if flags != nil && flags.NoInput {
		return fmt.Errorf("apps config is interactive only; remove --no-input")
	}

	project, err := resolveProjectContext()
	if err != nil {
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

	availableApps, err := api.List[api.App](ctx, client, "/apps")
	if err != nil {
		return fmt.Errorf("list apps: %w", err)
	}
	activeHost := currentAPIHost(flags)
	configured := apps.VisibleApps(project.ProjectRoot, project.Config, activeHost, site)
	configuredIDs := map[string]struct{}{}
	for _, item := range configured {
		configuredIDs[item.ID] = struct{}{}
	}

	candidates := make([]api.App, 0, len(availableApps))
	for _, app := range availableApps {
		id := strings.TrimSpace(app.Key)
		if id == "" {
			id = strings.TrimSpace(app.Name)
		}
		if _, ok := configuredIDs[id]; ok {
			continue
		}
		candidates = append(candidates, app)
	}
	if len(candidates) == 0 {
		return fmt.Errorf("all apps for this host/site are already configured")
	}

	fmt.Fprintln(os.Stderr, "Available apps:")
	for i, app := range candidates {
		fmt.Fprintf(os.Stderr, "[%d] %s\n", i+1, app.Name)
	}
	reader := bufio.NewReader(os.Stdin)
	choiceRaw, err := promptWithReader(reader, "Which application do you want to configure? [1]: ")
	if err != nil {
		return err
	}
	if strings.TrimSpace(choiceRaw) == "" {
		choiceRaw = "1"
	}
	choice, err := strconv.Atoi(choiceRaw)
	if err != nil || choice < 1 || choice > len(candidates) {
		return fmt.Errorf("invalid application chosen")
	}
	app := candidates[choice-1]

	defaultName := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(app.Name), " ", "_"))
	name, err := promptWithReader(reader, fmt.Sprintf("Local app name [%s]: ", defaultName))
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		name = defaultName
	}
	dir, err := promptWithReader(reader, "Where is the code? [code]: ")
	if err != nil {
		return err
	}
	if strings.TrimSpace(dir) == "" {
		dir = "code"
	}
	glob, err := promptWithReader(reader, "What files should be pushed? [**/*.js]: ")
	if err != nil {
		return err
	}
	if strings.TrimSpace(glob) == "" {
		glob = "**/*.js"
	}

	item := config.AppProjectConfig{
		ID:   strings.TrimSpace(app.Key),
		Name: strings.TrimSpace(name),
		Dir:  strings.TrimSpace(filepath.ToSlash(dir)),
		Glob: strings.TrimSpace(glob),
		Host: activeHost,
		Site: site,
	}
	if item.ID == "" {
		item.ID = strings.TrimSpace(app.Name)
	}
	if err := config.UpsertProjectApp(project.File, item); err != nil {
		return fmt.Errorf("write project config: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, item)
	}
	if mode.Plain {
		return output.Plain(ctx, item.Name, item.ID, item.Dir, item.Glob, item.Host, item.Site)
	}
	if _, err := output.Fprintf(ctx, "Configured app %s in %s\n", item.Name, project.File); err != nil {
		return err
	}
	return nil
}

func promptWithReader(reader *bufio.Reader, message string) (string, error) {
	fmt.Fprint(os.Stderr, message)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
