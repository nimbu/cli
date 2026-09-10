# 🍋 nimbu - the official Nimbu CLI  

[![CI](https://github.com/nimbu/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/nimbu/cli/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nimbu/cli)](https://goreportcard.com/report/github.com/nimbu/cli)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Fast, AI-agent friendly CLI for the [Nimbu](https://nimbu.io) API.

## Agent Skill

This CLI is available as an [open agent skill](https://skills.sh/) for AI assistants including
[Claude Code](https://claude.ai/code), [OpenClaw](https://openclaw.ai/),
[Codex](https://github.com/openai/codex), Cursor, GitHub Copilot, and
[35+ agents](https://github.com/vercel-labs/skills#supported-agents).

```bash
npx skills add nimbu/cli
```

## Features

- **Broad API coverage** - Channels, pages, products, orders, customers, themes, and more
- **Admin workflows** - Domains, sender verification, and support actions for orders/customers
- **Secure credentials** - OS keychain storage (macOS Keychain, Linux Secret Service)
- **JSON-first output** - `--json` and `--plain` (TSV) modes for scripting
- **Agent-friendly** - Command allowlists, readonly mode, deterministic output
- **Shell completions** - Bash, Zsh, Fish

## Installation

### Homebrew (macOS/Linux)

```bash
brew install nimbu/tap/nimbu
```

### Go

```bash
go install github.com/nimbu/cli/cmd/nimbu-cli@latest
```

### Download Binary

Download from [GitHub Releases](https://github.com/nimbu/cli/releases).

Available for:
- **macOS**: Intel & Apple Silicon (tar.gz)
- **Linux**: amd64 & arm64 (tar.gz)
- **Windows**: amd64 & arm64 (zip)

### Build from Source

```bash
git clone https://github.com/nimbu/cli.git
cd cli
make build
./bin/nimbu --help
```

## Quick Start

```bash
# Login to your Nimbu account
nimbu auth login

# List your sites
nimbu sites list

# Bootstrap a local theme project
nimbu init

# Work with a specific site
nimbu channels list --site my-site

# Manage custom domains
nimbu domains list --site my-site

# Verify a sender domain
nimbu senders verify-ownership --sender mail.example.com --site my-site

# JSON output for scripting
nimbu channels entries list --channel blog --site my-site --json | jq '.[]'

# Start local simulator proxy + project dev server
nimbu server
```

## CLI Grammar

This v0.x line intentionally makes a clean grammar cut: resource identity moved
from positional IDs to explicit flags. Existing scripts using positional
identity must be migrated before upgrading to this grammar.

Commands are flag-first for resource identity. Put identifiers in flags such as
`--site`, `--channel`, `--entry`, `--page`, `--menu`, `--field`, `--product`,
`--key`, `--domain`, `--sender`, `--from`, and `--to`.

Payload stays last. Inline assignments trail identity flags and command options:

```bash
nimbu channels entries update --channel blog --entry welcome title="Welcome" published:=true
nimbu pages update --page about --file payload.json
nimbu sites settings --site staging --json
```

## Upgrade Notes

`pages update --file` now merges by default. Use `--replace` with a file payload when existing automation depends on omission removing page content.

Migration examples:

```bash
# Old: nimbu channels get blog
nimbu channels get --channel blog

# Old: nimbu channels entries update blog welcome title=Welcome
nimbu channels entries update --channel blog --entry welcome title=Welcome
```

## Inline Payload Syntax

Most `create` and `update` commands accept inline assignments in addition to `--file` JSON payloads.

Operators:

- `key=value` - string value
- `key:=json` - typed JSON value (number, bool, object, array, null)
- `key=@file.txt` - read raw file content as string
- `key:=@file.json` - read and parse JSON from file

Use dot paths for nesting:

```bash
nimbu products update --product product-123 name="Wine Box" price:=29.9 seo.title="Gift box"
```

`--file` and inline assignments are mutually exclusive.

```bash
# File payload
nimbu pages update --page about --file payload.json

# Inline payload
nimbu pages update --page about title="About us" published:=true
```

For richer document resources, inline updates stay intentionally shallow:

- `pages update` accepts `title`, `template`, `published`, `locale`, or one
  complete top-level `translations` object
- `menus update` accepts `name`, `handle`, or one complete top-level
  `translations` object
- deep/nested edits for pages and menus should use `--file` or stdin JSON

## Rich Resource Contracts

`pages`, `menus`, and `channels` have resource-specific contracts rather than generic CRUD payloads.

- `pages get` and `pages update` use page `fullpath` as the canonical identifier
- `pages get --page <fullpath> --json` returns the full page document, including nested `items`
- `pages get --page <fullpath> --download-assets DIR --json` downloads file editables and rewrites them to `attachment_path`
- `pages update --page <fullpath>` uses replace-safe patch semantics and supports `attachment_path` file refs in JSON
- `menus get --menu <slug> --json` returns the full nested menu tree (recovers via `?nested=1` when needed)
- `menus update --file` reconciles nested menu trees with explicit tombstones; inline `name`/`handle` updates are shallow (no items rewrite)
- `channels get --channel <slug> --json` returns the richer channel contract, including schema/customizations and ACL-oriented fields

Examples:

```bash
# Fetch a page by fullpath
nimbu pages get --page about/team --json

# Download page file editables and rewrite JSON to local file refs
nimbu pages get --page about/team --download-assets tmp/page-assets --json

# Replace-safe page update using a full document payload
nimbu pages update --page about/team --file page.json

# Nested menu fetch
nimbu menus get --menu main --json

# Rich channel contract with schema and ACL data
nimbu channels get --channel articles --json

# Create a channel from a full JSON file (name, slug, title_field, customizations[])
nimbu channels create --file articles.json --json

# Create a channel from inline assignments (customizations as typed JSON)
nimbu channels create name=Articles slug=articles title_field=title customizations:=@fields.json

# Delete a whole channel definition (requires --force)
nimbu channels delete --channel articles --force
```

### Channel field workflows

`channels fields` manages a channel schema with the same grammar: identity in
flags, payload as trailing assignments or `--file`.

```bash
# Inspect fields
nimbu channels fields list --channel blog --json

# Add or update one field
nimbu channels fields add --channel blog --name summary type=string label="Summary"
nimbu channels fields update --channel blog --field summary label="Teaser" required:=true

# Delete one field
nimbu channels fields delete --channel blog --field summary --force

# Apply a partial schema patch from JSON
nimbu channels fields apply --channel blog --file fields.patch.json

# Replace the full field set from JSON
nimbu channels fields replace --channel blog --file fields.json --force

# Compare the current schema with a local JSON array
nimbu channels fields diff --channel blog --file fields.json --json
```

### Translations shorthand

`translations create` and `translations update` support locale shorthand: top-level locale keys are mapped to `values.<locale>`.

```bash
nimbu translations update --key activate.label.lastname nl=Achternaam
nimbu translations update --key activate.label.lastname values.fr=Nom
```

Locale keys are validated and canonicalized (`nl_BE` → `nl-BE`,
`zh_hant_tw` → `zh-Hant-TW`). `translations create --file` accepts either one
translation object or an array for batch creation.

### Localized content

For localized resources, `--locale` selects the content locale and is sent as
`content_locale`. This applies to pages, products, collections, blogs and
posts, menus, notifications, channel entries, and shipping rates.

```bash
# Update one locale without touching the default locale
nimbu pages update --page about --locale nl --file nl.json

# Update several locales in one request
nimbu pages update --page about --file translations.json
```

`nl.json` can contain `{"seo_title":"Nederlandse titel"}`.
`translations.json` can contain:

```json
{
  "translations": {
    "nl": {"seo_title": "Nederlandse titel"},
    "fr": {"seo_title": "Titre français"}
  }
}
```

Prefer `--file` for nested page content and multi-locale payloads. A complete
top-level map is also supported as `translations:=@translations.json`; other
deep page edits remain file-only. JSON output always preserves the complete API
response, including every `translations` map. Human and plain output
recursively overlay the selected locale and fall back to the default value when
a translated field is absent.

## Commands

```
nimbu auth       Authentication and credentials
nimbu init       Bootstrap a local theme project
nimbu sites      Manage sites
nimbu channels   Manage channels and entries
nimbu pages      Manage pages
nimbu menus      Manage navigation menus
nimbu products   Manage products
nimbu shipping-rates Manage shipping rates
nimbu collections Manage collections
nimbu coupons    Manage coupons
nimbu domains    Manage custom domains
nimbu orders     Manage orders
nimbu customers  Manage customers
nimbu mails      Sync notification templates to local files
nimbu settings   Manage site settings and consent configuration
nimbu notifications Manage notifications
nimbu roles      Manage roles
nimbu redirects  Manage redirects
nimbu functions  Execute cloud functions
nimbu jobs       Execute cloud jobs
nimbu apps       Manage OAuth apps
nimbu senders    Manage email sender domains
nimbu themes     Manage themes
nimbu uploads    Manage uploads
nimbu blogs      Manage blogs
nimbu webhooks   Manage webhooks
nimbu server     Run local simulator proxy with child dev server
nimbu config     Manage configuration
nimbu api        Raw API access
nimbu completion Generate shell completions
```

Completion debug output is normally hidden by shell wrappers. Set
`NIMBU_COMPLETION_DEBUG=1` and inspect the `completion-debug.log` file in the
Nimbu data directory when debugging dynamic completions.

## Admin Workflow Commands

```bash
# Make a domain primary
nimbu domains make-primary --domain shop.example.com --site my-site --force

# Trigger sender verification
nimbu senders verify --sender mail.example.com --site my-site

# Record manual payment for an order
nimbu orders pay --order 100012 --site my-site

# Resend customer confirmation
nimbu customers resend-confirmation --customer alice@example.com --site my-site

# Empty a channel with strict confirmation
nimbu channels empty --channel news --site my-site --confirm news --force

# Copy an existing Nimbu upload through FileRef
nimbu uploads create --site target-site --file-ref nimbu://archive-site/uploads/507f1f77bcf86cd799439014
```

## Advanced Workflows

```bash
# Native settings
nimbu settings update --section shipping --site my-site bpost_label_qty:=2

# Whole consent configuration
nimbu settings consent config get --site my-site --json
nimbu settings consent config update --site my-site enabled:=true
nimbu settings consent config replace --site my-site --file consent.json --force
nimbu settings consent config copy --from staging --to production --dry-run

# Shipping rates; translations use one write per locale
nimbu shipping-rates list --site my-site --json
nimbu shipping-rates create --site my-site name=Standard criteria=weight price:=7.5 region_id=REGION_ID
nimbu shipping-rates update --site my-site --rate RATE_ID --locale nl name="Standaard"
nimbu shipping-rates delete --site my-site --rate RATE_ID --force

# Role membership. Prefer add/remove/set over a raw customers array.
nimbu roles customers add --role bingo --site my-site --customer 6aa173bcac852eb6438192f1
nimbu roles customers remove --role bingo --site my-site --customer 6aa173bcac852eb6438192f1
nimbu roles customers set --role bingo --site my-site --customer 6aa173bcac852eb6438192f1
nimbu roles update --role bingo --site my-site --dry-run --file role.json
nimbu roles update --role bingo --site my-site --force --file role.json

# Media and version history
nimbu products attachments download --product PRODUCT --attachment ATTACHMENT --output manual.pdf
nimbu pages versions list --page about --site my-site

# Raw API remains an escape hatch
nimbu api patch /unsupported_endpoint --data @payload.json

# Legacy raw syntax remains compatible
nimbu api --method GET --path /subscriptions --site my-site
```

Shipping rates expose `list`, `get`, `create`, `update`, and `delete`; there is
no `count` or `copy`. The API does not accept a nested `translations` payload
for this resource, so create once and repeat `update --locale` for each language.
`region_id` is intentionally opaque: regions are site-specific and are not
exposed as a public CLI resource.

`roles update` treats `customers`, `children`, and `parents` as a full replace
when you send a JSON array. Shrinking any of those relations by more than half
requires `--force`. To clear a relation, send an empty array (`{"customers": []}`)
with `roles update --force`; `roles customers set` always requires at least one
`--customer`. `--dry-run` prints the request body and the projected member
counts without writing. Prefer `roles customers add|remove|set`: those
commands GET the role, PUT the full customer ID list, then GET again to verify.
The same `--force` rule applies when `remove` or `set` would drop more than
half of the members.

Relation `__op` envelopes (`AddReference`, `RemoveReference`, `Batch`, and the
`AddRelation` / `RemoveRelation` aliases) are a server-side feature. The CLI
sends them through verbatim and does not rewrite them into a local patch.

Accounts, regions, product types, and vendors are internal API resources and
are intentionally not exposed as public CLI commands.

## Configuration

### Environment Variables

```bash
NIMBU_SITE           # Default site ID
NIMBU_TOKEN          # Bearer token (overrides keychain)
NIMBU_API_URL        # API endpoint (default: https://api.nimbu.io)
NIMBU_JSON           # Default JSON output (1/true)
NIMBU_PLAIN          # Default TSV output (1/true)
NIMBU_NO_INPUT       # Disable prompts for CI
NIMBU_READONLY       # Disable write operations
NIMBU_ENABLE_COMMANDS # Command allowlist (comma-separated)
```

### Config File

`~/.config/nimbu/config.json`:

```json5
{
  default_site: "my-site",
  api_url: "https://api.nimbu.io",
  timeout: "30s",
}
```

### Project File

`nimbu.yml` in your project directory:

```yaml
site: my-site
theme: default
apps:
  - id: storefront
    name: storefront
    dir: code
    glob: "**/*.js"
    host: api.nimbu.io
    site: my-site
dev:
  proxy:
    host: 127.0.0.1
    port: 4568
    template_root: .
    watch: true
    watch_scan_interval: 3s
    max_body_mb: 64
  server:
    command: pnpm
    args:
      - vite
      - --port
      - "5173"
    cwd: .
    ready_url: http://127.0.0.1:5173
  routes:
    include:
      - POST /.well-known/*
sync:
  build:
    command: pnpm
    args:
      - build
  roots:
    assets:
      - images
      - fonts
      - javascripts
      - stylesheets
    layouts:
      - layouts
    templates:
      - templates
    snippets:
      - snippets
  generated:
    - javascripts/**
    - stylesheets/**
    - snippets/webpack_*.liquid
```

### Local Server Command

`nimbu server` starts:

1. Nimbu simulator proxy (default `http://127.0.0.1:4568`)
2. Child dev server command from `nimbu.yml`

Runtime notes:

- Child stdout/stderr is passed through unchanged.
- Proxy request lines are on by default: `2026-03-04T13:06:32.802Z GET / (200)`
- Use `--quiet-requests` to hide request lines.
- Child should proxy simulator requests to `NIMBU_PROXY_URL`.
- Vite starters may still accept `VITE_NIMBU_PROXY_URL` as a compatibility fallback, but `NIMBU_PROXY_URL` is the preferred name.
- `nimbu server` passes `NIMBU_DEV_PROXY_TOKEN` to the child dev server. Tools such as Vite can use it with `NIMBU_PROXY_URL` to register in-memory template overlays at `PUT /__nimbu/dev/templates/overlays` and clear them with `DELETE /__nimbu/dev/templates/overlays`.
- Template overlays are local-only, are never written to disk, and override disk templates with the same type/path while the dev proxy is running.

Override example:

```bash
nimbu server --cmd pnpm --arg vite --arg --port --arg 5173 --ready-url http://127.0.0.1:5173
```

### Dev Server Integration

Any dev server can integrate with `nimbu server` by reading the runtime environment that the CLI injects into the child process:

```bash
NIMBU_PROXY_URL        # Full simulator proxy URL, for example http://127.0.0.1:4568
NIMBU_PROXY_HOST       # Proxy host
NIMBU_PROXY_PORT       # Proxy port
NIMBU_DEV_PROXY_TOKEN  # Per-process token for local dev proxy APIs
NIMBU_SITE             # Current site ID, when available
```

The generated values override matching keys from `dev.server.env`, so project config cannot accidentally point the child at an old proxy or stale token.

Dev server responsibilities:

- Serve assets and HMR from the child dev server as usual.
- Proxy page and form requests that should render through Nimbu to `NIMBU_PROXY_URL`.
- Use `NIMBU_DEV_PROXY_TOKEN` only for local calls to `NIMBU_PROXY_URL`; do not expose it to browser code.
- Fail startup or show a clear terminal error if required overlay registration fails while `NIMBU_DEV_PROXY_TOKEN` is present.

Template overlays let a dev server provide virtual Liquid files without writing generated snippets to disk. Register the complete current overlay set with:

```http
PUT /__nimbu/dev/templates/overlays
X-Nimbu-Dev-Token: <NIMBU_DEV_PROXY_TOKEN>
Content-Type: application/json

{
  "templates": [
    {
      "type": "snippets",
      "path": "bundle_app.liquid",
      "content": "{% assign app_bundle_build_timestamp = \"dev\" %}\n"
    }
  ]
}
```

The `type` must be `layouts`, `templates`, or `snippets`; `path` is relative to that type and must end in `.liquid` or `.liquid.haml`. `PUT` replaces the full overlay set, so send every active virtual template each time. Send `{"templates":[]}` or call `DELETE /__nimbu/dev/templates/overlays` to clear overlays.

### Theme Push/Sync Commands

`nimbu themes push` uploads managed local theme resources without deleting remote
files.

`nimbu themes sync` uploads managed local theme resources and can also delete
managed remote resources that no longer exist locally.

`nimbu themes cdn-root` prints the resolved CDN root for the configured theme.

Supported managed resource kinds:

- `layouts/**`
- `templates/**`
- `snippets/**`
- asset roots such as `images/**`, `fonts/**`, `javascripts/**`, `stylesheets/**`

Notes:

- `code/**` and `content/**` are intentionally excluded from builtin theme sync.
- `--build` runs `sync.build` from `nimbu.yml` before collecting files.
- `--all` uploads the full managed file set.
- `--only` narrows uploads to specific managed files, directories, or globs and
  can be repeated; commas split multiple selectors.
- `--liquid-only`, `--css-only`, `--js-only`, `--images-only`, and `--fonts-only`
  select managed resource categories before upload/sync.
- `--no-images` excludes managed image assets from upload/sync.
- Explicit selectors and category flags are additive.
- `--prune` is only available on `themes sync` and deletes managed remote extras.

Examples:

```bash
nimbu themes push --build
nimbu themes push --liquid-only
nimbu themes push --only javascript/*.js --only stylesheets/*.css --only layouts/default.liquid --only snippets/bundle_app.liquid
nimbu themes sync --only javascript/*.js,stylesheets/*.css
nimbu themes push --only snippets/header.liquid --only stylesheets/theme.css
nimbu themes push --js-only --css-only --only layouts/default.liquid --only snippets/bundle_app.liquid
nimbu themes push --all --theme storefront
nimbu themes push --all --no-images
nimbu themes pull --theme storefront
nimbu themes diff --theme storefront
nimbu themes diff --content
nimbu themes cdn-root
nimbu themes copy --from source-site/storefront --to target-site/storefront
nimbu themes sync --build
nimbu themes sync --all --prune --dry-run
```

### Mail Template Sync

`nimbu notifications pull` and `nimbu notifications push` sync notification
templates between Nimbu and the legacy on-disk mail contract. `nimbu mails` is a
parity alias with the same `pull` and `push` subcommands.

Disk layout:

- `content/notifications/<slug>.txt`
- `content/notifications/<slug>.html`
- `content/notifications/<locale>/<slug>.txt`
- `content/notifications/<locale>/<slug>.html`

Text templates use YAML front matter:

```text
---
name: Order created
description: Sent after order creation
subject: Your order was created
---

Plain text body
```

Examples:

```bash
nimbu notifications pull
nimbu notifications push --only order_created
nimbu mails pull --only welcome
```

### Cloud Code App Workflows

`nimbu apps config` writes a host/site-scoped app entry to `nimbu.yml`.

`nimbu apps push` pushes local cloud code files for the selected configured app,
preserving dependency order for `require()` and static ESM imports. `--sync` also
deletes remote files that no longer exist locally. `--only` can be repeated and
each value may be comma-separated.

`nimbu apps code pull` pulls remote cloud code files for the selected configured
app into its local `dir`. `--only` accepts either remote names (`main.js`) or
project-relative paths (`code/main.js`). Pull overwrites selected local files but
does not delete local-only files.

`nimbu apps logs --app <app>` reads cloud code logs for a local app name or app key.
Use `--tail` to print the last `--limit` entries oldest-first and keep polling
for new log entries.

`nimbu sites copy` copies cloud code by default after content, theme,
notifications, redirects, and translations. Pass `--skip-cloud-code` to leave
target app code untouched.

Localized blog/post documents and product fields are preserved during copy.
Blog and product copy require explicit default locales on both sites, promote
the target default from the matching source locale, and preserve the remaining
shared translations. Site copy copies pages before consent so page-backed
privacy policies can be remapped safely. With `--allow-errors`, consent is
skipped with a warning when its page dependency was skipped. Shipping rates are inspected but not copied:
their `region_id` values are site-specific, so non-empty source rates produce a
warning instead.

Examples:

```bash
nimbu apps config
nimbu apps code pull --app storefront
nimbu apps code pull --app storefront --only main.js,code/jobs/daily.js
nimbu apps push --app storefront
nimbu apps push --app storefront --only code/main.js,code/hooks.js --only code/jobs/*.js
nimbu apps push --app storefront --sync --force
nimbu apps logs --app storefront --level error --query checkout
nimbu apps logs --app storefront --tail
nimbu apps logs --app storefront --job sync_products --level error
```

## Development

```bash
make build    # Build binary
make fmt      # Format code
make lint     # Run linter
make test     # Run tests
make ci       # Full CI check
```

## License

MIT
