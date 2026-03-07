package api

import "context"

// GetCustomerCustomizations fetches the customer field schema.
func GetCustomerCustomizations(ctx context.Context, c *Client, opts ...RequestOption) ([]CustomField, error) {
	var fields []CustomField
	if err := c.Get(ctx, "/customers/customizations", &fields, opts...); err != nil {
		return nil, err
	}
	return fields, nil
}

// ReplaceCustomerCustomizations replaces the customer field schema.
func ReplaceCustomerCustomizations(ctx context.Context, c *Client, fields any, opts ...RequestOption) error {
	opts = append(opts, WithParam("replace", "1"))
	return c.Post(ctx, "/customers/customizations", fields, nil, opts...)
}

// CreateCustomerCustomizations creates the customer field schema.
func CreateCustomerCustomizations(ctx context.Context, c *Client, fields any, opts ...RequestOption) error {
	return c.Post(ctx, "/customers/customizations", fields, nil, opts...)
}

// GetProductCustomizations fetches the product field schema.
func GetProductCustomizations(ctx context.Context, c *Client, opts ...RequestOption) ([]CustomField, error) {
	var fields []CustomField
	if err := c.Get(ctx, "/products/customizations", &fields, opts...); err != nil {
		return nil, err
	}
	return fields, nil
}

// ReplaceProductCustomizations replaces the product field schema.
func ReplaceProductCustomizations(ctx context.Context, c *Client, fields any, opts ...RequestOption) error {
	opts = append(opts, WithParam("replace", "1"))
	return c.Post(ctx, "/products/customizations", fields, nil, opts...)
}

// CreateProductCustomizations creates the product field schema.
func CreateProductCustomizations(ctx context.Context, c *Client, fields any, opts ...RequestOption) error {
	return c.Post(ctx, "/products/customizations", fields, nil, opts...)
}
