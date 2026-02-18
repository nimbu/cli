package api

import (
	"context"
	"net/http"
)

// OperationClass classifies request side-effect semantics.
type OperationClass string

const (
	OperationRead        OperationClass = "read"
	OperationMutate      OperationClass = "mutate"
	OperationDestructive OperationClass = "destructive"
)

type operationMeta struct {
	Class      OperationClass
	Idempotent bool
}

type operationMetaKey struct{}

// WithOperationClass overrides request operation class.
func WithOperationClass(class OperationClass) RequestOption {
	return func(o *requestOptions) {
		o.OperationClass = class
	}
}

// WithIdempotent marks whether the request is safe to retry.
func WithIdempotent(idempotent bool) RequestOption {
	return func(o *requestOptions) {
		o.Idempotent = &idempotent
	}
}

func operationDefaults(method string) operationMeta {
	switch method {
	case http.MethodGet, http.MethodHead:
		return operationMeta{Class: OperationRead, Idempotent: true}
	case http.MethodDelete:
		return operationMeta{Class: OperationDestructive, Idempotent: false}
	case http.MethodPut:
		return operationMeta{Class: OperationMutate, Idempotent: true}
	case http.MethodPatch:
		return operationMeta{Class: OperationMutate, Idempotent: false}
	default:
		return operationMeta{Class: OperationMutate, Idempotent: false}
	}
}

func operationMetaFromOptions(method string, opts *requestOptions) operationMeta {
	meta := operationDefaults(method)
	if opts == nil {
		return meta
	}
	if opts.OperationClass != "" {
		meta.Class = opts.OperationClass
	}
	if opts.Idempotent != nil {
		meta.Idempotent = *opts.Idempotent
	}
	return meta
}

func withOperationMeta(ctx context.Context, meta operationMeta) context.Context {
	return context.WithValue(ctx, operationMetaKey{}, meta)
}

func operationMetaFromContext(ctx context.Context) operationMeta {
	if ctx == nil {
		return operationDefaults(http.MethodGet)
	}
	if meta, ok := ctx.Value(operationMetaKey{}).(operationMeta); ok {
		return meta
	}
	return operationDefaults(http.MethodGet)
}
