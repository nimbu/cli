package migrate

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

// AppCodeCopyOptions controls app-code copy behavior.
type AppCodeCopyOptions struct {
	DryRun bool
}

// AppCodeCopyItem describes one copied or skipped app-code item.
type AppCodeCopyItem struct {
	AppKey  string `json:"app_key"`
	AppName string `json:"app_name,omitempty"`
	Name    string `json:"name,omitempty"`
	Action  string `json:"action"`
}

// AppCodeCopyResult reports app-code copy results.
type AppCodeCopyResult struct {
	From     SiteRef           `json:"from"`
	To       SiteRef           `json:"to"`
	Items    []AppCodeCopyItem `json:"items,omitempty"`
	Skipped  []AppCodeCopyItem `json:"skipped,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
}

// CopyAppCode copies cloud-code files for matching app keys between sites.
func CopyAppCode(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, opts AppCodeCopyOptions) (AppCodeCopyResult, error) {
	result := AppCodeCopyResult{From: fromRef, To: toRef}

	sourceApps, err := api.List[api.App](ctx, fromClient, "/apps")
	if err != nil {
		if isAppCodeUnavailable(err) {
			warning := fmt.Sprintf("skip cloud code: list source apps: %v", err)
			result.Warnings = append(result.Warnings, warning)
			emitStageWarning(ctx, "Cloud Code", warning)
			return result, nil
		}
		return result, fmt.Errorf("list source apps: %w", err)
	}
	targetApps, err := api.List[api.App](ctx, toClient, "/apps")
	if err != nil {
		if isAppCodeUnavailable(err) {
			warning := fmt.Sprintf("skip cloud code: list target apps: %v", err)
			result.Warnings = append(result.Warnings, warning)
			emitStageWarning(ctx, "Cloud Code", warning)
			return result, nil
		}
		return result, fmt.Errorf("list target apps: %w", err)
	}

	targetByKey := make(map[string]api.App, len(targetApps))
	for _, app := range targetApps {
		if app.Key != "" {
			targetByKey[app.Key] = app
		}
	}

	sort.SliceStable(sourceApps, func(i, j int) bool {
		return sourceApps[i].Key < sourceApps[j].Key
	})

	for _, sourceApp := range sourceApps {
		if sourceApp.Key == "" {
			continue
		}
		targetApp, ok := targetByKey[sourceApp.Key]
		if !ok {
			item := AppCodeCopyItem{AppKey: sourceApp.Key, AppName: sourceApp.Name, Action: "skip"}
			warning := fmt.Sprintf("skip cloud code app %s: target app key not found", sourceApp.Key)
			result.Skipped = append(result.Skipped, item)
			result.Warnings = append(result.Warnings, warning)
			emitStageWarning(ctx, "Cloud Code", warning)
			continue
		}
		if err := copyOneAppCode(ctx, fromClient, toClient, sourceApp, targetApp, opts, &result); err != nil {
			return result, err
		}
	}

	return result, nil
}

func copyOneAppCode(ctx context.Context, fromClient, toClient *api.Client, sourceApp, targetApp api.App, opts AppCodeCopyOptions, result *AppCodeCopyResult) error {
	sourceFiles, err := api.List[api.AppCodeFile](ctx, fromClient, "/apps/"+url.PathEscape(sourceApp.Key)+"/code")
	if err != nil {
		return fmt.Errorf("list source app code %s: %w", sourceApp.Key, err)
	}
	targetFiles, err := api.List[api.AppCodeFile](ctx, toClient, "/apps/"+url.PathEscape(targetApp.Key)+"/code")
	if err != nil {
		return fmt.Errorf("list target app code %s: %w", targetApp.Key, err)
	}
	sourceFiles, err = normalizeAppCodeFiles(sourceFiles)
	if err != nil {
		return fmt.Errorf("source app code %s: %w", sourceApp.Key, err)
	}
	targetFiles, err = normalizeAppCodeFiles(targetFiles)
	if err != nil {
		return fmt.Errorf("target app code %s: %w", targetApp.Key, err)
	}

	targetByName := make(map[string]api.AppCodeFile, len(targetFiles))
	for _, file := range targetFiles {
		targetByName[file.Name] = file
	}
	sort.SliceStable(sourceFiles, func(i, j int) bool {
		return sourceFiles[i].Name < sourceFiles[j].Name
	})

	total := int64(len(sourceFiles))
	for i, file := range sourceFiles {
		emitStageItem(ctx, "Cloud Code", sourceApp.Key+"/"+file.Name, int64(i+1), total)
		action := "create"
		if _, ok := targetByName[file.Name]; ok {
			action = "update"
		}
		if opts.DryRun {
			result.Items = append(result.Items, AppCodeCopyItem{AppKey: sourceApp.Key, AppName: sourceApp.Name, Name: file.Name, Action: "dry-run:" + action})
			continue
		}

		if action == "create" {
			body := map[string]any{"name": file.Name, "code": file.Code}
			if err := toClient.Post(ctx, "/apps/"+url.PathEscape(targetApp.Key)+"/code", body, &api.AppCodeFile{}); err != nil {
				return fmt.Errorf("create app code %s/%s: %w", sourceApp.Key, file.Name, err)
			}
		} else {
			body := map[string]any{"code": file.Code}
			if err := toClient.Put(ctx, "/apps/"+url.PathEscape(targetApp.Key)+"/code/"+url.PathEscape(file.Name), body, &api.AppCodeFile{}); err != nil {
				return fmt.Errorf("update app code %s/%s: %w", sourceApp.Key, file.Name, err)
			}
		}
		result.Items = append(result.Items, AppCodeCopyItem{AppKey: sourceApp.Key, AppName: sourceApp.Name, Name: file.Name, Action: action})
	}
	return nil
}

func isAppCodeUnavailable(err error) bool {
	return api.IsNotFound(err) || api.IsForbidden(err)
}

func normalizeAppCodeFiles(files []api.AppCodeFile) ([]api.AppCodeFile, error) {
	normalized := make([]api.AppCodeFile, 0, len(files))
	for _, file := range files {
		name, err := safeAppCodeName(file.Name)
		if err != nil {
			return nil, err
		}
		file.Name = name
		normalized = append(normalized, file)
	}
	return normalized, nil
}

func safeAppCodeName(raw string) (string, error) {
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
	name := path.Clean(slashed)
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("unsafe app code file name %q", raw)
	}
	return strings.TrimPrefix(name, "./"), nil
}
