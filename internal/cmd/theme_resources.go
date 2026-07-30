package cmd

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/nimbu/cli/internal/api"
)

func readThemeContent(file string, inline string) ([]byte, error) {
	return readThemeContentFromReader(file, inline, os.Stdin)
}

func verifyThemeCodeResponse(ctx context.Context, client *api.Client, theme, collection, name string, submitted []byte, result api.ThemeResource) error {
	remoteCode := result.Code
	if remoteCode == "" {
		path := "/themes/" + url.PathEscape(theme) + "/" + collection + "/" + url.PathEscape(name)
		var verified api.ThemeResource
		if err := client.Get(ctx, path, &verified); err != nil {
			return fmt.Errorf("verify theme resource: %w", err)
		}
		remoteCode = verified.Code
	}
	if remoteCode != string(submitted) {
		return fmt.Errorf("verify theme resource: remote code does not match submitted code")
	}
	return nil
}

func readThemeContentFromReader(file string, inline string, stdin io.Reader) ([]byte, error) {
	if file != "" && inline != "" {
		return nil, fmt.Errorf("use either --file or inline content, not both")
	}
	switch {
	case inline != "" && inline != "-":
		return []byte(inline), nil
	case file != "":
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}
		defer func() { _ = f.Close() }()

		content, err := io.ReadAll(f)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}
		return content, nil
	default:
		content, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return content, nil
	}
}
