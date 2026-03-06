package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

// CompletionCmd generates shell completions.
type CompletionCmd struct {
	Shell string `arg:"" optional:"" help:"Shell to generate completions for (bash, zsh, fish)" default:"bash"`
}

func (c *CompletionCmd) Run(ctx context.Context) error {
	parser, _, err := newParser()
	if err != nil {
		return fmt.Errorf("create parser: %w", err)
	}

	switch c.Shell {
	case "bash":
		return writeBashCompletion(parser)
	case "zsh":
		return writeZshCompletion(parser)
	case "fish":
		return writeFishCompletion(parser)
	default:
		return fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish)", c.Shell)
	}
}

func writeBashCompletion(_ *kong.Kong) error {
	_, _ = fmt.Fprintln(os.Stdout, `# Bash completion for nimbu-cli
# Add this to your ~/.bashrc:
#   eval "$(nimbu-cli completion bash)"

_nimbu_cli_completions() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local commands="auth sites channels pages menus products collections coupons orders customers accounts notifications roles redirects functions jobs apps themes uploads blogs webhooks translations server config api completion"

    if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=($(compgen -W "${commands}" -- "${cur}"))
        return
    fi

    if [[ ${COMP_CWORD} -eq 2 ]]; then
        case "${COMP_WORDS[1]}" in
            auth)
                COMPREPLY=($(compgen -W "login logout status whoami scopes token keyring" -- "${cur}"))
                ;;
            sites)
                COMPREPLY=($(compgen -W "list get current count settings" -- "${cur}"))
                ;;
            channels)
                COMPREPLY=($(compgen -W "list get entries fields" -- "${cur}"))
                ;;
            pages|menus|products|customers|collections|coupons|notifications|translations)
                COMPREPLY=($(compgen -W "list get create update delete count" -- "${cur}"))
                ;;
            orders)
                COMPREPLY=($(compgen -W "list get update count" -- "${cur}"))
                ;;
            accounts)
                COMPREPLY=($(compgen -W "list count" -- "${cur}"))
                ;;
            roles|redirects)
                COMPREPLY=($(compgen -W "list get create update delete" -- "${cur}"))
                ;;
            functions|jobs)
                COMPREPLY=($(compgen -W "run" -- "${cur}"))
                ;;
            apps)
                COMPREPLY=($(compgen -W "list get code" -- "${cur}"))
                ;;
            themes)
                COMPREPLY=($(compgen -W "list get push sync layouts templates snippets assets files" -- "${cur}"))
                ;;
            uploads)
                COMPREPLY=($(compgen -W "list get create delete count" -- "${cur}"))
                ;;
            blogs)
                COMPREPLY=($(compgen -W "list get create update delete count posts articles" -- "${cur}"))
                ;;
            webhooks)
                COMPREPLY=($(compgen -W "list get create update delete count" -- "${cur}"))
                ;;
            config)
                COMPREPLY=($(compgen -W "list get set unset path" -- "${cur}"))
                ;;
            server)
                COMPREPLY=($(compgen -W "--cmd --arg --cwd --ready-url --ready-timeout --proxy-host --proxy-port --template-root --no-watch --watch-scan-interval --max-body-mb --quiet-requests --events-json --debug" -- "${cur}"))
                ;;
            completion)
                COMPREPLY=($(compgen -W "bash zsh fish" -- "${cur}"))
                ;;
        esac
    elif [[ ${COMP_WORDS[1]} == "themes" ]]; then
        case "${COMP_WORDS[2]}" in
            push|sync)
                COMPREPLY=($(compgen -W "--all --build --dry-run --theme" -- "${cur}"))
                ;;
            layouts|templates|snippets|assets)
                COMPREPLY=($(compgen -W "list get create delete" -- "${cur}"))
                ;;
            files)
                COMPREPLY=($(compgen -W "list get put delete" -- "${cur}"))
                ;;
        esac
    elif [[ ${COMP_WORDS[1]} == "apps" && ${COMP_WORDS[2]} == "code" ]]; then
        COMPREPLY=($(compgen -W "list create" -- "${cur}"))
    elif [[ ${COMP_WORDS[1]} == "blogs" && (${COMP_WORDS[2]} == "posts" || ${COMP_WORDS[2]} == "articles") ]]; then
        COMPREPLY=($(compgen -W "list get create update delete count" -- "${cur}"))
    elif [[ ${COMP_WORDS[1]} == "channels" && ${COMP_WORDS[2]} == "entries" ]]; then
        COMPREPLY=($(compgen -W "list get create update delete count" -- "${cur}"))
    fi
}

complete -F _nimbu_cli_completions nimbu-cli
complete -F _nimbu_cli_completions nb`)
	return nil
}

func writeZshCompletion(_ *kong.Kong) error {
	_, _ = fmt.Fprintln(os.Stdout, `#compdef nimbu-cli nb

# Zsh completion for nimbu-cli
# Add this to your ~/.zshrc:
#   eval "$(nimbu-cli completion zsh)"

_nimbu_cli() {
    local -a commands
    commands=(
        'auth:Authentication and credentials'
        'sites:Manage sites'
        'channels:Manage channels and entries'
        'pages:Manage pages'
        'menus:Manage navigation menus'
        'products:Manage products'
        'collections:Manage collections'
        'coupons:Manage coupons'
        'orders:Manage orders'
        'customers:Manage customers'
        'accounts:Manage accounts'
        'notifications:Manage notifications'
        'roles:Manage roles'
        'redirects:Manage redirects'
        'functions:Execute cloud functions'
        'jobs:Execute cloud jobs'
        'apps:Manage OAuth apps'
        'themes:Manage themes'
        'uploads:Manage uploads'
        'blogs:Manage blogs'
        'webhooks:Manage webhooks'
        'translations:Manage translations'
        'server:Run local simulator proxy with child dev server'
        'config:Manage configuration'
        'api:Raw API access'
        'completion:Generate shell completions'
    )

    if (( CURRENT == 2 )); then
        _describe -t commands 'command' commands
        return
    fi

    case "${words[2]}" in
        auth)
            local -a auth_commands
            auth_commands=(
                'login:Log in to Nimbu'
                'logout:Log out and remove stored credentials'
                'status:Show authentication status'
                'whoami:Show current authenticated user'
                'scopes:Show active token scopes'
                'token:Print access token for scripts'
                'keyring:Manage keyring backend'
            )
            _describe -t auth-commands 'auth command' auth_commands
            ;;
        sites)
            local -a sites_commands
            sites_commands=(
                'list:List accessible sites'
                'get:Get site details'
                'current:Show current site'
            )
            _describe -t sites-commands 'sites command' sites_commands
            ;;
        channels)
            local -a channels_commands
            channels_commands=(
                'list:List channels'
                'get:Get channel details'
                'entries:Manage channel entries'
                'fields:List channel fields'
            )
            _describe -t channels-commands 'channels command' channels_commands
            ;;
        themes)
            local -a themes_commands
            themes_commands=(
                'list:List themes'
                'get:Get theme details'
                'push:Upload managed local theme files'
                'sync:Upload and reconcile managed local theme files'
                'layouts:Manage layouts'
                'templates:Manage templates'
                'snippets:Manage snippets'
                'assets:Manage assets'
                'files:Manage theme files'
            )
            _describe -t themes-commands 'themes command' themes_commands
            ;;
        layouts|templates|snippets|assets)
            local -a theme_section_commands
            theme_section_commands=(
                'list:List section items'
                'get:Get section item'
                'create:Create or update section item'
                'delete:Delete section item'
            )
            _describe -t theme-section-commands 'theme section command' theme_section_commands
            ;;
        config)
            local -a config_commands
            config_commands=(
                'list:List all config values'
                'get:Get a config value'
                'set:Set a config value'
                'unset:Unset a config value'
                'path:Print config file path'
            )
            _describe -t config-commands 'config command' config_commands
            ;;
        apps)
            local -a apps_commands
            apps_commands=(
                'list:List apps'
                'get:Get app details'
                'code:Manage app code files'
            )
            _describe -t apps-commands 'apps command' apps_commands
            ;;
        completion)
            local -a completion_commands
            completion_commands=('bash' 'zsh' 'fish')
            _describe -t shells 'shell' completion_commands
            ;;
    esac
}

compdef _nimbu_cli nimbu-cli
compdef _nimbu_cli nb`)
	return nil
}

func writeFishCompletion(_ *kong.Kong) error {
	_, _ = fmt.Fprintln(os.Stdout, `# Fish completion for nimbu-cli
# Add this to your ~/.config/fish/config.fish:
#   nimbu-cli completion fish | source

# Main commands
complete -c nimbu-cli -n "__fish_use_subcommand" -a "auth" -d "Authentication and credentials"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "sites" -d "Manage sites"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "channels" -d "Manage channels and entries"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "pages" -d "Manage pages"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "menus" -d "Manage navigation menus"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "products" -d "Manage products"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "collections" -d "Manage collections"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "coupons" -d "Manage coupons"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "orders" -d "Manage orders"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "customers" -d "Manage customers"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "accounts" -d "Manage accounts"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "notifications" -d "Manage notifications"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "roles" -d "Manage roles"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "redirects" -d "Manage redirects"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "functions" -d "Execute cloud functions"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "jobs" -d "Execute cloud jobs"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "apps" -d "Manage OAuth apps"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "themes" -d "Manage themes"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "uploads" -d "Manage uploads"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "blogs" -d "Manage blogs"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "webhooks" -d "Manage webhooks"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "translations" -d "Manage translations"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "server" -d "Run local simulator proxy with child dev server"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "config" -d "Manage configuration"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "api" -d "Raw API access"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "completion" -d "Generate shell completions"

# Auth subcommands
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "login" -d "Log in to Nimbu"
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "logout" -d "Log out"
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "status" -d "Show authentication status"
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "whoami" -d "Show current user"
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "scopes" -d "Show active token scopes"
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "token" -d "Print access token"
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "keyring" -d "Manage keyring"

# Themes subcommands
complete -c nimbu-cli -n "__fish_seen_subcommand_from themes" -a "list get push sync layouts templates snippets assets files" -d "Theme commands"
complete -c nimbu-cli -n "__fish_seen_subcommand_from channels" -a "list get fields entries" -d "Channel commands"
complete -c nimbu-cli -n "__fish_seen_subcommand_from blogs" -a "list get create update delete count posts articles" -d "Blog commands"
complete -c nimbu-cli -n "__fish_seen_subcommand_from apps" -a "list get code" -d "App commands"
complete -c nimbu-cli -n "__fish_seen_subcommand_from functions" -a "run" -d "Run function"
complete -c nimbu-cli -n "__fish_seen_subcommand_from jobs" -a "run" -d "Run job"

# Theme section subcommands
complete -c nimbu-cli -n "__fish_seen_subcommand_from layouts" -a "list get create delete" -d "Manage layouts"
complete -c nimbu-cli -n "__fish_seen_subcommand_from templates" -a "list get create delete" -d "Manage templates"
complete -c nimbu-cli -n "__fish_seen_subcommand_from snippets" -a "list get create delete" -d "Manage snippets"
complete -c nimbu-cli -n "__fish_seen_subcommand_from assets" -a "list get create delete" -d "Manage assets"
complete -c nimbu-cli -n "__fish_seen_subcommand_from files" -a "list get put delete" -d "Manage theme files"
complete -c nimbu-cli -n "__fish_seen_subcommand_from code" -a "list create" -d "Manage app code files"

# Config subcommands
complete -c nimbu-cli -n "__fish_seen_subcommand_from config" -a "list" -d "List all config values"
complete -c nimbu-cli -n "__fish_seen_subcommand_from config" -a "get" -d "Get a config value"
complete -c nimbu-cli -n "__fish_seen_subcommand_from config" -a "set" -d "Set a config value"
complete -c nimbu-cli -n "__fish_seen_subcommand_from config" -a "unset" -d "Unset a config value"
complete -c nimbu-cli -n "__fish_seen_subcommand_from config" -a "path" -d "Print config file path"

# Completion shells
complete -c nimbu-cli -n "__fish_seen_subcommand_from completion" -a "bash zsh fish"

# Alias for nb
complete -c nb -w nimbu-cli`)
	return nil
}
