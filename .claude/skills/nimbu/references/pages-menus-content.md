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

Any deeper field (editables, nested content) requires `--file` or stdin. The CLI fetches the current document, merges inline assignments on top, and PATCHes. This is a read-modify-write cycle, not a blind overwrite.

### update --file / stdin

When using `--file` (or `-` for stdin), the CLI sends the full document as a PATCH. `attachment_path` refs in file editables are auto-expanded: the CLI reads the local file, base64-encodes it, and sets `attachment`/`filename` before sending.

### create

Accepts `--file` or inline assignments. No inline key restrictions -- the body is POSTed as-is.

### copy

`nimbu pages copy --only <fullpath|prefix*> --from <site> --to <site>`

Copies pages between sites. Supports glob prefix (`archive/*`). Default is `*` (all pages). Requires `--write`. Cross-host copy supported via `--from-host` / `--to-host`.

### delete

Requires `--force`. Accepts page ID or slug.

## Menus (`nimbu menus`)

Subcommands: `list`, `get`, `create`, `update`, `delete`, `count`, `copy`.

### get returns nested tree

`nimbu menus get --menu <slug>` returns the full menu document including nested `items` tree with depth stats.

### update inline limits

Inline assignments only accept:
- `name`, `handle`

Editing menu items requires `--file` or stdin with the full document. The body is normalized before write (`NormalizeMenuDocumentForWrite`), so the CLI handles internal cleanup.

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

Reserved keys (`key`, `value`, `values`, `locale`, `url`) are NOT rewritten. Locale keys are normalized: lowercased, underscores become hyphens, validated against `[a-z]{2,3}(-[a-z0-9]{2,8})*`.

Duplicate locale assignments (e.g., both `nl=X` and `values.nl=Y`) produce an error.

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

Downloads all notifications into `content/notifications/`. Locale directories are created only when a translation differs from the base. Requires `--write`.

### push

`nimbu notifications push [--only slug1 --only slug2]`

Reads local templates, validates locale directories against the site's configured locales (falls back to a built-in allowlist of 38 ISO codes), and upserts via the API. Each template is checked for existence first -- existing ones are updated, missing ones are created.

## Mails (`nimbu mails`)

**Alias for notifications pull/push only.** `nimbu mails pull` and `nimbu mails push` are identical to `nimbu notifications pull` and `nimbu notifications push`. No other notification subcommands are exposed under `mails`.

## Common flags

- `--write` -- required for any mutating operation (create, update, delete, pull, push, copy)
- `--force` -- required for delete; skips confirmation on copy overwrite
- `--locale <code>` -- filter by locale on get/update for pages
- `--json` / `--plain` -- output format control
- `--all` -- fetch all pages (no pagination) for list commands
- `--page N` / `--per-page N` -- pagination (default: page 1, 25 per page)
