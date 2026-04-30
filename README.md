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

- `pages update` accepts inline updates for `title`, `template`, `published`, `locale`
- `menus update` accepts inline updates for `name`, `handle`
- deep/nested edits for pages and menus should use `--file` or stdin JSON

## Rich Resource Contracts

`pages`, `menus`, and `channels` have resource-specific contracts rather than generic CRUD payloads.

- `pages get` and `pages update` use page `fullpath` as the canonical identifier
- `pages get --page <fullpath> --json` returns the full page document, including nested `items`
- `pages get --page <fullpath> --download-assets DIR --json` downloads file editables and rewrites them to `attachment_path`
- `pages update --page <fullpath>` uses replace-safe patch semantics and supports `attachment_path` file refs in JSON
- `menus get --menu <slug> --json` returns the full nested menu tree
- `menus update` uses replace-safe patch semantics for nested menu updates
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

Locales are validated with a strict-lite BCP47 pattern (`nl`, `fr`, `nl-BE`, `zh-Hant`, ...).

## Commands

```
nimbu auth       Authentication and credentials
nimbu init       Bootstrap a local theme project
nimbu sites      Manage sites
nimbu channels   Manage channels and entries
nimbu pages      Manage pages
nimbu menus      Manage navigation menus
nimbu products   Manage products
nimbu collections Manage collections
nimbu coupons    Manage coupons
nimbu domains    Manage custom domains
nimbu orders     Manage orders
nimbu customers  Manage customers
nimbu mails      Sync notification templates to local files
nimbu accounts   Manage accounts
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
```

## Advanced Admin Endpoints

Some newer admin endpoints are intentionally left on the raw API surface until they justify dedicated CLI UX.

```bash
# Settings
nimbu sites settings --site my-site --json
nimbu api --method PATCH --path /settings/shipping --site my-site -d '{"bpost_label_qty":2}'

# Shipping rates
nimbu api --method GET --path /shipping_rates --site my-site

# Tax schemes
nimbu api --method GET --path /tax_schemes --site my-site

# Subscriptions
nimbu api --method GET --path /subscriptions --site my-site
```

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
nimbu themes pull --theme storefront
nimbu themes diff --theme storefront
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

Examples:

```bash
nimbu apps config
nimbu apps push --app storefront
nimbu apps push --app storefront --only code/main.js,code/hooks.js --only code/jobs/*.js
nimbu apps push --app storefront --sync --force
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
