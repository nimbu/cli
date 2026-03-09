package migrate

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

func openDownloadResponse(ctx context.Context, client *api.Client, rawURL string) (*http.Response, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, "", err
	}
	attachAuth := false
	if !parsed.IsAbs() {
		base, err := url.Parse(client.BaseURL)
		if err != nil {
			return nil, "", err
		}
		parsed = base.ResolveReference(parsed)
		attachAuth = true
	} else if base, err := url.Parse(client.BaseURL); err == nil && strings.EqualFold(base.Host, parsed.Host) {
		attachAuth = true
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", err
	}
	if attachAuth && client.Token != "" {
		req.Header.Set("Authorization", "Bearer "+client.Token)
	}
	if attachAuth && client.Site != "" {
		req.Header.Set("X-Nimbu-Site", client.Site)
	}
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	return resp, parsed.String(), nil
}
