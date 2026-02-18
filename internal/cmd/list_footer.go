package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type listFooterMeta struct {
	All        bool
	Page       int
	PerPage    int
	Returned   int
	HasNext    bool
	Total      int
	TotalKnown bool
}

func newListFooterMeta(page, perPage int, paged api.Pagination, links api.Links, returned int) listFooterMeta {
	return listFooterMeta{
		All:        false,
		Page:       page,
		PerPage:    perPage,
		Returned:   returned,
		HasNext:    links.HasNext(),
		Total:      paged.Total,
		TotalKnown: paged.TotalKnown,
	}
}

func allListFooterMeta(returned int) listFooterMeta {
	return listFooterMeta{All: true, Returned: returned, Total: returned, TotalKnown: true}
}

func (m *listFooterMeta) probeTotal(ctx context.Context, client *api.Client, path string, opts []api.RequestOption) {
	if m == nil || m.All || m.TotalKnown || path == "" {
		return
	}
	if m.Page > 1 {
		return
	}
	count, err := api.Count(ctx, client, path, opts...)
	if err != nil {
		return
	}
	m.Total = count
	m.TotalKnown = true
}

func writeListFooter(ctx context.Context, resource string, meta listFooterMeta) error {
	mode := output.FromContext(ctx)
	if mode.JSON || mode.Plain {
		return nil
	}

	w := output.WriterFromContext(ctx)
	if meta.All {
		_, err := fmt.Fprintf(w.Out, "\nShowing all %d %s.\n", meta.Returned, resource)
		return err
	}

	if meta.TotalKnown {
		shown := meta.Returned
		if meta.Total < shown {
			shown = meta.Total
		}
		_, err := fmt.Fprintf(w.Out, "\nShowing %d of %d %s (page %d, per-page %d). Use --all for full results.\n", shown, meta.Total, resource, meta.Page, meta.PerPage)
		return err
	}

	if meta.HasNext {
		_, err := fmt.Fprintf(w.Out, "\nShowing %d %s on page %d (total unknown). More results available; use --page %d or --all.\n", meta.Returned, resource, meta.Page, meta.Page+1)
		return err
	}

	_, err := fmt.Fprintf(w.Out, "\nShowing %d %s on page %d (total unknown).\n", meta.Returned, resource, meta.Page)
	return err
}
