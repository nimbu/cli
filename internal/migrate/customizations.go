package migrate

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
)

// CustomizationKind selects customer or product schemas.
type CustomizationKind string

const (
	CustomizationCustomers CustomizationKind = "customers"
	CustomizationProducts  CustomizationKind = "products"
)

// CustomizationCopyResult reports one schema copy operation.
type CustomizationCopyResult struct {
	Kind       CustomizationKind `json:"kind"`
	From       SiteRef           `json:"from"`
	To         SiteRef           `json:"to"`
	Action     string            `json:"action"`
	FieldCount int               `json:"field_count"`
}

// CustomizationDiffResult reports normalized schema differences.
type CustomizationDiffResult struct {
	Kind CustomizationKind `json:"kind"`
	From SiteRef           `json:"from"`
	To   SiteRef           `json:"to"`
	Diff DiffSet           `json:"diff"`
}

// CustomizationService loads and writes customer/product customizations.
type CustomizationService struct {
	Kind CustomizationKind
}

// Load fetches the schema.
func (s CustomizationService) Load(ctx context.Context, client *api.Client) ([]api.CustomField, error) {
	switch s.Kind {
	case CustomizationCustomers:
		return api.GetCustomerCustomizations(ctx, client)
	case CustomizationProducts:
		return api.GetProductCustomizations(ctx, client)
	default:
		return nil, fmt.Errorf("unsupported customization kind %q", s.Kind)
	}
}

// Write creates or replaces the schema.
func (s CustomizationService) Write(ctx context.Context, client *api.Client, fields []api.CustomField, replace bool) error {
	sanitized := sanitizeCustomFields(fields)
	switch s.Kind {
	case CustomizationCustomers:
		if replace {
			return api.ReplaceCustomerCustomizations(ctx, client, sanitized)
		}
		return api.CreateCustomerCustomizations(ctx, client, sanitized)
	case CustomizationProducts:
		if replace {
			return api.ReplaceProductCustomizations(ctx, client, sanitized)
		}
		return api.CreateProductCustomizations(ctx, client, sanitized)
	default:
		return fmt.Errorf("unsupported customization kind %q", s.Kind)
	}
}

// CopyCustomizations copies one schema between sites.
func CopyCustomizations(ctx context.Context, service CustomizationService, fromClient, toClient *api.Client, fromRef, toRef SiteRef) (CustomizationCopyResult, error) {
	fields, err := service.Load(ctx, fromClient)
	if err != nil {
		return CustomizationCopyResult{Kind: service.Kind, From: fromRef, To: toRef}, err
	}
	target, err := service.Load(ctx, toClient)
	if err != nil && !api.IsNotFound(err) {
		return CustomizationCopyResult{Kind: service.Kind, From: fromRef, To: toRef}, err
	}
	replace := len(target) > 0
	if err := service.Write(ctx, toClient, fields, replace); err != nil {
		return CustomizationCopyResult{Kind: service.Kind, From: fromRef, To: toRef}, err
	}
	action := "create"
	if replace {
		action = "replace"
	}
	return CustomizationCopyResult{
		Kind:       service.Kind,
		From:       fromRef,
		To:         toRef,
		Action:     action,
		FieldCount: len(fields),
	}, nil
}

func sanitizeCustomFields(fields []api.CustomField) []map[string]any {
	return NormalizeCustomizations(fields)
}

// DiffCustomizations diffs one schema between sites.
func DiffCustomizations(ctx context.Context, service CustomizationService, fromClient, toClient *api.Client, fromRef, toRef SiteRef) (CustomizationDiffResult, error) {
	fromFields, err := service.Load(ctx, fromClient)
	if err != nil {
		return CustomizationDiffResult{Kind: service.Kind, From: fromRef, To: toRef}, err
	}
	toFields, err := service.Load(ctx, toClient)
	if err != nil {
		return CustomizationDiffResult{Kind: service.Kind, From: fromRef, To: toRef}, err
	}
	return CustomizationDiffResult{
		Kind: service.Kind,
		From: fromRef,
		To:   toRef,
		Diff: DiffNormalized(NormalizeCustomizations(fromFields), NormalizeCustomizations(toFields)),
	}, nil
}
