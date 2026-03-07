package apps

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// FileAction describes one cloud-code operation.
type FileAction struct {
	Action string `json:"action"`
	Local  string `json:"local,omitempty"`
	Name   string `json:"name"`
}

// PushPlan groups upload/delete work.
type PushPlan struct {
	Uploads []FileAction `json:"uploads,omitempty"`
	Deletes []FileAction `json:"deletes,omitempty"`
}

// Result is emitted by apps push.
type Result struct {
	AppKey    string       `json:"app_key"`
	LocalName string       `json:"local_name"`
	Files     []string     `json:"files,omitempty"`
	Uploads   []FileAction `json:"uploads,omitempty"`
	Created   []FileAction `json:"created,omitempty"`
	Updated   []FileAction `json:"updated,omitempty"`
	Deleted   []FileAction `json:"deleted,omitempty"`
	Sync      bool         `json:"sync,omitempty"`
}

// NormalizeHost converts an API URL into the stored host form.
func NormalizeHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	}
	return parsed.Host
}

// ExplicitFiles intersects discovered files with explicit project-relative selections.
func ExplicitFiles(discovered []string, explicit []string) ([]string, error) {
	if len(explicit) == 0 {
		return append([]string{}, discovered...), nil
	}
	allowed := make(map[string]struct{}, len(discovered))
	for _, file := range discovered {
		allowed[normalizePath(file)] = struct{}{}
	}
	selected := make([]string, 0, len(explicit))
	for _, raw := range explicit {
		item := normalizePath(raw)
		if item == "" || item == "." {
			continue
		}
		if _, ok := allowed[item]; !ok {
			return nil, fmt.Errorf("file %q is not part of configured app selection", raw)
		}
		selected = append(selected, item)
	}
	sort.Strings(selected)
	return selected, nil
}

// BuildPushPlan computes upload/delete actions.
func BuildPushPlan(app AppConfig, localFiles []string, remoteFiles []api.AppCodeFile, sync bool) PushPlan {
	remoteByName := make(map[string]api.AppCodeFile, len(remoteFiles))
	for _, file := range remoteFiles {
		remoteByName[file.Name] = file
	}

	plan := PushPlan{Uploads: make([]FileAction, 0, len(localFiles))}
	for _, file := range localFiles {
		name := RemoteName(app, file)
		action := "create"
		if _, ok := remoteByName[name]; ok {
			action = "update"
		}
		plan.Uploads = append(plan.Uploads, FileAction{
			Action: action,
			Local:  file,
			Name:   name,
		})
	}
	if sync {
		localNames := make(map[string]struct{}, len(localFiles))
		for _, file := range localFiles {
			localNames[RemoteName(app, file)] = struct{}{}
		}
		for _, remote := range remoteFiles {
			if _, ok := localNames[remote.Name]; ok {
				continue
			}
			plan.Deletes = append(plan.Deletes, FileAction{Action: "delete", Name: remote.Name})
		}
		sort.SliceStable(plan.Deletes, func(i, j int) bool {
			return plan.Deletes[i].Name < plan.Deletes[j].Name
		})
	}
	return plan
}

// PlanPush computes a structured push result from local and remote files.
func PlanPush(ctx context.Context, client *api.Client, app AppConfig, selected []string, sync bool) (Result, []string, error) {
	remoteFiles, err := api.List[api.AppCodeFile](ctx, client, "/apps/"+url.PathEscape(app.ID)+"/code")
	if err != nil {
		return Result{}, nil, err
	}
	plan := BuildPushPlan(app, selected, remoteFiles, sync)
	result := Result{
		AppKey:    app.ID,
		LocalName: app.Name,
		Files:     append([]string{}, selected...),
		Uploads:   append([]FileAction{}, plan.Uploads...),
		Sync:      sync,
	}
	for _, action := range plan.Uploads {
		if action.Action == "create" {
			result.Created = append(result.Created, action)
		} else {
			result.Updated = append(result.Updated, action)
		}
	}
	result.Deleted = append(result.Deleted, plan.Deletes...)

	deletes := make([]string, 0, len(plan.Deletes))
	for _, action := range plan.Deletes {
		deletes = append(deletes, action.Name)
	}
	return result, deletes, nil
}

// ExecutePush uploads/updates/deletes remote app files.
func ExecutePush(ctx context.Context, client *api.Client, app AppConfig, result Result) error {
	task := output.ProgressFromContext(ctx).Counter("push app code", int64(len(result.Uploads)+len(result.Deleted)))
	for _, action := range result.Uploads {
		task.SetLabel(fmt.Sprintf("%s %s", action.Action, action.Name))
		data, err := os.ReadFile(filepath.Join(app.ProjectRoot, filepath.FromSlash(action.Local)))
		if err != nil {
			task.Fail(err)
			return err
		}
		if action.Action == "create" {
			body := map[string]any{"name": action.Name, "code": string(data)}
			if err := client.Post(ctx, "/apps/"+url.PathEscape(app.ID)+"/code", body, &api.AppCodeFile{}); err != nil {
				task.Fail(err)
				return fmt.Errorf("create %s: %w", action.Name, err)
			}
			task.Add(1)
			continue
		}
		body := map[string]any{"code": string(data)}
		if err := client.Put(ctx, "/apps/"+url.PathEscape(app.ID)+"/code/"+url.PathEscape(action.Name), body, &api.AppCodeFile{}); err != nil {
			task.Fail(err)
			return fmt.Errorf("update %s: %w", action.Name, err)
		}
		task.Add(1)
	}
	for _, action := range result.Deleted {
		task.SetLabel(fmt.Sprintf("delete %s", action.Name))
		if err := client.Delete(ctx, "/apps/"+url.PathEscape(app.ID)+"/code/"+url.PathEscape(action.Name), nil); err != nil {
			task.Fail(err)
			return fmt.Errorf("delete %s: %w", action.Name, err)
		}
		task.Add(1)
	}
	task.Done("done")
	return nil
}
