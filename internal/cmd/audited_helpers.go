package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

func auditedClient(ctx context.Context) (*api.Client, error) {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return nil, err
	}
	return GetAPIClientWithSite(ctx, site)
}

func auditedGet(ctx context.Context, path string, opts ...api.RequestOption) error {
	return auditedGetWith(ctx, auditedClient, path, opts...)
}

func auditedGlobalGet(ctx context.Context, path string, opts ...api.RequestOption) error {
	return auditedGetWith(ctx, GetAPIClient, path, opts...)
}

func auditedGetWith(ctx context.Context, clientFor func(context.Context) (*api.Client, error), path string, opts ...api.RequestOption) error {
	client, err := clientFor(ctx)
	if err != nil {
		return err
	}
	var result json.RawMessage
	if err := client.Get(ctx, path, &result, opts...); err != nil {
		return err
	}
	return output.JSON(ctx, result)
}

func auditedGlobalList(ctx context.Context, path string, query *QueryFlags, all bool, page, perPage int, extra ...api.RequestOption) error {
	return auditedListWith(ctx, GetAPIClient, path, query, all, page, perPage, extra...)
}

func auditedListWith(ctx context.Context, clientFor func(context.Context) (*api.Client, error), path string, query *QueryFlags, all bool, page, perPage int, extra ...api.RequestOption) error {
	client, err := clientFor(ctx)
	if err != nil {
		return err
	}
	opts, err := listRequestOptions(query, extra...)
	if err != nil {
		return err
	}
	var results []json.RawMessage
	if all {
		results, err = api.List[json.RawMessage](ctx, client, path, opts...)
	} else {
		if page == 0 {
			page = 1
		}
		if perPage == 0 {
			perPage = 25
		}
		var response *api.PagedResponse[json.RawMessage]
		response, err = api.ListPage[json.RawMessage](ctx, client, path, page, perPage, opts...)
		if response != nil {
			results = response.Data
		}
	}
	if err != nil {
		return err
	}
	return output.JSON(ctx, results)
}

func auditedArray(ctx context.Context, path string, query *QueryFlags, extra ...api.RequestOption) error {
	client, err := auditedClient(ctx)
	if err != nil {
		return err
	}
	opts, err := listRequestOptions(query, extra...)
	if err != nil {
		return err
	}
	var results []json.RawMessage
	if err := client.Get(ctx, path, &results, opts...); err != nil {
		return err
	}
	return output.JSON(ctx, results)
}

func auditedWrite(ctx context.Context, flags *RootFlags, method, path, file string, assignments []string) error {
	body, err := readJSONBodyInput(file, assignments)
	if err != nil {
		return err
	}
	return auditedWriteBody(ctx, flags, method, path, body)
}

func auditedGlobalWrite(ctx context.Context, flags *RootFlags, method, path, file string, assignments []string) error {
	body, err := readJSONBodyInput(file, assignments)
	if err != nil {
		return err
	}
	return auditedGlobalWriteBody(ctx, flags, method, path, body)
}

func auditedWriteBody(ctx context.Context, flags *RootFlags, method, path string, body any) error {
	return auditedWriteBodyWith(ctx, flags, auditedClient, method, path, body)
}

func auditedGlobalWriteBody(ctx context.Context, flags *RootFlags, method, path string, body any) error {
	return auditedWriteBodyWith(ctx, flags, GetAPIClient, method, path, body)
}

func auditedWriteBodyWith(ctx context.Context, flags *RootFlags, clientFor func(context.Context) (*api.Client, error), method, path string, body any) error {
	if err := requireWrite(flags, method+" "+path); err != nil {
		return err
	}
	client, err := clientFor(ctx)
	if err != nil {
		return err
	}
	var result json.RawMessage
	switch method {
	case http.MethodPost:
		err = client.Post(ctx, path, body, &result)
	case http.MethodPut:
		err = client.Put(ctx, path, body, &result)
	case http.MethodPatch:
		err = client.Patch(ctx, path, body, &result)
	default:
		return fmt.Errorf("unsupported audited write method: %s", method)
	}
	if err != nil {
		return err
	}
	if len(result) == 0 {
		return output.JSON(ctx, output.SuccessPayload("updated"))
	}
	return output.JSON(ctx, result)
}

func auditedDelete(ctx context.Context, flags *RootFlags, path string) error {
	return auditedDeleteWith(ctx, flags, auditedClient, path)
}

func auditedGlobalDelete(ctx context.Context, flags *RootFlags, path string) error {
	return auditedDeleteWith(ctx, flags, GetAPIClient, path)
}

func auditedDeleteWith(ctx context.Context, flags *RootFlags, clientFor func(context.Context) (*api.Client, error), path string) error {
	if err := requireWrite(flags, "delete "+path); err != nil {
		return err
	}
	if err := requireForce(flags, path); err != nil {
		return err
	}
	client, err := clientFor(ctx)
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, path, nil); err != nil {
		return err
	}
	return output.JSON(ctx, output.SuccessPayload("deleted"))
}

func auditedDownloadPath(ctx context.Context, flags *RootFlags, path, destination string) error {
	if destination == "" {
		return fmt.Errorf("--output is required")
	}
	client, err := auditedClient(ctx)
	if err != nil {
		return err
	}
	response, err := client.RawRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(response.Body)
		return decodeRawAPIError(response.StatusCode, body)
	}
	return writeDownloadResponse(ctx, response.Body, destination, flags != nil && flags.Force)
}

func escaped(parts ...string) string {
	result := ""
	for _, part := range parts {
		result += "/" + url.PathEscape(part)
	}
	return result
}
