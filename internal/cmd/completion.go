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
    local commands="auth sites channels pages menus products orders customers themes uploads blogs webhooks tokens config api completion"

    if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=($(compgen -W "${commands}" -- "${cur}"))
        return
    fi

    case "${COMP_WORDS[1]}" in
        auth)
            COMPREPLY=($(compgen -W "login logout status whoami token keyring" -- "${cur}"))
            ;;
        sites)
            COMPREPLY=($(compgen -W "list get current" -- "${cur}"))
            ;;
        channels|pages|menus|products|customers)
            COMPREPLY=($(compgen -W "list get create update delete" -- "${cur}"))
            ;;
        orders)
            COMPREPLY=($(compgen -W "list get update count" -- "${cur}"))
            ;;
        themes)
            COMPREPLY=($(compgen -W "list get files" -- "${cur}"))
            ;;
        uploads)
            COMPREPLY=($(compgen -W "list get create delete" -- "${cur}"))
            ;;
        blogs)
            COMPREPLY=($(compgen -W "list get posts" -- "${cur}"))
            ;;
        webhooks)
            COMPREPLY=($(compgen -W "list get create update delete" -- "${cur}"))
            ;;
        tokens)
            COMPREPLY=($(compgen -W "list create revoke" -- "${cur}"))
            ;;
        config)
            COMPREPLY=($(compgen -W "list get set unset path" -- "${cur}"))
            ;;
        completion)
            COMPREPLY=($(compgen -W "bash zsh fish" -- "${cur}"))
            ;;
    esac
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
        'orders:Manage orders'
        'customers:Manage customers'
        'themes:Manage themes'
        'uploads:Manage uploads'
        'blogs:Manage blogs'
        'webhooks:Manage webhooks'
        'tokens:Manage API tokens'
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
            )
            _describe -t channels-commands 'channels command' channels_commands
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
complete -c nimbu-cli -n "__fish_use_subcommand" -a "orders" -d "Manage orders"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "customers" -d "Manage customers"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "themes" -d "Manage themes"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "uploads" -d "Manage uploads"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "blogs" -d "Manage blogs"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "webhooks" -d "Manage webhooks"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "tokens" -d "Manage API tokens"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "config" -d "Manage configuration"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "api" -d "Raw API access"
complete -c nimbu-cli -n "__fish_use_subcommand" -a "completion" -d "Generate shell completions"

# Auth subcommands
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "login" -d "Log in to Nimbu"
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "logout" -d "Log out"
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "status" -d "Show authentication status"
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "whoami" -d "Show current user"
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "token" -d "Print access token"
complete -c nimbu-cli -n "__fish_seen_subcommand_from auth" -a "keyring" -d "Manage keyring"

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
