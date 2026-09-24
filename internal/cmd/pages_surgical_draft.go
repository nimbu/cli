package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// draftLocaleWarning is printed whenever a draft write carries --locale. The
// draft endpoints have been observed to store the default-locale text under the
// requested locale (theme-zenjoy-2026 audit, Sep 2026); until the API fix lands,
// agents should write translations on the live page after `draft publish`.
const draftLocaleWarning = "warning: --draft with --locale may store the default-locale text under that locale; write translations on the live page after draft publish, then verify with pages get --locale"

func warnDraftLocale(ctx context.Context, draft bool, locale string) {
	if draft && locale != "" {
		_, _ = fmt.Fprintln(output.WriterFromContext(ctx).Err, draftLocaleWarning)
	}
}

func (s *surgicalSession) applyWriteMode(write surgicalWriteFlags) error {
	if !write.Draft {
		return nil
	}
	warnDraftLocale(s.ctx, true, write.Locale)
	s.draftMode = true
	draft, err := s.client.GetPageDraft(s.ctx, s.pageID, api.DraftOptions{ContentLocale: s.locale})
	if err != nil {
		if api.IsNotFound(err) {
			return nil
		}
		return draftAPIError(err, s.fullpath)
	}
	s.draft = draft
	if doc, ok := draftSnapshotToDocument(draft.ContentMap()); ok {
		s.doc = mergeDraftResolutionDoc(s.doc, doc)
		s.draftConverted = true
	}
	return nil
}

func mergeDraftResolutionDoc(live, draftDoc map[string]any) map[string]any {
	out := cloneMap(live)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range draftDoc {
		if key == "id" {
			continue
		}
		out[key] = value
	}
	return out
}

func (s *surgicalSession) postDraftBatch(batch []api.BatchOperation) (*api.BatchResult, error) {
	res, err := s.client.PostPageDraftBatch(s.ctx, s.pageID, batch, api.DraftBatchOptions{ContentLocale: s.locale})
	if err != nil {
		return nil, draftAPIError(err, s.fullpath)
	}
	s.absorbDraftResult(res)
	return &api.BatchResult{Results: res.Results}, nil
}

func (s *surgicalSession) absorbDraftResult(res *api.DraftBatchResult) {
	if res == nil || len(res.Draft) == 0 {
		s.draftConverted = false
		return
	}
	var draft api.PageDraft
	if err := json.Unmarshal(res.Draft, &draft); err != nil {
		s.draftConverted = false
		return
	}
	s.draft = &draft
	if doc, ok := draftSnapshotToDocument(draft.ContentMap()); ok {
		s.doc = mergeDraftResolutionDoc(s.doc, doc)
		s.draftConverted = true
		return
	}
	s.draftConverted = false
}
