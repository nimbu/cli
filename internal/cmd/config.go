package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// ConfigCmd manages CLI configuration.
type ConfigCmd struct {
	List   ConfigListCmd   `cmd:"" help:"List all config values"`
	Get    ConfigGetCmd    `cmd:"" help:"Get a config value"`
	Set    ConfigSetCmd    `cmd:"" help:"Set a config value"`
	Unset  ConfigUnsetCmd  `cmd:"" help:"Unset a config value"`
	Path   ConfigPathCmd   `cmd:"" help:"Print config file path"`
	Banner ConfigBannerCmd `cmd:"" help:"Pick a banner theme interactively"`
}

// ConfigListCmd lists all config values.
type ConfigListCmd struct{}

func (c *ConfigListCmd) Run(ctx context.Context) error {
	cfg, err := config.Read()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	mode := output.FromContext(ctx)

	data := map[string]string{
		"default_site":    cfg.DefaultSite,
		"api_url":         cfg.APIURL,
		"timeout":         cfg.Timeout,
		"keyring_backend": cfg.KeyringBackend,
		"banner_theme":    cfg.BannerTheme,
	}

	if mode.JSON {
		return output.JSON(ctx, data)
	}

	if mode.Plain {
		for k, v := range data {
			fmt.Printf("%s\t%s\n", k, v)
		}
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tVALUE")
	for _, k := range []string{"default_site", "api_url", "timeout", "keyring_backend", "banner_theme"} {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", k, data[k])
	}
	return tw.Flush()
}

// ConfigGetCmd gets a config value.
type ConfigGetCmd struct {
	Key string `arg:"" help:"Config key to get"`
}

func (c *ConfigGetCmd) Run(ctx context.Context) error {
	cfg, err := config.Read()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	val, err := cfg.Get(c.Key)
	if err != nil {
		return err
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, map[string]string{c.Key: val})
	}

	fmt.Println(val)
	return nil
}

// ConfigSetCmd sets a config value.
type ConfigSetCmd struct {
	Key   string `arg:"" help:"Config key to set"`
	Value string `arg:"" help:"Value to set"`
}

func (c *ConfigSetCmd) Run(ctx context.Context) error {
	cfg, err := config.Read()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	if err := cfg.Set(c.Key, c.Value); err != nil {
		return err
	}

	if err := config.Write(cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, map[string]string{"status": "ok", "key": c.Key, "value": c.Value})
	}

	if strings.ToLower(c.Key) == "banner_theme" {
		if _, ok := BannerThemeByName(c.Value); !ok {
			fmt.Fprintf(os.Stderr, "warning: unknown theme %q; known themes: %s\n", c.Value, strings.Join(BannerThemeNames(), ", "))
		}
	}

	fmt.Printf("Set %s = %s\n", c.Key, c.Value)
	return nil
}

// ConfigUnsetCmd unsets a config value.
type ConfigUnsetCmd struct {
	Key string `arg:"" help:"Config key to unset"`
}

func (c *ConfigUnsetCmd) Run(ctx context.Context) error {
	cfg, err := config.Read()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	if err := cfg.Unset(c.Key); err != nil {
		return err
	}

	if err := config.Write(cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, map[string]string{"status": "ok", "key": c.Key})
	}

	fmt.Printf("Unset %s\n", c.Key)
	return nil
}

// ConfigPathCmd prints the config file path.
type ConfigPathCmd struct{}

func (c *ConfigPathCmd) Run(ctx context.Context) error {
	path, err := config.ConfigPath()
	if err != nil {
		return fmt.Errorf("get config path: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, map[string]string{"path": path})
	}

	fmt.Println(path)
	return nil
}
