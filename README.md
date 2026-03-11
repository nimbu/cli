# nimbu

Fast, AI-agent friendly CLI for the [Nimbu](https://nimbu.io) API.

## Features

- **Broad API coverage** - Channels, pages, products, orders, customers, themes, and more
- **Secure credentials** - OS keychain storage (macOS Keychain, Linux Secret Service)
- **JSON-first output** - `--json` and `--plain` (TSV) modes for scripting
- **Agent-friendly** - Command allowlists, readonly mode, deterministic output
- **Shell completions** - Bash, Zsh, Fish

## Installation

### Homebrew

```bash
brew install --cask nimbu/tap/nimbu
```

### Build from Source

```bash
git clone https://github.com/nimbu/nimbu-go-cli.git
cd nimbu-go-cli
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

# JSON output for scripting
nimbu channels entries list blog --site my-site --json | jq '.[]'

# Start local simulator proxy + project dev server
nimbu server
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
nimbu products update product-123 name="Wine Box" price:=29.9 seo.title="Gift box"
```

`--file` and inline assignments are mutually exclusive.

```bash
# File payload
nimbu pages update about --file payload.json

# Inline payload
nimbu pages update about title="About us" published:=true
```

For richer document resources, inline updates stay intentionally shallow:

- `pages update` accepts inline updates for `title`, `template`, `published`, `locale`
- `menus update` accepts inline updates for `name`, `handle`
- deep/nested edits for pages and menus should use `--file` or stdin JSON

## Rich Resource Contracts

`pages`, `menus`, and `channels` have resource-specific contracts rather than generic CRUD payloads.

- `pages get` and `pages update` use page `fullpath` as the canonical identifier
- `pages get --json` returns the full page document, including nested `items`
- `pages get --download-assets DIR --json` downloads file editables and rewrites them to `attachment_path`
- `pages update` uses replace-safe patch semantics and supports `attachment_path` file refs in JSON
- `menus get --json` returns the full nested menu tree
- `menus update` uses replace-safe patch semantics for nested menu updates
- `channels get --json` returns the richer channel contract, including schema/customizations and ACL-oriented fields

Examples:

```bash
# Fetch a page by fullpath
nimbu pages get about/team --json

# Download page file editables and rewrite JSON to local file refs
nimbu pages get about/team --download-assets tmp/page-assets --json

# Replace-safe page update using a full document payload
nimbu pages update about/team --file page.json

# Nested menu fetch
nimbu menus get main --json

# Rich channel contract with schema and ACL data
nimbu channels get articles --json
```

### Translations shorthand

`translations create` and `translations update` support locale shorthand: top-level locale keys are mapped to `values.<locale>`.

```bash
nimbu translations update activate.label.lastname nl=Achternaam
nimbu translations update activate.label.lastname values.fr=Nom
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
nimbu themes     Manage themes
nimbu uploads    Manage uploads
nimbu blogs      Manage blogs
nimbu webhooks   Manage webhooks
nimbu server     Run local simulator proxy with child dev server
nimbu config     Manage configuration
nimbu api        Raw API access
nimbu completion Generate shell completions
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
    env:
      NIMBU_PROXY_URL: http://127.0.0.1:4568
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

Override example:

```bash
nimbu server --cmd pnpm --arg vite --arg --port --arg 5173 --ready-url http://127.0.0.1:5173
```

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
- `--only` narrows uploads to specific managed project-relative paths.
- `--liquid-only`, `--css-only`, `--js-only`, `--images-only`, and `--fonts-only`
  filter the managed set before upload/sync.
- `--prune` is only available on `themes sync` and deletes managed remote extras.

Examples:

```bash
nimbu themes push --build
nimbu themes push --liquid-only
nimbu themes push --only snippets/header.liquid --only stylesheets/theme.css
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
deletes remote files that no longer exist locally.

Examples:

```bash
nimbu apps config
nimbu apps push --app storefront
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
