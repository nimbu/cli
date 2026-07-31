# Pages, Menus & Content Commands

Quick-reference for content management commands. Covers gotchas that `--help` does not surface.

## Pages (`nimbu pages`)

Subcommands: `list`, `get`, `create`, `update`, `delete`, `count`, `copy`.

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

`--shape` emits just the structure (editable name → `type`, and for canvases the `repeatables` with their `slug` and nested skeleton), no content. It is the fastest way to learn the exact repeatable slugs before you write.
If combined with `--download-assets`, `--shape` wins and the CLI warns on stderr instead of downloading files.

### File editable write shapes

A file editable is `{ "type": "file", "file": { ... } }`. The `file` object accepts three write shapes — pick one:

1. **Base64 inline** — `{ "__type": "File", "attachment": "<base64>", "filename": "logo.png" }`. The `__type: "File"` marker is required (without it the server drops the upload). The CLI builds this for you: set `"attachment_path": "./logo.png"` in the file object and it reads the file, base64-encodes it, marks `__type` and sets `filename`.
2. **URL-populate (FileRef)** — `{ "__type": "FileRef", "source": "https://cdn.example.com/x.png" }`. The server copies the asset from that URL. You can send this shape directly, or use the CLI convention `"attachment_url": "https://..."` in the file object; the CLI rewrites it to the `FileRef` shape on write.
3. **Leave unchanged** — omit the `file` object entirely (or omit that editable). The existing asset is untouched.

A file editable that ends up with none of attachment / `attachment_path` / `attachment_url` / `source` is an **error** — the CLI refuses to write it rather than silently clearing the asset. A read-only `url` by itself does **not** count as a write payload. In default merge mode, the CLI drops URL-only file objects from the write payload so the existing asset is left unchanged; under `--replace`, provide a real write shape. To intentionally clear a file editable, pass `--allow-empty-file`.

> **Read vs write asymmetry.** On *read*, the public CDN link is in the `url` key (`items.<name>.file.url`). That `url` is a read-only convenience; it is NOT a write shape. To re-point a file editable at that URL, copy it into `attachment_url` (FileRef), don't leave it as `url`.

### create

Accepts `--file` or inline assignments. No inline key restrictions -- the body is POSTed as-is.

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

- `--readonly` -- rejects every mutating operation (create, update, delete, pull, push, copy)
- `--force` -- required for delete; skips confirmation on copy overwrite
- `--locale <code>` -- select `content_locale` for localized content reads and writes
- `--json` / `--plain` -- output format control
- `--all` -- fetch all pages (no pagination) for list commands
- `--page N` / `--per-page N` -- pagination (default: page 1, 25 per page)
