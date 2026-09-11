# Pages, Menus & Content Commands

Quick-reference for content management commands. Covers gotchas that `--help` does not surface.

## Pages (`nimbu pages`)

Subcommands: `list`, `get`, `create`, `update`, `delete`, `set`, `insert`, `delete-block`, `move`, `batch`, `draft`, `items`, `schema`, `count`, `copy`, `versions`.

Page versions are listed, fetched, and restored with `nimbu pages versions list|get|restore --page <id-or-path>`; `get` and `restore` also take `--page-version <id>`.

### Surgical verbs (default page workflow)

Prefer path verbs over a whole-document read-modify-write. Discover structure, then write one field or block. The CLI computes `If-Match` from the page `id` + `updated_at` and retries once on 412 by refetching; agents do not send or cache ETags. Drafts skip `If-Match`.

Recipe: `pages schema --page P` → `pages get --page P --shape` → `pages set|insert|move|delete-block` or `pages batch`. Fall back to `pages get --json` → edit → `pages update --file` only for whole-document rewrites.

`pages create` / `pages update --help` list `security_mechanism` (`none|humans|customers`) and `published` (`published:=true`).

#### Path grammar

Human paths resolve against the page document (ids/positions) plus the schema (candidates). Positions are **0-based**. A path that starts with `/` is a raw API path and is passed through unchanged.

```
<path>   := "/" raw-api-path
          | field
          | segment ("." segment)*
segment  := name [ "[" selector "]" ]
selector := <0-based index> | "id=" <objectid> | "slug=" <slug>
name     := unquoted (no . or [) | "quoted \"...\""
field    := title|slug|seo_title|seo_description|seo_keywords|published|og_image|template
```

| Example | Resolves to |
|---------|-------------|
| `title` | page field `/title` |
| `Blokken[2].Title` | third repeatable's `Title` |
| `Blokken[id=6a6d…].Photos[0].Image` | nested canvas, two levels max |
| `Blokken[2]` | the repeatable itself (for `move` / `delete-block`) |
| `Blokken` | the canvas (for `insert` / `items`) |
| `"Left Button - Link"` | quote a name that contains `.` or `[` |
| `/items/Blokken/repeatables/<id>/items/Title` | raw path, verbatim |

`slug=` must match exactly one sibling. Unresolved selectors list candidates (`[index] slug id`, or `Name (type)` for editables) so the next command can use an index or `id=`. An empty canvas tells you to `pages insert --path <canvas> --slug <slug>` and lists allowed slugs from the schema.

#### `pages schema --page P`

`GET /pages/{id}/schema`. Shows the template, available blocks per canvas (slug, label, fields), and select options.

```bash
nimbu pages schema --page about/team --json
```

#### `pages get --page P --shape`

Emits the canvas/repeatable skeleton (no content). Repeatable lines include `slug [position] id`. Select fields append `type: option | option`. Combined with `--download-assets`, `--shape` wins and the CLI warns on stderr.

```bash
nimbu pages get --page about/team --shape
nimbu pages get --page about/team --shape --json
```

#### `pages items --page P --path <path>`

`GET /pages/{id}/items/...` for one resolved subtree (human or raw path under `/items`). Page fields (`title`, …) are rejected — use `pages get`. Flags: `--page`, `--path` only.

```bash
nimbu pages items --page about/team --path Blokken[0]
```

#### `pages set --page P --path <path> [value]`

Set one page field or editable. Value sources are mutually exclusive: a positional `[value]`, `--file` (JSON value; `-` for stdin), or `--from-file` (local file uploaded as a file editable). `--from-file` requires a file editable.

Key flags: `--locale`, `--diff`, `--dry-run`, `--draft`.

```bash
nimbu pages set --page about/team --path title --dry-run --diff "Our Team"
nimbu pages set --page about/team --path Blokken[0].Title "Fast"
nimbu pages set --page about/team --path Blokken[0].Image --from-file ./hero.png
```

A file-editable positional value may be a JSON object, or an `http(s)://` / `nimbu://` URL (treated as `attachment_url`). See the FileRef table below.

#### `pages insert --page P --path <canvas>`

Insert a repeatable into a canvas. `--path` is the canvas (`Blokken`, `/items/Blokken`, or `/items/Blokken/repeatables`). `--slug` is required unless `--file` is a `{slug, items}` object. `--file` is a JSON items map or `{slug, items}`. There is no `--first` flag: omit `--after`/`--position` to append; `--position 0` inserts first; `--after` accepts a sibling id or 0-based index. `--after` and `--position` are mutually exclusive.

Key flags: `--slug`, `--after`, `--position`, `--file`, `--locale`, `--diff`, `--dry-run`, `--draft`.

```bash
nimbu pages insert --page about/team --path Blokken --slug item --position 0
nimbu pages insert --page about/team --path Blokken --slug item --file items.json
```

File editables in `--file` are stripped from the insert and applied as follow-up `set` ops (the API insert does not coerce FileRefs).

#### `pages delete-block --page P --path <repeatable>`

Delete one repeatable. `--path` must resolve to a repeatable (`Blokken[2]`, `Blokken[id=…]`). Requires global `--force`. Key flags: `--diff`, `--dry-run`, `--draft`.

```bash
nimbu pages delete-block --page about/team --path Blokken[2] --force
```

#### `pages move --page P --path <repeatable>`

Move a repeatable within its canvas. Requires `--after` (sibling id or 0-based index) or `--position` (0-based target index). No `--first` flag — use `--position 0`. Already-at-target is a no-op (`already at position N`). Key flags: `--after`, `--position`, `--diff`, `--dry-run`, `--draft`.

```bash
nimbu pages move --page about/team --path Blokken[2] --position 0
nimbu pages move --page about/team --path Blokken[2] --after 0
```

#### `pages batch --page P --file ops.json`

Apply up to 10 operations atomically (default; `--no-atomic` to disable). `--file` is `{"operations":[...]}` or a bare array. Each object:

| Field | Required | Meaning |
|-------|----------|---------|
| `op` | yes | `set`, `insert`, `delete`, or `move` |
| `path` | yes | human path (resolved) or raw `/…` (verbatim) |
| `value` | `set` / `insert` | set value, or insert `{slug, items}` |
| `after` | insert / move | JSON `null` = first; string = sibling **id** (not an index) |

Key flags: `--file`, `--[no-]atomic`, `--locale`, `--diff`, `--dry-run`, `--draft`. `pages draft batch` is the same as `pages batch --draft` (always atomic).

```bash
nimbu pages batch --page about/team --file ops.json --dry-run
```

```json
{
  "operations": [
    {"op": "set", "path": "Blokken[0].Title", "value": "x"},
    {"op": "insert", "path": "/items/Blokken/repeatables", "value": {"slug": "item", "items": {"Title": "New"}}, "after": null},
    {"op": "move", "path": "Blokken[2]", "after": "6a6d0123456789abcdef0001"},
    {"op": "delete", "path": "Blokken[1]"}
  ]
}
```

Human paths are resolved to raw API paths before POST. Raw paths that already start with `/` are sent unchanged. Insert wants the canvas `repeatables` collection (`/items/<canvas>/repeatables`).

`--file` accepts a path, `-`, a pipe, or process substitution (`--file <(echo '{"operations":[...]}')`).

### Drafts

`pages draft` edits and previews without publishing. `--draft` on `set` / `insert` / `delete-block` / `move` / `batch` writes the draft instead of the live page. A 403 "Page drafts are not enabled" means the server has drafts disabled — write the live page instead.

| Command | Flags | Notes |
|---------|-------|-------|
| `pages draft get --page P` | | Current draft document |
| `pages draft save --page P --file draft.json` | `--locale` | Whole-page draft from JSON (`-` for stdin) |
| `pages draft batch --page P --file ops.json` | `--locale`, `--diff`, `--dry-run` | Alias for `pages batch --draft` |
| `pages draft publish --page P` | `--confirm` | Promote draft to live |
| `pages draft discard --page P` | `--force` (required) | Drop the draft |
| `pages draft preview-url --page P` | `--open` | Absolute preview URL (`expires_in` 24h in `--json`) |

```bash
nimbu pages set --page about/team --path title --draft "Coming soon"
nimbu pages draft preview-url --page about/team
nimbu pages draft publish --page about/team
```

409 `draft_base_changed`: the live page changed after this draft was based on it. Re-run `pages draft publish --page P --confirm`, or `pages draft discard --page P --force`.

`nimbu server --draft <page>` (repeatable) requests a preview token at startup and injects `?preview=` on matching local URLs (the page fullpath, locale prefixes, and `translations.*.fullpath`). Child paths are not matched. Startup fails if drafts are disabled or the page has no draft. The simulator already honours `?preview=<token>` when the browser URL carries it.

### `--dry-run` and `--diff`

Surgical `--dry-run` resolves paths and prints the planned operations (JSON `{operations, paths}`) without writing. `--diff` prints a unified diff of the changed subtree (canvas repeatables for insert/move/delete-block) after a successful write. Draft `--diff` needs a converted snapshot; otherwise the CLI prints `(diff unavailable for drafts)`.

`pages update --dry-run` fetches, merges, and prints the PATCH body without sending it (stderr: `dry-run: no request sent`).

### 422 theme hint

`pages create` / `pages update` (and surgical writes) map 422 `invalid editable <name>` / `invalid slug (<slug>) for repeatable in canvas '<canvas>'` to a hint: the editable or slug is not in the pushed theme. Push it first:

```bash
nimbu themes push --only templates/<t>.liquid --dry-run
nimbu themes push --only templates/<t>.liquid
```

`--only` auto-includes Liquid snippet/layout dependencies. See [themes-and-local-dev.md](themes-and-local-dev.md).

### Parent field gotcha

**`parent` must be a fullpath string, NOT an object ID.** The API resolves parents by path. Setting `parent` to an ID is silently ignored and the page lands at root level. The `parent_path` field is read-only (ignored on write).

```json
{ "parent": "archive", "slug": "old-stuff", "title": "Old Stuff" }
```

The Go CLI learned this the hard way: the Node.js toolbelt does `data.parent = data.parent_path` and that works. Always use the fullpath string.

### get --download-assets DIR

Downloads file editables into DIR and rewrites the JSON output to `attachment_path` refs (local file paths instead of URLs). Useful for round-tripping page content: `get --download-assets ./assets` then edit JSON and `update --file`.

### update inline limits

Inline assignments (`nimbu pages update --page <fullpath> key=value`) only accept these shallow keys:
- `title`, `template`, `published`, `locale`
- one complete top-level `translations` object

Any deeper field (editables, nested content) requires `--file` or stdin. The CLI fetches the current document, merges inline assignments on top, and PATCHes. This is a read-modify-write cycle, not a blind overwrite.

### Locale-specific and multi-locale writes

`--locale` selects content through `content_locale`, so a write updates that
locale without overwriting the default locale:

```bash
printf '%s\n' '{"seo_title":"Nederlandse titel"}' |
  nimbu pages update --page about --locale nl --file=-
```

To update several locales together, send a top-level `translations` map.
`--file`/stdin is clearest for complex payloads:

```json
{
  "translations": {
    "nl": {"seo_title": "Nederlandse titel"},
    "fr": {"seo_title": "Titre français"}
  }
}
```

The equivalent inline form is
`translations:=@translations.json`. It recursively merges the supplied locale
and field keys into the fetched translations, preserving omitted locales and
sibling fields. Other deep page edits remain file-only.

With `--json`, reads and mutations retain the complete API document, including
all translations. Human/plain output overlays the selected locale recursively
and falls back to the default value for fields without a translation.

### update --file / stdin: merge by default, --replace to rebuild

**`pages update` MERGES by default.** A `--file` (or `-` for stdin) PATCH layers your editables on top of what the server already has; canvases you omit are left intact. This is the safe mode — use it for almost everything.

```bash
nimbu pages update --page about/team --file page.json   # merges
```

**`--replace` does a full destructive rebuild from `--file`/stdin.** It sends `replace=1`, so the server discards the current canvas contents and rebuilds them from exactly what you send. Anything you omit is gone. Inline assignments are merge-only; `--replace` with inline assignments is rejected.

```bash
nimbu pages update --page about/team --file page.json --replace
```

`--replace` is guarded against the classic wipe. Before sending, the CLI fetches the current page and compares repeatable counts per canvas. If any canvas would go from N repeatables down to 0, it **aborts**:

```
refusing to update: canvas 'features' would be wiped from 7->0; pass --allow-empty-canvas to override
```

To intentionally clear a canvas, add `--allow-empty-canvas`. The guard checks nested canvases too (`sections.gallery`, etc.) and only blocks the N->0 transition; shrinking 7->3 is allowed.

> **Never blind-resend a raw GET as a `--replace`.** A read-shape document is not a write-shape document (see "Page document shape" below) — re-sending it under `--replace` was the bug that silently wiped 12 pages. If you must replace, build the canvas payload deliberately. For round-trips, prefer plain merge (no `--replace`).

**Applied-count check.** After a `--file` update the CLI prints `Updated page <id> (<N> editables, <M> attachments)`. In `--replace` mode, if the server returns fewer editables than you submitted, it warns on stderr (`warning: server applied X editables but Y were submitted`) so a destructive rebuild that drops content is visible.

**Attachment expansion.** In any mode, file editables are auto-expanded before sending — see "File editable write shapes" below.

### Page document shape

A page document is a top-level object. The editable content lives under `items`, keyed by **editable name** (the `key` from the `{% editable_* %}` tag in the theme):

```json
{
  "fullpath": "about/team",
  "title": "Our Team",
  "items": {
    "hero_title": { "type": "string", "content": "Welcome" },
    "features": {
      "type": "canvas",
      "repeatables": [
        { "slug": "item", "position": 0, "items": { "title": { "type": "string", "content": "Fast" } } },
        { "slug": "item", "position": 1, "items": { "title": { "type": "string", "content": "Safe" } } }
      ]
    }
  }
}
```

- An editable with `"type": "canvas"` carries a `repeatables` array. Each entry has a `slug`, a `position` (sort order), and its own nested `items` map.
- **The repeatable `slug` is the template container slug, and it is NOT always `"item"`.** It comes from the `{% repeatable 'name' %}` tag in the theme. A checklist whose theme uses `{% repeatable 'row' %}` has `slug: "row"`, not `"item"`. Guessing `"item"` is the #1 footgun — confirm the real slug.
- Nesting goes **2 levels deep**: a canvas repeatable can contain another canvas with its own repeatables, but no deeper.
- An **empty page** shows `"items": {}`; an empty canvas shows `"repeatables": []`.

Stop reverse-engineering this from a live reference site — ask the CLI:

```bash
nimbu pages get --page about/team --shape          # readable tree
nimbu pages get --page about/team --shape --json   # machine-readable skeleton
```

`--shape` emits just the structure (editable name → `type`, and for canvases the `repeatables` with their `slug`, **0-based `position`**, **id**, and nested skeleton), no content. Select fields include their options. It is the fastest way to learn the exact repeatable slugs, ids, and positions before you write.
If combined with `--download-assets`, `--shape` wins and the CLI warns on stderr instead of downloading files.

### File editable write shapes

A file editable is `{ "type": "file", "file": { ... } }`. Pick one FileRef / file form for the `file` object (or for a surgical `pages set` / `insert` / `batch` value). Omit `file` (or the whole editable) to leave the existing asset unchanged.

| Form | Example | When to use | Notes |
|------|---------|-------------|-------|
| `attachment_path` | `{"attachment_path":"./logo.png"}` | Local file on disk | CLI reads the file, base64-encodes it, sets `__type: File` and `filename`. Surgical `--from-file` uses the equivalent `{data, filename, content_type}` shape. |
| `attachment_url` | `{"attachment_url":"https://cdn.nimbu.io/s/oa8td8r/assets/…/logo.png"}` | Remote URL, including a CDN link copied from `uploads list` or a page document | Same-site `https://cdn.nimbu.io/s/<siteShortId>/…` URLs are looked up in `GET /uploads` and rewritten to `nimbu://<siteShortId>/uploads/<id>` so the server references the existing upload. Any other HTTP(S) URL (plain remote, another site's CDN URL, or a lookup miss) is downloaded by the CLI (30s timeout, 25 MiB cap) and inlined as a copy: `pages update` sends `{"__type":"File","attachment":"<base64>","filename":"…"}`; surgical `pages set`/`insert`/`batch` send `{data, filename, content_type}` like `--from-file`. The CLI warns `warning: <url> is not an upload of this site; downloading it and storing a copy`. |
| `nimbu://` source | `{"__type":"FileRef","source":"nimbu://oa8td8r/uploads/507f1f77bcf86cd799439014"}` | Reuse an upload you already know by id | Does not duplicate the asset. Site short id is the `/s/<id>/` segment of the site's CDN root (also `site_short_id` on `GET /themes/<id>/info`). |
| `__type: FileRef` | `{"__type":"FileRef","source":"nimbu://oa8td8r/uploads/507f1f77bcf86cd799439014"}` | Send a FileRef object directly | Left untouched (user-explicit). `source` must be `nimbu://…` — the API rejects HTTP(S) FileRef sources with 422 (`Invalid FileRef URI: expected nimbu:// format`). To attach a remote HTTP(S) file, use `attachment_url` so the CLI downloads and inlines a copy. |
| `__type: File` + `attachment` | `{"__type":"File","attachment":"<base64>","filename":"logo.png"}` | Inline base64 bytes | `__type: File` is required; without it the server drops the upload. |
| `data` | `{"data":"<base64>","filename":"logo.png","content_type":"image/png"}` | Surgical set/insert of raw bytes | Produced by `pages set --from-file`. |
| read-only `url` | `items.<name>.file.url` | Do not write this | On *read*, the public CDN link. A bare `url` is not a write payload. Copy it into `attachment_url` to re-point the editable. |

A file editable that ends up with none of attachment / `attachment_path` / `attachment_url` / `source` is an **error** — the CLI refuses to write it rather than silently clearing the asset. In default merge mode, the CLI drops URL-only file objects from the write payload so the existing asset is left unchanged; under `--replace`, provide a real write shape. To intentionally clear a file editable, pass `--allow-empty-file`.

### create

Accepts `--file` or inline assignments. No inline key restrictions -- the body is POSTed as-is. `--help` lists `security_mechanism` (`none|humans|customers`) and `published` (`published:=true`) among the supported top-level fields.

### copy

`nimbu pages copy --only <fullpath|prefix*> --from <site> --to <site>`

Copies pages between sites. Supports glob prefix (`archive/*`). Default is `*` (all pages). Fails under `--readonly`. Cross-host copy supported via `--from-host` / `--to-host`.

### delete

Requires `--force`. Accepts page ID or slug.

## Menus (`nimbu menus`)

Subcommands: `list`, `get`, `create`, `update`, `delete`, `count`, `copy`.

### get returns nested tree

`nimbu menus get --menu <slug>` returns the full menu document including nested `items` tree with depth stats. If the singular GET returns a flat `items` projection, the CLI recovers the nested tree via `GET /menus?nested=1`.

### update: shallow inline vs replace tree

Inline assignments only accept:
- `name`, `handle`

Shallow inline updates PATCH **only those fields** (no `items`, no `replace=1`), so renaming a menu cannot flatten the tree.

Editing menu items requires `--file` or stdin with the full document. The body is normalized before write (`NormalizeMenuDocumentForWrite` strips `target_page` recursively and fills API aliases: `title`→`name`, `url`→`target_url`).

### create / update with a nested tree

Both `menus create --file` and `menus update --file` send nested `items[].children[]` through the menu contract and keep your ordering. Prefer these over raw `nimbu api` for menus so the CLI can normalize aliases and reject a write when verification shows that the server changed the tree.

Recipe for a nested menu (create in one shot, or create empty then replace):

```bash
# One-shot create with a tree
nimbu menus create --file tree.json

# Or: create empty, then replace the tree
nimbu menus create name=Main handle=main
nimbu menus update --menu main --file tree.json
```

`tree.json`:

```json
{
  "name": "Main",
  "handle": "main",
  "items": [
    { "name": "Home", "url": "/", "position": 0 },
    {
      "name": "Products", "url": "/products", "position": 1,
      "children": [
        { "name": "Wine",  "url": "/products/wine",  "position": 0 },
        { "name": "Boxes", "url": "/products/boxes", "position": 1 }
      ]
    }
  ]
}
```

- Nest via `children[]` on a parent item; a child may itself carry `children[]`.
- Item label/URL: API fields are `name` and `target_url`. The CLI also accepts `title`/`url` on write and copies them into `name`/`target_url`.
- Set `position` on each item to control sort order — without it the server may fall back to ordering by `name`.
- `menus update --file` reconciles the desired tree client-side and sends explicit tombstones for removed items; it does not use the server's recursive `replace=1` mode.
- The CLI verifies item count, hierarchy, ordering, and labels after the write. A changed tree is a hard failure.

### copy

`nimbu menus copy --only <slug> --from <site> --to <site>`

Default slug is `*` (all). For a single menu, prompts for overwrite confirmation unless `--force` is set.

## Blogs (`nimbu blogs`)

Subcommands: `list`, `get`, `create`, `update`, `delete`, `count`, `copy`.

Has a nested `posts` subcommand (alias: `articles`):

`nimbu blogs posts <list|get|create|update|delete|count> --blog <handle>`

Posts are scoped to a blog handle.

## Translations (`nimbu translations`)

Subcommands: `list`, `get`, `create`, `update`, `delete`, `count`, `copy`.

### Locale shorthand

In `create` and `update`, bare locale keys are automatically rewritten to `values.<locale>`:

```bash
nimbu translations create key=home.title nl=Welkom fr=Bienvenue en=Welcome
```

This is equivalent to:

```bash
nimbu translations create key=home.title values.nl=Welkom values.fr=Bienvenue values.en=Welcome
```

Reserved keys (`key`, `value`, `values`, `locale`, `url`) are NOT rewritten.
Locale keys are normalized, underscores become hyphens, and the result is
validated as a strict-lite BCP47 tag.

Duplicate locale assignments (e.g., both `nl=X` and `values.nl=Y`) produce an error.

Canonical casing follows BCP47 conventions (`nl_BE` → `nl-BE`,
`zh_hant_tw` → `zh-Hant-TW`). `translations create --file` accepts either one
object or an array of translation objects for batch creation.

## Notifications (`nimbu notifications`)

Subcommands: `list`, `get`, `create`, `update`, `delete`, `count`, `copy`, `pull`, `push`.

### pull / push disk layout

Templates live in `content/notifications/` relative to the project root (directory containing `nimbu.yml`).

```
content/notifications/
  welcome.txt          # base template (YAML front matter + text body)
  welcome.html         # optional HTML variant
  nl/
    welcome.txt        # locale override (front matter: subject only)
    welcome.html       # locale HTML override
  fr/
    welcome.txt
```

**Base template front matter** (required fields: `name`, `description`, `subject`):

```
---
description: Welcome email
name: welcome
subject: Welcome to our site
---

Hello {{ user.name }}, ...
```

**Locale override front matter** has only `subject`; the body is the localized text.

### pull

`nimbu notifications pull [--only slug1 --only slug2]`

Downloads all notifications into `content/notifications/`. Locale directories are created only when a translation differs from the base. Fails under `--readonly`.

### push

`nimbu notifications push [--only slug1 --only slug2]`

Reads local templates, validates locale directories against the site's configured locales (falls back to a built-in allowlist of 38 ISO codes), and upserts via the API. Each template is checked for existence first -- existing ones are updated, missing ones are created.

## Mails (`nimbu mails`)

**Alias for notifications pull/push only.** `nimbu mails pull` and `nimbu mails push` are identical to `nimbu notifications pull` and `nimbu notifications push`. No other notification subcommands are exposed under `mails`.

## Common flags

- `--readonly` -- rejects every mutating operation (create, update, delete, pull, push, copy, surgical writes, draft publish/discard)
- `--force` -- required for delete, `pages delete-block`, and `pages draft discard`; skips confirmation on copy overwrite
- `--locale <code>` -- select `content_locale` for localized content reads and writes
- `--dry-run` / `--diff` -- surgical verbs and `pages batch` resolve without writing / show a subtree diff; `pages update --dry-run` prints the merged body
- `--json` / `--plain` -- output format control
- `--all` -- fetch all pages (no pagination) for list commands
- `--page N` / `--per-page N` -- pagination (default: page 1, 25 per page)
