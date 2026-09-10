# Plan: surgical page verbs, drafts, and the friction list

Source: `theme-zenjoy-2026/reports/agent-workflow-audit-2026-09-10.md`. Agents still run
`pages get` → Python → `pages update --replace` loops because the CLI does not expose the
batch API (`POST /pages/{id}/batch`), `schema`, `items`, or drafts. This plan wires them in.

## API facts (from Rails code, not the OpenAPI stubs)

- `POST /pages/{id}/batch` — `{operations:[...]}` (1..10 ops), `?atomic=1`, `?include=result`,
  `?content_locale=xx`. Requires `If-Match: "<etag>"` (strong). `etag = md5("<id>-<updated_at.to_f>")`.
  GET does **not** expose that ETag; the CLI computes it from `id` + `updated_at` (ms precision) and
  falls back to `current_etag` from the 412 body. Response `{results:[{index,status,path,id?,error?,warning?}], etag, updated_at, page?}`.
  Non-atomic batches return 200 even with per-op errors; atomic returns 422 `atomic_failure` with `results`.
  Route regex is `([^/]+)` so multi-segment fullpaths do not match: always call with the page ObjectId.
- Ops: `set {path,value}` (page fields `title|slug|seo_*|published`, items, or `{slug,position>=1}` on a repeatable);
  `insert {path:/items/<canvas>/repeatables, value:{slug,items:{name:value|{content}}}, after:<id>|null}`;
  `delete {path:<repeatable>}`; `move {path:<repeatable>, after:<id>|null}`. Anchors are ObjectIds only;
  `after` absent = append (insert) / no-op (move); `after: null` = first. Insert does not coerce values
  (no FileRef, no select validation) — the CLI issues a `set` for file editables after insert.
- Raw path grammar: `/title` … or `/items/<canvas>/repeatables/<id>/items/<editable>[/repeatables/<id>/items/<editable>]`, max two canvas levels.
- `GET /pages/{id}/schema` → `{template, available_blocks:{canvas:[{slug,label,fields:[{slug,type,options?}]}]}, current_structure, select_options:{"Canvas.slug.Field":[{label,value}]}}`.
- `GET /pages/{id}/items/<raw path without /items/>` → `{path,parent_path,position,siblings_count,type:item|repeatable,data}`.
- Drafts (`PAGE_DRAFTS_DISABLED` env flips them off → 403 "Page drafts are not enabled"):
  `GET|POST|PUT|PATCH|DELETE /pages/{id}/draft`, `POST /pages/{id}/draft/batch` (no If-Match, all-or-nothing),
  `POST /pages/{id}/draft/publish {confirm}` (409 `draft_base_changed` when live moved), `POST /pages/{id}/draft/preview_token` → `{token, preview_url}` (path-only URL; CLI prefixes the site domain).
- 422s on PUT: `invalid editable <name>` / `invalid slug (<slug>) for repeatable in canvas '<canvas>'`, message only.
- Channel entries: `?content_locale=en` sets the Globalize locale; localized fields write to EN. No `translations` map on GET.

## Human path grammar (CLI)

```
<path>   := "/" raw-api-path                       # passed verbatim
          | field                                  # title|slug|seo_title|seo_description|seo_keywords|published → /field
          | segment ("." segment)*
segment  := name [ "[" selector "]" ]
selector := <0-based index> | "id=" <objectid> | "slug=" <slug>   # slug= must be unique, else error lists matches
name     := unquoted (no . or [ ) | "quoted \"...\""
```

Examples: `Blokken[2].Title`, `Blokken[id=6a6d…].Photos[0].Image`, `Blokken[2]` (the repeatable itself),
`Blokken` (the canvas), `"Left Button - Link"` (only quote when the name has `.` or `[`).
Resolution uses the page document (ids/positions) plus the schema for candidates. Errors list the
candidates so an agent can self-correct in one step.

## Lanes

A. `pages set|insert|delete-block|move|batch|items|schema`, `--diff`, `--shape` with ids/positions/options.
B. `pages draft get|save|discard|batch|publish|preview-url`, `--draft` on lane-A verbs, dev-server preview.
C. `themes push --only` dependency auto-include + warning; 422 hint mapping; `--dry-run` if the API lands it.
D. Channel entry `--locale` write regression + lossless get.
E. Site resolution (git top-level, `NIMBU_PROJECT_DIR`), help text, CDN URL FileRef alias, theme get flags.
F. Skill + README.
