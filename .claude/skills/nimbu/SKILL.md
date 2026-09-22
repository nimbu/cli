---
name: nimbu
description: >
  Nimbu CMS CLI — manage channels, pages, products, orders, customers, themes,
  translations, menus, blogs, notifications, redirects, uploads, webhooks,
  cloud code, and local dev server for Nimbu sites. Use when building,
  querying, migrating, or deploying Nimbu CMS content and themes.
metadata:
  version: "0.6.1"
---

# Nimbu CLI

The `nimbu` binary is a Go CLI for the [Nimbu](https://nimbu.io) CMS API.

## Documentation

- **Full docs**: https://docs.nimbu.io/
- **Liquid & themes**: https://docs.nimbu.io/docs/themes/introduction/overview.md
- **Cloud code**: https://docs.nimbu.io/docs/cloud-code/overview.md
- **REST API**: https://docs.nimbu.io/docs/api/overview.md

Refer to these for Liquid template syntax, theme structure, cloud code APIs, authentication, ACLs, and custom field payload shapes. This skill covers the CLI tool; the docs cover the platform concepts.

## Prerequisites

Install via Homebrew (`brew install nimbu/tap/nimbu`), Go (`go install github.com/nimbu/cli/cmd/nimbu-cli@latest`), or download from [GitHub Releases](https://github.com/nimbu/cli/releases).

Authenticate before use:

```bash
nimbu auth login          # Interactive browser OAuth — stores token in OS keychain
nimbu auth status         # Verify current credentials
nimbu auth scopes         # List granted scopes
```

Or set `NIMBU_TOKEN` env var to skip the keychain.

## Site Resolution

Commands that require a site resolve it in this order:

1. `--site` flag
2. `NIMBU_SITE` env var
3. `default_site` in `~/.config/nimbu/config.json`
4. `site` in `nimbu.yml`, found by walking up from `NIMBU_PROJECT_DIR` (if set), else the current working directory, stopping at the git top-level

Each walk stops at the first directory holding `.git` (that directory is still checked) or at the filesystem root. If nothing is found the error names that boundary: `nimbu.yml not found (searched up to /path/to/repo)`. A scratchpad CWD sits outside the repo, so the walk-up finds nothing — always pass `--site` (or set `NIMBU_SITE`) in subagent briefs.

## Output Modes

| Flag | Format | Use case |
|------|--------|----------|
| *(default)* | Human-readable table | Interactive use |
| `--json` | JSON | **Always use for agents** — structured, parseable |
| `--plain` | TSV | Piping to `cut`, `awk`, etc. |

Env overrides: `NIMBU_JSON=1`, `NIMBU_PLAIN=1`.

**Agent rule: always pass `--json` to get structured output.**

### Output Contract

In `--json` mode, **stdout is the bare JSON value** — nothing else:

- A list command prints a JSON **array**.
- A single-resource command (`get`, `create`, `update`) prints a JSON **object**.

Progress UI, applied-count lines, warnings, and the structured error envelope all go to **stderr**, never stdout. So `nimbu ... --json 2>/dev/null | jq` always gets clean, parseable JSON, and you can watch stderr separately for warnings (e.g. the `pages update --replace` dropped-editables warning).

## Global Flags

| Flag | Purpose |
|------|---------|
| `--site <id>` | Site ID or subdomain |
| `--locale <code>` | Select the content locale on localized resources |
| `--fields <list>` | Comma-separated fields to return |
| `--sort <field>` | Sort, e.g. `created_at:desc` |
| `--filters key=val` | Filter criteria (repeatable) |
| `--include <rels>` | Include related resources |
| `--force` | Skip confirmations for destructive ops |
| `--readonly` | Disable all write operations |
| `--no-input` | Never prompt; fail instead (CI mode) |
| `--no-progress` | Disable live progress UI |
| `--verbose` | Verbose logging |
| `--debug` | HTTP request/response traces |

## Localized Content

On pages, products, collections, blogs/posts, menus, notifications, and shipping
rates, `--locale` is sent as `content_locale`. Use it for locale-specific reads
and writes:

```bash
nimbu pages get --page about --locale nl --json
nimbu pages set --page about --path seo_title --locale nl "Nederlandse titel"
nimbu pages update --page about --locale nl --file nl.json
```

Human and plain output overlay the selected translation recursively; missing
translated fields fall back to the default-locale value. On `pages get`,
`pages get --outline` and `pages items`, `--json` projects the locale too
(`pages get --locale en --json` returns the EN content) while keeping the
`translations` map. Other resources return the raw document under `--json`.
Canonically equivalent keys such as `nl_BE` and `nl-BE` match during projection.

Use a nested `translations` object to write several locales in one
`pages update --file` request:

```json
{
  "translations": {
    "nl": {"seo_title": "Nederlandse titel"},
    "fr": {"seo_title": "Titre français"}
  }
}
```

A complete top-level map is also supported as `translations:=@translations.json`.

**Channel entries differ:** they have no `translations` map. `--locale` is a
per-locale read/write of top-level fields. See the Locales section in
[references/channels-and-entries.md](references/channels-and-entries.md) — do
not duplicate that contract here.

## Safety

Use these to constrain agent access:

- `NIMBU_READONLY=1` — blocks all create/update/delete/copy/push operations
- `NIMBU_ENABLE_COMMANDS=channels,pages` — allowlist of permitted command groups
- `NIMBU_NO_INPUT=1` — never prompt; fail instead (CI/agent mode)
- `NIMBU_COMPLETION_DEBUG=1` — writes dynamic completion diagnostics to `completion-debug.log` in the Nimbu data directory
- `--dry-run` — surgical page verbs, `pages update`, `pages batch`, and copy/sync/theme commands; resolve or print without writing
- `--force` — required for delete commands (including `pages delete-block` and `pages draft discard`)

## CLI Grammar

This v0.x CLI uses a clean flag-first grammar for resource identity. Do not use
old positional resource IDs in examples or generated commands; scripts using
those forms must be migrated before upgrading.

Resource identity is flag-first. Use flags such as `--site`, `--channel`,
`--entry`, `--page`, `--menu`, `--field`, `--product`, `--key`, `--domain`,
`--sender`, `--from`, and `--to` instead of positional IDs.

Payload stays last. Inline assignments trail identity flags and command options:

```bash
nimbu channels entries update --channel blog --entry welcome title="Welcome" published:=true
nimbu pages update --page about --file payload.json
nimbu sites settings --site staging --json
```

Use `nimbu sites settings --site staging`, not `nimbu sites settings staging`.

## Inline Payload Syntax

Most `create` and `update` commands accept inline assignments:

| Syntax | Meaning |
|--------|---------|
| `key=value` | String value |
| `key:=json` | Typed JSON (number, bool, object, array, null) |
| `key=@file.txt` | Raw file content as string |
| `key:=@file.json` | Parse JSON from file |

Dot paths for nesting: `seo.title="My Page"`.

`--file` and inline assignments are **mutually exclusive**. `--file` accepts a path, `-` for stdin, a pipe, or process substitution (`--file <(echo '{"title":"Hi"}')`).

```bash
nimbu products update --product sku-123 name="Wine Box" price:=29.9 seo.title="Gift box"
nimbu pages update --page about --file payload.json
```

## Command Map

### Content

| Command | Subcommands | Notes |
|---------|-------------|-------|
| `channels` | list, get, create, info, copy, diff, empty, delete, fields | `create` from JSON/inline; `delete` needs `--force` |
| `channels fields` | list, add, update, delete, apply, replace, diff | Channel field schema workflows |
| `channels entries` | list, get, create, update, delete, count, copy, gallery, watch | Entry CRUD within a channel; `watch` streams live changes over a websocket |
| `pages` | list, get, create, update, delete, set, insert, delete-block, move, batch, items, schema, draft, count, copy, versions | Fullpath as identifier. Surgical path verbs; `draft get\|save\|batch\|publish\|discard\|preview-url`; `--draft` on writes |
| `menus` | list, get, create, update, delete, count, copy | Nested tree structure |
| `blogs` | list, get, create, update, delete, count, copy | Has `posts` subcommand |
| `blogs posts` | list, get, create, update, delete, count | Blog post CRUD |
| `translations` | list, get, create, update, delete, count, copy | Locale shorthand support |
| `notifications` | list, get, create, update, delete, count, copy, pull, push | Template sync |
| `mails` | pull, push | Alias for `notifications` sync commands |

### Commerce

| Command | Subcommands | Notes |
|---------|-------------|-------|
| `products` | list, get, create, update, delete, count, copy, attachments | Product catalog |
| `shipping-rates` | list, get, create, update, delete | Site-specific shipping rates; no count/copy |
| `collections` | list, get, create, update, delete, count, copy | Product collections |
| `coupons` | list, get, create, update, delete, count, copy | Discount coupons |
| `orders` | list, get, update, count | **No create/delete** — orders are read-only except status |
| `customers` | list, get, create, update, delete, count, copy, roles | Customer records |

### Infrastructure

| Command | Subcommands | Notes |
|---------|-------------|-------|
| `themes` | list, get, cdn-root, pull, push, sync, diff, copy | `--only` auto-adds Liquid snippet/layout deps (stderr `adding dependency X (referenced by Y)`); `--no-deps` disables; `--dry-run` marks `(dependency)` |
| `themes layouts` | list, get, create, delete | `get` accepts `--layout` as alias for `--name` |
| `themes templates` | list, get, create, delete | `get` accepts `--template` as alias for `--name` |
| `themes snippets` | list, get, create, delete | `get` accepts `--snippet` as alias for `--name` |
| `themes assets` | list, get, create, delete | `get` accepts `--asset` as alias for `--path` |
| `themes files` | list, get, create, delete | Generic file CRUD |
| `apps` | list, get, config, push, logs, code | Cloud code management |
| `uploads` | list, get, create, download, delete, count | File uploads and exact-byte downloads |
| `webhooks` | list, get, create, update, delete, count | Webhook management |
| `redirects` | list, get, create, update, delete, copy | URL redirects |
| `roles` | list, get, create, update, delete, copy, customers | Permission roles. `update` replaces array fields; use `roles customers add\|remove\|set` to edit members |
| `announcements` | list, get, create, update, delete | HQ announcement management |
| `domain-registrations` | list, get, count, upsert, update | HQ domain registration management |
| `settings` | get, update, consent | Includes whole consent config get/update/replace/copy |
| `events` | track, ingest | Event submission |

### Operations

| Command | Subcommands | Notes |
|---------|-------------|-------|
| `sites` | list, get, current, count, settings, copy | Site management + full-site copy |
| `auth` | login, logout, status, scopes, token, keyring | Credential management |
| `server` | *(run directly)* | Local simulator proxy + child dev server; `--draft <page>` injects preview tokens |
| `init` | *(run directly)* | Bootstrap theme project with TUI |
| `config` | list, get, set, unset, banner, path | CLI configuration |
| `functions` | run | Execute cloud functions |
| `realtime` | grant | Mint a single-use realtime grant for custom websocket clients |
| `jobs` | list, run | Inspect and execute cloud jobs |
| `api` | get, post, put, patch, delete | Raw escape hatch: `nimbu api get /path`; legacy `--method/--path` remains valid |
| `commands` | *(run directly)* | Export the machine-readable CLI contract |
| `completion` | --shell bash/zsh/fish | Shell completions |

### Raw API and downloads

Prefer native commands. Use `nimbu api get|post|put|patch|delete /path` only as an escape hatch. `--data` accepts inline JSON, `@file`, or `-` for stdin; `--all` follows paginated array responses. The old `--method/--path` form remains compatible.

Binary commands require `--output=<file>` or `--output=-`. They preserve exact bytes, write files atomically, and refuse to overwrite without `--force`. Never replace a native product attachment, upload, app-code, page-version, settings, announcement, domain-registration, customer-role, or event workflow with raw API calls.

## Schema Discovery for Theme Development

```bash
nimbu channels fields list --channel blog --json   # types, flags, references, select_options
nimbu channels get --channel blog --json           # ACL, ordering, dependency graph
nimbu channels info --channel blog --typescript    # TypeScript interface (works cross-site)
```

**`channels fields list --channel <channel> --json`** returns an array of field definitions:

- `name` — field key used in templates and entry data
- `type` — `string`, `text`, `file`, `date`, `belongs_to`, `select`, `boolean`, `integer`, `float`, `geo`
- `label`, `hint` — human-readable label and help text
- `required`, `unique`, `localized`, `encrypted` — field flags
- `reference` — target channel slug for `belongs_to` / `has_many`
- `select_options` — available options for `select` fields

Always list fields before using channel data in templates.

## Gallery fields

Use the dedicated gallery commands instead of hand-building gallery JSON:

```bash
nimbu channels entries gallery list --channel=articles --entry=ENTRY --field=photos --json
nimbu channels entries gallery add --channel=articles --entry=ENTRY --field=photos --image=hero.jpg
nimbu channels entries gallery update --channel=articles --entry=ENTRY --field=photos --image-id=IMAGE --caption="Hero"
nimbu channels entries gallery remove --channel=articles --entry=ENTRY --field=photos --image-id=IMAGE --force
```

Inspect the channel schema and current gallery first. Use returned image IDs, never guessed IDs, and prefer `--dry-run --json` before risky replacements.

## Channel Field Workflows

```bash
nimbu channels fields list --channel blog --json
nimbu channels fields add --channel blog --name summary type=string label="Summary"
nimbu channels fields update --channel blog --field summary label="Teaser" required:=true
nimbu channels fields delete --channel blog --field summary --force
nimbu channels fields apply --channel blog --file fields.patch.json
nimbu channels fields replace --channel blog --file fields.json --force
nimbu channels fields diff --channel blog --file fields.json --json
```

## Rich Resource Contracts

Three resource types have special contracts beyond standard CRUD:

### Pages

- **Identifier**: fullpath (e.g., `about/team`), not UUID
- **Read the recipe, the path grammar and the gotchas in [references/pages-quickstart.md](references/pages-quickstart.md)** — two screens, working commands, nothing else needed for a normal page edit.
  - Read: `pages get --page P --outline [--locale xx]` for ids, paths and a content preview; `pages items --page P --path <path>` for one block; `pages schema` only for select options.
  - Write: `pages set` for one field, `pages batch --file ops.json` for several, `--draft` + `pages draft preview-url` + `pages draft publish` for QA.
  - Never put a raw `pages get --json` in context (270 KB for a landing page): use `--outline`, `--compact`, or `pages items`.
  - The full contract (FileRef shapes, `--replace` guard, draft errors, menus, blogs) stays in [references/pages-menus-content.md](references/pages-menus-content.md).
- **Fallback** for whole-document rewrites: `pages get --json --compact` → edit → `pages update --file` (merge by default; omitted canvases stay intact). `--replace` is a full destructive rebuild (file-only; guarded against wiping a canvas to 0 unless `--allow-empty-canvas`). Never blind-resend a raw GET under `--replace`.
- `pages update` inline stays shallow (`title`, `template`, `published`, `locale`, or one top-level `translations` object)
- `pages update --dry-run` prints the merged PATCH body without sending a request
- `pages create`/`update` `--help` lists `security_mechanism` (`none|humans|customers`) and `published`
- The CLI computes `If-Match` and retries once on 412; agents do not manage ETags
- File editables: use the FileRef table in [references/pages-menus-content.md](references/pages-menus-content.md) — do not invent write shapes

Path grammar, the batch ops table and the locale gotchas: [references/pages-quickstart.md](references/pages-quickstart.md).

### Draft -> preview -> publish QA loop

Edit a draft, preview it, then publish. The live page stays as-is until `pages draft publish`.

```bash
nimbu pages set --page about/team --path title --draft "Coming soon"
nimbu pages draft preview-url --page about/team
# screenshot / inspect (or: nimbu server --draft about/team)
nimbu pages draft publish --page about/team
```

A 409 `draft_base_changed` means the live page moved after the draft was based on it. Re-run with `--confirm`, or `nimbu pages draft discard --page about/team --force`.

### Agent rules

- Always pass `--site` (or set `NIMBU_SITE`) in subagent briefs.
- Run `nimbu commands --json` once to learn the live flag contract.
- `security_mechanism` (`none|humans|customers`): `none` is public; `customers` requires a logged-in customer; `humans` is the intermediate access level. Set it explicitly instead of guessing what the site default is; `pages get --json` shows the current value.
- FileRef write shapes: [pages-menus-content.md](references/pages-menus-content.md) table. Do not repeat it.
- Push the theme before content that uses new editables: `nimbu themes push --only templates/<t>.liquid --dry-run`, then without `--dry-run`.
- Prefer `--dry-run` / `--diff` before writes. Surgical `--dry-run` prints resolved ops; `pages update --dry-run` prints the exact PATCH body.
- The CLI owns ETags (`If-Match`, one 412 refetch). Do not send or cache them.
- `--file` accepts pipes and process substitution (`--file <(echo '{...}')`).

### Menus

- **Identifier**: slug/handle
- `menus get --menu <slug> --json` returns full nested tree (recovers via `?nested=1` when needed)
- `menus update --file` reconciles nested trees with explicit tombstones; inline `name`/`handle` is shallow
- `menus update` inline: `name`, `handle`, or a complete top-level `translations` object — other deep edits need `--file`

### Channels

- `channels get --channel <slug> --json` returns schema, customizations, ACL fields
- `channels info --channel <slug>` outputs TypeScript-friendly schema definition
- `channels diff --from <site> --to <site>` compares channel configs

## Gotchas

1. **Parent field on pages**: Set `parent` to the **fullpath string** (e.g., `"archive"`), NOT an object ID. The API resolves parents by path. `parent_path` is ignored on write.

2. **Inline update limits**: `pages update` and `menus update` inline assignments only accept shallow fields. Prefer surgical page verbs for nested edits; otherwise `--file`.

3. **`--file` vs inline**: Mutually exclusive. The CLI errors if both are provided.

4. **Delete requires `--force`**: All delete commands fail without `--force`, including `pages delete-block` and `pages draft discard`.

5. **Orders are read-only**: No `create` or `delete` — only `list`, `get`, `update` (status), `count`.

6. **Translations locale shorthand**: Bare locale keys like `nl=text` become `values.nl=text`. Keys are canonicalized (`nl_BE` → `nl-BE`); duplicates after canonicalization are rejected. `translations create --file` accepts one object or an array.

7. **Theme sync excludes `code/` and `content/`**: These directories are intentionally not managed by `themes push/sync`.

8. **Copy commands use `--from`/`--to` refs**: Format is `site` for site-level ops, `site/channel` for channel-level ops.

9. **Jobs are site-level**: `jobs run --wait` resolves the owning app from the server-side job registry; no `--app` or `nimbu.yml` needed.

10. **Fetched page `translations` are fallback-filled**: `pages get` resolves every locale with the default-locale values and does not mark fallbacks. Never write a fetched `translations` map (or `fullpath`/`public_url`/`depth`) back; `pages update` sends only what you assign. To set a localized slug: `echo '{"slug":"about-us"}' | nimbu pages update --page P --locale en --file -`.

10. **Shipping-rate translations are per-locale**: The API rejects nested `translations`. No `count`/`copy`. Keep `region_id` opaque (site-specific).

11. **Internal resources stay internal**: Accounts, regions, product types, and vendors are not public CLI commands. Do not replace them with raw API calls.

12. **Role membership is a full replace**: `roles update` with `customers`/`children`/`parents` replaces the whole relation (needs `--force` if it would drop more than half). Prefer `roles customers add|remove|set`. `__op` envelopes are sent verbatim. Use `--dry-run` on `roles update` to print the body without writing.

## Common Workflows

### List and filter channel entries

```bash
nimbu channels entries list --channel blog --site my-site --json --sort created_at:desc
```

See [references/channels-and-entries.md](references/channels-and-entries.md) for copy, diff, and schema workflows.

### Watch a channel for live changes

```bash
nimbu channels entries watch --channel blog --site my-site --json --for 30s
```

Streams one raw event envelope per line on stdout; status/control lines go to
stderr. Pagination, sort, projection, search and regex/geo operators are not
live-query operators and fail with exit code 2. See
[references/channels-and-entries.md](references/channels-and-entries.md).

### Edit a page (surgical, then fallback)

```bash
nimbu pages get --page about/team --outline
nimbu pages set --page about/team --path 'Blokken[0].Title' --dry-run --diff "Fast"
nimbu pages set --page about/team --path 'Blokken[0].Title' "Fast"
```

Full recipe: [references/pages-quickstart.md](references/pages-quickstart.md). Whole-document rewrite fallback: `pages get --json --compact` → edit → `pages update --file`. See [references/pages-menus-content.md](references/pages-menus-content.md).

### Upload a file and reuse its CDN URL

```bash
URL=$(nimbu uploads create --file ./logo.png --json | jq -r '.url')
```

Feed `url` into a file editable as `attachment_url` (see the FileRef table). `uploads create` accepts `--file`/`-f` and keeps `--source`.

### Push theme changes after build

```bash
nimbu themes push --only templates/page.liquid --dry-run   # deps auto-included
nimbu themes push --only templates/page.liquid
nimbu themes push --build --all
```

See [references/themes-and-local-dev.md](references/themes-and-local-dev.md) for `--only` dependency auto-include, `--no-deps`, local `--draft` preview, and cloud code.

### Copy a site between environments

```bash
nimbu sites copy --from staging-site --to production-site --dry-run --json
```

Site copy preserves localized blog/post documents and shared product locales.
Blog and product copy require explicit default locales on both sites and
promote the target default from the matching source locale. Site copy runs
pages before consent so page-backed privacy-policy IDs can be remapped; with
`--allow-errors`, a skipped privacy page also skips consent with a warning.
Shipping rates are inspected but skipped with a warning because their region IDs
are site-specific. See [references/site-migration.md](references/site-migration.md).

For a targeted localized blog copy:

```bash
nimbu blogs copy --from staging-site --to production-site --only news --json
```

### Manage the complete consent configuration

```bash
nimbu settings consent config get --site storefront --json
nimbu settings consent config update --site storefront enabled:=true
nimbu settings consent config replace --site storefront --file consent.json --force
nimbu settings consent config copy --from staging --to production --dry-run
```

`update` is partial. `replace` is file/stdin-only and requires `--force`.
`copy` preserves localized and unknown fields and remaps a page-backed privacy
policy by page fullpath; it stops before writing if the target page is missing.
Consent config copy itself does not require `--force`.

### Set up local development

```bash
nimbu init                        # Bootstrap nimbu.yml + theme structure
nimbu server                      # Start proxy + child dev server
nimbu server --draft about/team   # Same, rendering that page's draft
```

See [references/themes-and-local-dev.md](references/themes-and-local-dev.md).

## Error Handling

In `--json` mode, errors emit a structured envelope to stderr:

```json
{
  "status": "error",
  "error": {
    "code": "resource.not_found",
    "message": "page not found: about/missing",
    "hint": "",
    "exit_code": 5,
    "http_status": 404,
    "retryable": false
  }
}
```

A 422 `invalid editable` / `invalid slug` on `pages update`/`create` carries a hint: `nimbu themes push --only templates/<t>.liquid --dry-run`.

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Usage / invalid arguments |
| 3 | Authentication failure |
| 4 | Authorization / scope failure |
| 5 | Resource not found |
| 6 | Validation error |
| 7 | Rate limit exceeded (retryable) |
| 8 | Network error (retryable) |

### Canonical Error Codes

`auth.not_logged_in`, `auth.unauthorized`, `auth.forbidden`, `auth.scope_missing`,
`resource.not_found`, `resource.conflict`, `request.invalid`, `request.validation`,
`rate_limit.exceeded`, `network.timeout`, `network.failure`, `server.error`.

Check `retryable: true` before retrying failed requests.

## Pagination

List commands support:

| Flag | Purpose |
|------|---------|
| `--all` | Fetch all pages automatically |
| `--page N` | Specific page number |
| `--per-page N` | Items per page |

Default behavior returns the first page. Use `--all --json` for complete datasets.

## Reference Files

| File | Covers |
|------|--------|
| [channels-and-entries.md](references/channels-and-entries.md) | Channel CRUD, entry CRUD, schema, info, copy, diff, locales |
| [pages-quickstart.md](references/pages-quickstart.md) | **Start here for pages**: read/write recipe, path grammar, batch ops, locale gotchas |
| [pages-menus-content.md](references/pages-menus-content.md) | Pages (surgical verbs, drafts, FileRef), menus, blogs, translations, notifications |
| [products-orders-customers.md](references/products-orders-customers.md) | Products, shipping rates, orders, customers, collections, coupons |
| [themes-and-local-dev.md](references/themes-and-local-dev.md) | Theme sync, `--only` deps, local `--draft` preview, cloud code apps |
| [site-migration.md](references/site-migration.md) | Full-site copy, per-resource copy, cross-API migration |
