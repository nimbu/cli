# nimbu-cli

Fast, AI-agent friendly CLI for the [Nimbu](https://nimbu.io) API.

## Features

- **Full API coverage** - Channels, pages, products, orders, customers, themes, and more
- **Secure credentials** - OS keychain storage (macOS Keychain, Linux Secret Service)
- **JSON-first output** - `--json` and `--plain` (TSV) modes for scripting
- **Agent-friendly** - Command allowlists, readonly mode, deterministic output
- **Shell completions** - Bash, Zsh, Fish, PowerShell

## Installation

### Homebrew

```bash
brew install nimbu/tap/nimbu-cli
```

### Build from Source

```bash
git clone https://github.com/nimbu/nimbu-go-cli.git
cd nimbu-go-cli
make build
./bin/nimbu-cli --help
```

## Quick Start

```bash
# Login to your Nimbu account
nimbu-cli auth login

# List your sites
nimbu-cli sites list

# Work with a specific site
nimbu-cli channels list --site my-site

# JSON output for scripting
nimbu-cli channels entries list blog --site my-site --json | jq '.[]'
```

## Commands

```
nimbu-cli auth       Authentication and credentials
nimbu-cli sites      Manage sites
nimbu-cli channels   Manage channels and entries
nimbu-cli pages      Manage pages
nimbu-cli menus      Manage navigation menus
nimbu-cli products   Manage products
nimbu-cli orders     Manage orders
nimbu-cli customers  Manage customers
nimbu-cli themes     Manage themes
nimbu-cli uploads    Manage uploads
nimbu-cli blogs      Manage blogs
nimbu-cli webhooks   Manage webhooks
nimbu-cli tokens     Manage API tokens
nimbu-cli config     Manage configuration
nimbu-cli api        Raw API access
nimbu-cli completion Generate shell completions
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

`~/.config/nimbu-cli/config.json`:

```json5
{
  default_site: "my-site",
  api_url: "https://api.nimbu.io",
  timeout: "30s",
}
```

### Project File

`.nimbu.json` in your project directory:

```json
{
  "site": "my-site",
  "theme": "default"
}
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
