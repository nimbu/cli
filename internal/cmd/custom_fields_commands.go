package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CustomersFieldsCmd shows the customer field schema.
type CustomersFieldsCmd struct{}

// ProductsFieldsCmd shows the product field schema.
type ProductsFieldsCmd struct{}

func (c *CustomersFieldsCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCustomFieldsCommand(
		ctx,
		"customers",
		"",
		func(ctx context.Context, client *api.Client) ([]api.CustomField, error) {
			return api.GetCustomerCustomizations(ctx, client)
		},
	)
}

func (c *ProductsFieldsCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCustomFieldsCommand(
		ctx,
		"products",
		"",
		func(ctx context.Context, client *api.Client) ([]api.CustomField, error) {
			return api.GetProductCustomizations(ctx, client)
		},
	)
}

func runCustomFieldsCommand(ctx context.Context, owner string, name string, loader func(context.Context, *api.Client) ([]api.CustomField, error)) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	fields, err := loader(ctx, client)
	if err != nil {
		return fmt.Errorf("load %s field schema: %w", owner, err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, fields)
	}
	if mode.Plain {
		return writeSchemaFieldPlain(ctx, owner, fields)
	}

	return writeSchemaFieldHuman(ctx, customFieldSchemaView{
		Key:    owner,
		Name:   name,
		Fields: fields,
	})
}
