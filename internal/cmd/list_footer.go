package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type listFooterMeta struct {
	All      bool
	Page     int
	PerPage  int
	Returned int
	HasNext  bool
	// MaybeMore is set for a full page that came back without any Link
	// header: the same "probably more" signal api.PagedResponse.HasMore uses
	// for degraded responses. It only counts while the total is unknown.
	MaybeMore  bool
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
		MaybeMore:  links == (api.Links{}) && perPage > 0 && returned >= perPage,
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

// moreAvailable reports whether the listing left results out, or may have:
// the server advertised a next page, the known total exceeds what was
// returned, or (total unknown) a full page came back without a Link header.
func (m listFooterMeta) moreAvailable() bool {
	if m.All {
		return false
	}
	if m.HasNext {
		return true
	}
	if m.TotalKnown {
		return m.Total > m.Returned
	}
	return m.MaybeMore
}

// nextPage is the page to suggest, or 0 when the current page is the last.
func (m listFooterMeta) nextPage() int {
	if m.HasNext {
		return m.Page + 1
	}
	if m.TotalKnown {
		if m.PerPage > 0 && m.Page*m.PerPage < m.Total {
			return m.Page + 1
		}
		return 0
	}
	if m.MaybeMore {
		return m.Page + 1
	}
	return 0
}

// writeListFooter tells a human that a table is incomplete. It goes to stderr
// so `nimbu sites list | wc -l` counts rows only, and stays silent when the
// listing was complete. Machine modes get noteMoreResults instead.
func writeListFooter(ctx context.Context, resource string, meta listFooterMeta) error {
	if !output.IsHuman(ctx) || !meta.moreAvailable() {
		return nil
	}

	w := output.WriterFromContext(ctx)
	var msg string
	if meta.TotalKnown {
		msg = fmt.Sprintf("Showing %d of %d %s (page %d, per-page %d).",
			meta.Returned, meta.Total, pluralNoun(meta.Total, resource), meta.Page, meta.PerPage)
	} else {
		more := "more results available"
		if !meta.HasNext {
			more = "more results may be available"
		}
		msg = fmt.Sprintf("Showing %d %s on page %d; %s.",
			meta.Returned, pluralNoun(meta.Returned, resource), meta.Page, more)
	}
	if next := meta.nextPage(); next > 0 {
		msg += fmt.Sprintf(" Use --page %d for more, or --all for everything.", next)
	} else {
		msg += " Use --all for full results."
	}
	_, err := fmt.Fprintf(w.Err, "\n%s\n", w.ErrTheme().Muted(msg))
	return err
}

// noteMoreResults writes a one-line stderr notice in --json/--plain mode when
// the listing is incomplete, so agents learn to paginate without the stdout
// array or TSV shape changing. Human mode is handled by writeListFooter.
func noteMoreResults(ctx context.Context, resource string, meta listFooterMeta) {
	if output.IsHuman(ctx) || !meta.moreAvailable() {
		return
	}
	w := output.WriterFromContext(ctx)
	if w.Err == nil {
		return
	}

	shown := fmt.Sprintf("showing %d %s", meta.Returned, pluralNoun(meta.Returned, resource))
	if meta.TotalKnown {
		shown = fmt.Sprintf("showing %d of %d %s", meta.Returned, meta.Total, pluralNoun(meta.Total, resource))
	}
	hint := "pass --all"
	if next := meta.nextPage(); next > 0 {
		hint = fmt.Sprintf("pass --all or --page %d", next)
	}
	_, _ = fmt.Fprintf(w.Err, "nimbu: %s; %s\n", shown, hint)
}

// pluralNoun returns the plural resource name for n != 1 and its singular
// otherwise. Resource names are passed in their plural form.
func pluralNoun(n int, plural string) string {
	if n != 1 {
		return plural
	}
	switch {
	case strings.HasSuffix(plural, "ies"):
		return strings.TrimSuffix(plural, "ies") + "y"
	case strings.HasSuffix(plural, "s"):
		return strings.TrimSuffix(plural, "s")
	default:
		return plural
	}
}
