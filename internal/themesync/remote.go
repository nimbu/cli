package themesync

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

type themeBundle struct {
	Assets    []api.ThemeResource `json:"assets"`
	Layouts   []api.ThemeResource `json:"layouts"`
	Snippets  []api.ThemeResource `json:"snippets"`
	Templates []api.ThemeResource `json:"templates"`
}

// FetchRemoteResources returns the current remote theme resources exposed by the theme API.
func FetchRemoteResources(ctx context.Context, client *api.Client, theme string) ([]Resource, error) {
	var bundle themeBundle
	path := "/themes/" + url.PathEscape(theme)
	if err := client.Get(ctx, path, &bundle); err != nil {
		return nil, fmt.Errorf("get theme bundle: %w", err)
	}

	resources := make([]Resource, 0, len(bundle.Layouts)+len(bundle.Templates)+len(bundle.Snippets)+len(bundle.Assets))
	resources = append(resources, remoteResources(KindLayout, bundle.Layouts)...)
	resources = append(resources, remoteResources(KindTemplate, bundle.Templates)...)
	resources = append(resources, remoteResources(KindSnippet, bundle.Snippets)...)
	resources = append(resources, remoteResources(KindAsset, bundle.Assets)...)
	return resources, nil
}

// FetchResource returns one theme resource from its real collection endpoint.
func FetchResource(ctx context.Context, client *api.Client, theme string, kind Kind, remoteName string) (api.ThemeResource, error) {
	var resource api.ThemeResource
	path := fmt.Sprintf("/themes/%s/%s/%s", url.PathEscape(theme), kind.Collection(), url.PathEscape(remoteName))
	if err := client.Get(ctx, path, &resource); err != nil {
		return api.ThemeResource{}, err
	}
	return resource, nil
}

// Upsert uploads or updates one local resource.
func Upsert(ctx context.Context, client *api.Client, theme string, resource Resource, force bool) error {
	content, err := readFile(resource.AbsPath)
	if err != nil {
		return err
	}
	return UpsertBytes(ctx, client, theme, resource, content, force)
}

// UpsertBytes uploads or updates one resource from in-memory content.
func UpsertBytes(ctx context.Context, client *api.Client, theme string, resource Resource, content []byte, force bool) error {
	requestPath := fmt.Sprintf("/themes/%s/%s", url.PathEscape(theme), resource.Kind.Collection())
	var body map[string]any
	switch resource.Kind {
	case KindLayout, KindTemplate, KindSnippet:
		body = map[string]any{
			"name": resource.RemoteName,
			"code": string(content),
		}
	default:
		source := map[string]any{
			"__type":     "File",
			"attachment": base64.StdEncoding.EncodeToString(content),
			"filename":   path.Base(resource.RemoteName),
		}
		if contentType := mime.TypeByExtension(path.Ext(resource.RemoteName)); contentType != "" {
			source["content_type"] = contentType
		}
		body = map[string]any{
			"name":   resource.RemoteName,
			"source": source,
		}
	}

	opts := []api.RequestOption{}
	if force {
		opts = append(opts, api.WithQuery(map[string]string{"force": "true"}))
	}
	var ignored api.ThemeResource
	if err := client.Post(ctx, requestPath, body, &ignored, opts...); err != nil {
		return err
	}
	return nil
}

// Delete removes one remote resource.
func Delete(ctx context.Context, client *api.Client, theme string, resource Resource) error {
	requestPath := fmt.Sprintf("/themes/%s/%s/%s", url.PathEscape(theme), resource.Kind.Collection(), url.PathEscape(resource.RemoteName))
	return client.Delete(ctx, requestPath, nil)
}

// ReadContent fetches one theme resource body.
func ReadContent(ctx context.Context, client *api.Client, theme string, kind Kind, remoteName string) ([]byte, error) {
	resource, err := FetchResource(ctx, client, theme, kind, remoteName)
	if err != nil {
		return nil, err
	}
	if resource.Code != "" {
		return []byte(resource.Code), nil
	}
	if kind == KindAsset && strings.TrimSpace(resource.PublicURL) != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, resource.PublicURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("download asset: HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	return nil, fmt.Errorf("resource %s has no downloadable content", DisplayPath(kind, remoteName))
}

func remoteResources(kind Kind, values []api.ThemeResource) []Resource {
	resources := make([]Resource, 0, len(values))
	for _, value := range values {
		remoteName := remoteNameFor(kind, value)
		if remoteName == "" {
			continue
		}
		resources = append(resources, Resource{
			DisplayPath: DisplayPath(kind, remoteName),
			Kind:        kind,
			RemoteName:  remoteName,
		})
	}
	return resources
}

func remoteNameFor(kind Kind, value api.ThemeResource) string {
	if kind == KindAsset {
		if value.Path != "" {
			return strings.TrimPrefix(normalizePath(value.Path), "/")
		}
	}
	folder := normalizePath(strings.Trim(strings.TrimSpace(value.Folder), "/"))
	name := normalizePath(strings.TrimSpace(value.Name))
	if folder != "" && folder != "." && name != "" {
		return path.Join(folder, name)
	}
	if name != "" && name != "." {
		return name
	}
	if folder != "" && folder != "." {
		return folder
	}
	return normalizePath(strings.TrimSpace(value.ID))
}
