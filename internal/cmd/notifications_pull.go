package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/notifications"
	"github.com/nimbu/cli/internal/output"
)

// NotificationsPullCmd downloads notifications into content/notifications.
type NotificationsPullCmd struct {
	Only []string `help:"Only these notification slugs" name:"only"`
}

// Run executes notifications pull.
func (c *NotificationsPullCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "pull notification templates"); err != nil {
		return err
	}
	projectRoot, _, err := resolveProjectRoot()
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

	result, err := notifications.Pull(ctx, client, notifications.RootPath(projectRoot), setFromSlice(c.Only))
	if err != nil {
		return err
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		for _, item := range result.Written {
			fmt.Println(item)
		}
		return nil
	}
	for _, item := range result.Written {
		fmt.Printf("write %s\n", item)
	}
	fmt.Printf("pull complete: %d files\n", len(result.Written))
	return nil
}
