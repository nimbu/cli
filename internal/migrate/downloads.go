package migrate

import (
	"context"
	"net/http"

	"github.com/nimbu/cli/internal/api"
)

func openDownloadResponse(ctx context.Context, client *api.Client, rawURL string) (*http.Response, string, error) {
	return client.DownloadURL(ctx, rawURL)
}
