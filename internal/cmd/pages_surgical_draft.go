package cmd

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

func (s *surgicalSession) applyWriteMode(write surgicalWriteFlags) error {
	if !write.Draft {
		return nil
	}
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

func (s *surgicalSession) postDraftBatch(ops []plannedOp, batch []api.BatchOperation) (*api.BatchResult, error) {
	res, err := s.client.PostPageDraftBatch(s.ctx, s.pageID, batch, api.DraftBatchOptions{ContentLocale: s.locale})
	if err != nil {
		return nil, hintDraftFileRef(draftAPIError(err, s.fullpath), ops)
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

func hintDraftFileRef(err error, ops []plannedOp) error {
	if err == nil || !opsHaveFileRef(ops) {
		return err
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	for _, result := range apiErr.BatchResults() {
		if result.Error == nil {
			continue
		}
		text := strings.ToLower(result.Error.Code + " " + result.Error.Message)
		if strings.Contains(text, "unauthorized") {
			return newHintedError(err, errorAuthForbidden, ExitAuthz,
				"FileRef uploads are not supported on drafts yet; use --from-file or attachment_path")
		}
	}
	return err
}

func opsHaveFileRef(ops []plannedOp) bool {
	for _, op := range ops {
		if valueIsFileRef(op.Op.Value) {
			return true
		}
	}
	return false
}

func valueIsFileRef(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if stringAny(object["__type"]) == "FileRef" {
		return true
	}
	return strings.HasPrefix(stringAny(object["source"]), "nimbu://")
}
