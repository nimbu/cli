package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type PageVersionsCmd struct {
	List    PageVersionsListCmd    `cmd:"" help:"List page versions"`
	Get     PageVersionsGetCmd     `cmd:"" help:"Get a page version"`
	Restore PageVersionsRestoreCmd `cmd:"" help:"Restore a page version"`
}

type PageVersionsListCmd struct {
	Page string `required:"" help:"Page ID or path"`
}

func (c *PageVersionsListCmd) Run(ctx context.Context) error {
	client, err := auditedClient(ctx)
	if err != nil {
		return err
	}
	const batchSize = 100
	const maxVersions = 100_000
	path := "/pages" + escaped(c.Page) + "/versions"
	versions := make([]json.RawMessage, 0, batchSize)
	var previousBatch []byte
	for offset := 0; offset < maxVersions; offset += batchSize {
		var batch []json.RawMessage
		if err := client.Get(ctx, path, &batch,
			api.WithParam("limit", fmt.Sprint(batchSize)),
			api.WithParam("offset", fmt.Sprint(offset)),
		); err != nil {
			return fmt.Errorf("list page versions: %w", err)
		}
		encodedBatch, err := json.Marshal(batch)
		if err != nil {
			return fmt.Errorf("list page versions: compare pagination batch: %w", err)
		}
		if len(previousBatch) > 0 && bytes.Equal(previousBatch, encodedBatch) {
			return fmt.Errorf("list page versions: pagination did not advance at offset %d", offset)
		}
		previousBatch = encodedBatch
		versions = append(versions, batch...)
		if len(batch) < batchSize {
			return output.JSON(ctx, versions)
		}
	}
	return fmt.Errorf("list page versions: exceeded safety limit of %d versions", maxVersions)
}

type PageVersionsGetCmd struct {
	Page    string `required:"" help:"Page ID or path"`
	Version string `required:"" name:"page-version" help:"Page version ID"`
}

func (c *PageVersionsGetCmd) Run(ctx context.Context) error {
	return auditedGet(ctx, "/pages"+escaped(c.Page)+"/versions"+escaped(c.Version))
}

type PageVersionsRestoreCmd struct {
	Page    string `required:"" help:"Page ID or path"`
	Version string `required:"" name:"page-version" help:"Page version ID"`
}

func (c *PageVersionsRestoreCmd) Run(ctx context.Context, flags *RootFlags) error {
	return auditedWriteBody(ctx, flags, http.MethodPost, "/pages"+escaped(c.Page)+"/versions"+escaped(c.Version)+"/restore", nil)
}
