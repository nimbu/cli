package api

import (
	"fmt"
	"io"
)

// RequestOption configures a request.
type RequestOption func(*requestOptions)

type requestOptions struct {
	Site           string
	Query          map[string]string
	Headers        map[string]string
	OperationClass OperationClass
	Idempotent     *bool
}

// RequestBody supplies a custom request stream instead of JSON-marshaled data.
type RequestBody struct {
	Reader        io.Reader
	GetBody       func() (io.ReadCloser, error)
	ContentType   string
	ContentLength int64
}

// WithSite sets the site for this request.
func WithSite(site string) RequestOption {
	return func(o *requestOptions) {
		o.Site = site
	}
}

// WithQuery adds query parameters.
func WithQuery(params map[string]string) RequestOption {
	return func(o *requestOptions) {
		if o.Query == nil {
			o.Query = make(map[string]string)
		}
		for k, v := range params {
			o.Query[k] = v
		}
	}
}

// WithParam adds a single query parameter.
func WithParam(key, value string) RequestOption {
	return func(o *requestOptions) {
		if o.Query == nil {
			o.Query = make(map[string]string)
		}
		o.Query[key] = value
	}
}

// WithHeader adds a request header.
func WithHeader(key, value string) RequestOption {
	return func(o *requestOptions) {
		if o.Headers == nil {
			o.Headers = make(map[string]string)
		}
		o.Headers[key] = value
	}
}

// WithPage adds pagination parameters.
func WithPage(page, perPage int) RequestOption {
	return func(o *requestOptions) {
		if o.Query == nil {
			o.Query = make(map[string]string)
		}
		if page > 0 {
			o.Query["page"] = itoa(page)
		}
		if perPage > 0 {
			o.Query["per_page"] = itoa(perPage)
		}
	}
}

// WithLocale sets the locale for the request.
func WithLocale(locale string) RequestOption {
	return func(o *requestOptions) {
		if o.Query == nil {
			o.Query = make(map[string]string)
		}
		if locale != "" {
			o.Query["locale"] = locale
		}
	}
}

// WithFields sets the fields to include.
func WithFields(fields string) RequestOption {
	return func(o *requestOptions) {
		if o.Query == nil {
			o.Query = make(map[string]string)
		}
		if fields != "" {
			o.Query["fields"] = fields
		}
	}
}

// WithInclude sets related resources to include.
func WithInclude(include string) RequestOption {
	return func(o *requestOptions) {
		if o.Query == nil {
			o.Query = make(map[string]string)
		}
		if include != "" {
			o.Query["include"] = include
		}
	}
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
