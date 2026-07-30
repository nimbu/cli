package cmd

import (
	"context"
	"net/http"
)

type ProductAttachmentsCmd struct {
	List     ProductAttachmentsListCmd     `cmd:"" help:"List product attachments"`
	Get      ProductAttachmentsGetCmd      `cmd:"" help:"Get a product attachment"`
	Create   ProductAttachmentsCreateCmd   `cmd:"" help:"Create a product attachment"`
	Update   ProductAttachmentsUpdateCmd   `cmd:"" help:"Update a product attachment"`
	Delete   ProductAttachmentsDeleteCmd   `cmd:"" help:"Delete a product attachment (requires --force)"`
	Download ProductAttachmentsDownloadCmd `cmd:"" help:"Download a product attachment"`
}

type ProductAttachmentsListCmd struct {
	QueryFlags `embed:""`
	Product    string `required:"" help:"Product ID or slug"`
}

func (c *ProductAttachmentsListCmd) Run(ctx context.Context) error {
	return auditedArray(ctx, "/products"+escaped(c.Product)+"/attachments", &c.QueryFlags)
}

type ProductAttachmentsGetCmd struct {
	Product    string `required:"" help:"Product ID or slug"`
	Attachment string `required:"" help:"Attachment ID"`
}

func (c *ProductAttachmentsGetCmd) Run(ctx context.Context) error {
	return auditedGet(ctx, "/products"+escaped(c.Product)+"/attachments"+escaped(c.Attachment))
}

type ProductAttachmentsCreateCmd struct {
	Product     string   `required:"" help:"Product ID or slug"`
	File        string   `help:"Read JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline attachment field assignments"`
}

func (c *ProductAttachmentsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	return auditedWrite(ctx, flags, http.MethodPost, "/products"+escaped(c.Product)+"/attachments", c.File, c.Assignments)
}

type ProductAttachmentsUpdateCmd struct {
	Product     string   `required:"" help:"Product ID or slug"`
	Attachment  string   `required:"" help:"Attachment ID"`
	File        string   `help:"Read JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline attachment field assignments"`
}

func (c *ProductAttachmentsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	return auditedWrite(ctx, flags, http.MethodPut, "/products"+escaped(c.Product)+"/attachments"+escaped(c.Attachment), c.File, c.Assignments)
}

type ProductAttachmentsDeleteCmd struct {
	Product    string `required:"" help:"Product ID or slug"`
	Attachment string `required:"" help:"Attachment ID"`
}

func (c *ProductAttachmentsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return auditedDelete(ctx, flags, "/products"+escaped(c.Product)+"/attachments"+escaped(c.Attachment))
}

type ProductAttachmentsDownloadCmd struct {
	Product    string `required:"" help:"Product ID or slug"`
	Attachment string `required:"" help:"Attachment ID"`
	Output     string `help:"Write the file to this path, or - for stdout" required:""`
}

func (c *ProductAttachmentsDownloadCmd) Run(ctx context.Context, flags *RootFlags) error {
	return auditedDownloadPath(ctx, flags, "/products"+escaped(c.Product)+"/attachments"+escaped(c.Attachment)+"/download", c.Output)
}
