package apps

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PullOptions controls app-code pull scope.
type PullOptions struct {
	Only []string
}

// PullResult is emitted by apps code pull.
type PullResult struct {
	AppKey    string       `json:"app_key"`
	LocalName string       `json:"local_name"`
	Files     []string     `json:"files,omitempty"`
	Written   []FileAction `json:"written,omitempty"`
}

// Pull downloads remote app code files into the configured local app directory.
func Pull(ctx context.Context, client *api.Client, app AppConfig, opts PullOptions) (PullResult, error) {
	remoteFiles, err := api.List[api.AppCodeFile](ctx, client, "/apps/"+url.PathEscape(app.ID)+"/code")
	if err != nil {
		return PullResult{}, err
	}

	selected, err := selectRemoteFiles(app, remoteFiles, opts.Only)
	if err != nil {
		return PullResult{}, err
	}

	result := PullResult{
		AppKey:    app.ID,
		LocalName: app.Name,
		Files:     make([]string, 0, len(selected)),
		Written:   make([]FileAction, 0, len(selected)),
	}

	task := output.ProgressFromContext(ctx).Counter("pull app code", int64(len(selected)))
	for _, file := range selected {
		name, err := safeRemoteName(file.Name)
		if err != nil {
			task.Fail(err)
			return result, err
		}
		localPath, target, err := appCodeTargetPath(app, name)
		if err != nil {
			task.Fail(err)
			return result, err
		}
		task.SetLabel("write " + name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			task.Fail(err)
			return result, err
		}
		if err := os.WriteFile(target, []byte(file.Code), 0o644); err != nil {
			task.Fail(err)
			return result, err
		}
		result.Files = append(result.Files, name)
		result.Written = append(result.Written, FileAction{Action: "write", Local: localPath, Name: name})
		task.Add(1)
	}
	task.Done("done")
	return result, nil
}

func selectRemoteFiles(app AppConfig, files []api.AppCodeFile, only []string) ([]api.AppCodeFile, error) {
	byName := make(map[string]api.AppCodeFile, len(files))
	for _, file := range files {
		name, err := safeRemoteName(file.Name)
		if err != nil {
			return nil, err
		}
		file.Name = name
		byName[name] = file
	}

	if len(only) == 0 {
		selected := make([]api.AppCodeFile, 0, len(byName))
		for _, file := range byName {
			selected = append(selected, file)
		}
		sort.SliceStable(selected, func(i, j int) bool {
			return selected[i].Name < selected[j].Name
		})
		return selected, nil
	}

	seen := map[string]struct{}{}
	selected := make([]api.AppCodeFile, 0, len(only))
	for _, raw := range only {
		name, err := remoteNameFromSelector(app, raw)
		if err != nil {
			return nil, err
		}
		file, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("remote app code file %q not found", raw)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		selected = append(selected, file)
	}
	sort.SliceStable(selected, func(i, j int) bool {
		return selected[i].Name < selected[j].Name
	})
	return selected, nil
}

func remoteNameFromSelector(app AppConfig, raw string) (string, error) {
	value := normalizePath(raw)
	dirPrefix := normalizePath(app.Dir)
	if dirPrefix != "" && dirPrefix != "." {
		if value == dirPrefix {
			value = "."
		} else if strings.HasPrefix(value, dirPrefix+"/") {
			value = strings.TrimPrefix(value, dirPrefix+"/")
		}
	}
	return safeRemoteName(value)
}

func safeRemoteName(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	slashed := strings.ReplaceAll(trimmed, "\\", "/")
	if filepath.IsAbs(trimmed) || strings.HasPrefix(slashed, "/") {
		return "", fmt.Errorf("unsafe app code file name %q", raw)
	}
	for _, segment := range strings.Split(slashed, "/") {
		if segment == ".." {
			return "", fmt.Errorf("unsafe app code file name %q", raw)
		}
	}
	name := normalizePath(trimmed)
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("unsafe app code file name %q", raw)
	}
	return name, nil
}

func appCodeTargetPath(app AppConfig, name string) (string, string, error) {
	dirPrefix := normalizePath(app.Dir)
	if dirPrefix == "." {
		dirPrefix = ""
	}
	localPath := normalizePath(path.Join(dirPrefix, name))
	if localPath == "" || localPath == "." || filepath.IsAbs(localPath) || strings.HasPrefix(localPath, "/") || strings.HasPrefix(localPath, "../") {
		return "", "", fmt.Errorf("unsafe app code local path %q", localPath)
	}
	target := filepath.Join(app.ProjectRoot, filepath.FromSlash(localPath))
	rel, err := filepath.Rel(app.ProjectRoot, target)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("unsafe app code local path %q", localPath)
	}
	return localPath, target, nil
}
