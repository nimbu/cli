package themes

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// RunBuild executes the configured build command.
func RunBuild(ctx context.Context, cfg Config) error {
	if cfg.Build == nil || cfg.Build.Command == "" {
		return fmt.Errorf("--build requested but sync.build.command is not configured in nimbu.yml")
	}

	cmd := exec.CommandContext(ctx, cfg.Build.Command, cfg.Build.Args...)
	cmd.Dir = cfg.Build.CWD
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	for key, value := range cfg.Build.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run build command: %w", err)
	}
	return nil
}
