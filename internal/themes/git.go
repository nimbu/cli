package themes

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitChanges captures project-relative file changes from git.
type GitChanges struct {
	Changed     []string
	Deleted     []string
	FallbackAll bool
}

// CollectGitChanges returns git-based changed and deleted files for the project.
// When since is non-empty it is used as the diff ref instead of HEAD, allowing
// callers to detect committed-but-not-pushed changes (e.g. "origin/main").
func CollectGitChanges(cfg Config, since string) (GitChanges, error) {
	repoRoot, ok := gitRepoRoot(cfg.ProjectRoot)
	if !ok {
		return GitChanges{FallbackAll: true}, nil
	}
	if !gitHasHead(cfg.ProjectRoot) {
		if since != "" {
			return GitChanges{}, fmt.Errorf("--since %s: repository has no commits", since)
		}
		return GitChanges{FallbackAll: true}, nil
	}

	var changes GitChanges
	changedSeen := map[string]struct{}{}
	deletedSeen := map[string]struct{}{}

	ref := "HEAD"
	if since != "" {
		ref = since
	}

	lines, err := runGitLines(cfg.ProjectRoot, "diff", "--name-status", "--find-renames", ref, "--")
	if err != nil {
		return GitChanges{}, err
	}
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		status := ""
		if len(parts) > 0 {
			status = parts[0]
		}
		code := ""
		if status != "" {
			code = status[:1]
		}

		switch code {
		case "D":
			if rel, ok := repoPathToProjectRelative(repoRoot, cfg.ProjectRoot, parts[1:2]); ok {
				if _, exists := deletedSeen[rel]; !exists {
					deletedSeen[rel] = struct{}{}
					changes.Deleted = append(changes.Deleted, rel)
				}
			}
		case "R":
			if rel, ok := repoPathToProjectRelative(repoRoot, cfg.ProjectRoot, parts[1:2]); ok {
				if _, exists := deletedSeen[rel]; !exists {
					deletedSeen[rel] = struct{}{}
					changes.Deleted = append(changes.Deleted, rel)
				}
			}
			if rel, ok := repoPathToProjectRelative(repoRoot, cfg.ProjectRoot, parts[2:3]); ok {
				if _, exists := changedSeen[rel]; !exists {
					changedSeen[rel] = struct{}{}
					changes.Changed = append(changes.Changed, rel)
				}
			}
		default:
			if rel, ok := repoPathToProjectRelative(repoRoot, cfg.ProjectRoot, parts[1:2]); ok {
				if _, exists := changedSeen[rel]; !exists {
					changedSeen[rel] = struct{}{}
					changes.Changed = append(changes.Changed, rel)
				}
			}
		}
	}

	lines, err = runGitLines(cfg.ProjectRoot, "ls-files", "--others", "--exclude-standard", "--")
	if err != nil {
		return GitChanges{}, err
	}
	for _, line := range lines {
		if rel, ok := repoPathToProjectRelative(repoRoot, cfg.ProjectRoot, []string{line}); ok {
			if _, exists := changedSeen[rel]; !exists {
				changedSeen[rel] = struct{}{}
				changes.Changed = append(changes.Changed, rel)
			}
		}
	}

	return changes, nil
}

func gitRepoRoot(projectRoot string) (string, bool) {
	cmd := exec.Command("git", "-C", projectRoot, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", false
	}
	return filepath.Clean(root), true
}

func gitHasHead(projectRoot string) bool {
	cmd := exec.Command("git", "-C", projectRoot, "rev-parse", "--verify", "HEAD")
	return cmd.Run() == nil
}

func runGitLines(projectRoot string, args ...string) ([]string, error) {
	cmd := exec.Command("git", append([]string{"-C", projectRoot}, args...)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}

	lines := strings.Split(stdout.String(), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			filtered = append(filtered, line)
		}
	}
	return filtered, nil
}

func repoPathToProjectRelative(repoRoot, projectRoot string, values []string) (string, bool) {
	if len(values) == 0 {
		return "", false
	}
	repoPath := strings.TrimSpace(values[0])
	if repoPath == "" {
		return "", false
	}
	absPath := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	rel, err := projectRelativePath(projectRoot, absPath)
	if err != nil {
		return "", false
	}
	return normalizePath(rel), true
}
