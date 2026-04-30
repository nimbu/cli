package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

// CompletionCmd generates shell completions.
type CompletionCmd struct {
	Shell string `help:"Shell to generate completions for (bash, zsh, fish)" default:"bash"`
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
	_, _ = fmt.Fprintln(os.Stdout, `# Bash completion for nimbu
# Add this to your ~/.bashrc:
#   eval "$(nimbu completion --shell=bash)"

_nimbu_completions() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local prev=""
    if [[ ${COMP_CWORD} -gt 0 ]]; then
        prev="${COMP_WORDS[COMP_CWORD-1]}"
    fi
    if [[ "${cur}" == --site=* || "${cur}" == --from=* || "${cur}" == --to=* || "${cur}" == --channel=* || "${cur}" == --theme=* || "${prev}" == "--site" || "${prev}" == "--from" || "${prev}" == "--to" || "${prev}" == "--channel" || "${prev}" == "--theme" ]]; then
        local dynamic
        dynamic="$(nimbu __complete --shell bash --current="${cur}" -- "${COMP_WORDS[@]:0:COMP_CWORD}" 2>/dev/null)"
        if [[ -n "${dynamic}" ]]; then
            local no_space=0
            if [[ "${cur}" == --*=* ]]; then
                local flag_prefix="${cur%%=*}="
                local value_prefix="${cur#*=}"
                COMPREPLY=($(compgen -W "${dynamic}" -- "${value_prefix}"))
                local i
                for i in "${!COMPREPLY[@]}"; do
                    COMPREPLY[$i]="${flag_prefix}${COMPREPLY[$i]}"
                    if [[ "${COMPREPLY[$i]}" == */ ]]; then
                        no_space=1
                    fi
                done
            else
                COMPREPLY=($(compgen -W "${dynamic}" -- "${cur}"))
                local i
                for i in "${!COMPREPLY[@]}"; do
                    if [[ "${COMPREPLY[$i]}" == */ ]]; then
                        no_space=1
                    fi
                done
            fi
            if [[ ${no_space} -eq 1 ]]; then
                compopt -o nospace 2>/dev/null
            fi
            return
        fi
    fi
    if [[ "${cur}" == --* && "${cur}" != --*=* ]]; then
        local flags
        flags="$(nimbu __complete --shell bash --flag-names --current="${cur}" -- "${COMP_WORDS[@]:0:COMP_CWORD}" 2>/dev/null)"
        if [[ -n "${flags}" ]]; then
            COMPREPLY=($(compgen -W "${flags}" -- "${cur}"))
            return
        fi
    fi
    if [[ "${cur}" != -* ]]; then
        local command_names
        command_names="$(nimbu __complete --shell bash --command-names --current="${cur}" -- "${COMP_WORDS[@]:0:COMP_CWORD}" 2>/dev/null)"
        if [[ -n "${command_names}" ]]; then
            COMPREPLY=($(compgen -W "${command_names}" -- "${cur}"))
            return
        fi
    fi
    local commands="auth init sites channels pages menus products collections coupons domains orders customers mails accounts notifications roles redirects functions jobs apps senders themes uploads blogs webhooks translations server config api completion"

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
                COMPREPLY=($(compgen -W "list get current count settings copy" -- "${cur}"))
                ;;
            channels)
                COMPREPLY=($(compgen -W "list get info copy diff empty entries fields" -- "${cur}"))
                ;;
            pages|menus|collections|translations)
                COMPREPLY=($(compgen -W "list get create update delete count copy" -- "${cur}"))
                ;;
            coupons)
                COMPREPLY=($(compgen -W "list get create update delete count" -- "${cur}"))
                ;;
            customers)
                COMPREPLY=($(compgen -W "list get create update delete count copy fields config reset-password resend-confirmation" -- "${cur}"))
                ;;
            products)
                COMPREPLY=($(compgen -W "list get create update delete count fields config copy" -- "${cur}"))
                ;;
            mails)
                COMPREPLY=($(compgen -W "pull push" -- "${cur}"))
                ;;
            notifications)
                COMPREPLY=($(compgen -W "list get pull push create update delete count copy" -- "${cur}"))
                ;;
            orders)
                COMPREPLY=($(compgen -W "list get update count pay finish cancel reopen archive" -- "${cur}"))
                ;;
            accounts)
                COMPREPLY=($(compgen -W "list count" -- "${cur}"))
                ;;
            roles|redirects)
                COMPREPLY=($(compgen -W "list get create update delete copy" -- "${cur}"))
                ;;
            domains)
                COMPREPLY=($(compgen -W "list get create update delete make-primary" -- "${cur}"))
                ;;
            functions|jobs)
                COMPREPLY=($(compgen -W "run" -- "${cur}"))
                ;;
            apps)
                COMPREPLY=($(compgen -W "list get config push code" -- "${cur}"))
                ;;
            senders)
                COMPREPLY=($(compgen -W "list get create verify-ownership verify" -- "${cur}"))
                ;;
            themes)
                COMPREPLY=($(compgen -W "list get cdn-root pull diff copy push sync layouts templates snippets assets files" -- "${cur}"))
                ;;
            uploads)
                COMPREPLY=($(compgen -W "list get create delete count" -- "${cur}"))
                ;;
            blogs)
                COMPREPLY=($(compgen -W "list get create update delete count posts articles" -- "${cur}"))
                ;;
            webhooks)
                COMPREPLY=($(compgen -W "list get delete" -- "${cur}"))
                ;;
            config)
                COMPREPLY=($(compgen -W "list get set unset path banner" -- "${cur}"))
                ;;
            server)
                COMPREPLY=($(compgen -W "--cmd --arg --cwd --ready-url --ready-timeout --proxy-host --proxy-port --template-root --no-watch --watch-scan-interval --max-body-mb --quiet-requests --events-json --debug" -- "${cur}"))
                ;;
            completion)
                COMPREPLY=($(compgen -W "--shell" -- "${cur}"))
                ;;
        esac
    elif [[ ${COMP_WORDS[1]} == "themes" ]]; then
        case "${COMP_WORDS[2]}" in
            pull)
                COMPREPLY=($(compgen -W "--theme --liquid-only" -- "${cur}"))
                ;;
            diff)
                COMPREPLY=($(compgen -W "--theme" -- "${cur}"))
                ;;
            copy)
                COMPREPLY=($(compgen -W "--from --to --from-host --to-host --liquid-only" -- "${cur}"))
                ;;
            push|sync)
                COMPREPLY=($(compgen -W "--all --build --dry-run --since --theme --only --liquid-only --css-only --js-only --images-only --fonts-only --prune" -- "${cur}"))
                ;;
            layouts|templates|snippets|assets)
                COMPREPLY=($(compgen -W "list get create delete" -- "${cur}"))
                ;;
            files)
                COMPREPLY=($(compgen -W "list get put delete" -- "${cur}"))
                ;;
        esac
    elif [[ ${COMP_CWORD} -eq 3 && ${COMP_WORDS[1]} == "apps" ]]; then
        case "${COMP_WORDS[2]}" in
            push)
                COMPREPLY=($(compgen -W "--app --only --sync" -- "${cur}"))
                ;;
            code)
                COMPREPLY=($(compgen -W "list create" -- "${cur}"))
                ;;
        esac
    elif [[ ${COMP_WORDS[1]} == "blogs" && (${COMP_WORDS[2]} == "posts" || ${COMP_WORDS[2]} == "articles") ]]; then
        COMPREPLY=($(compgen -W "list get create update delete count" -- "${cur}"))
    elif [[ ${COMP_WORDS[1]} == "channels" && ${COMP_WORDS[2]} == "entries" ]]; then
        COMPREPLY=($(compgen -W "list get create update delete count copy" -- "${cur}"))
    elif [[ ${COMP_WORDS[1]} == "channels" && ${COMP_WORDS[2]} == "fields" ]]; then
        COMPREPLY=($(compgen -W "list add update delete apply replace diff" -- "${cur}"))
    elif [[ ${COMP_WORDS[1]} == "customers" && ${COMP_WORDS[2]} == "config" ]]; then
        COMPREPLY=($(compgen -W "copy diff" -- "${cur}"))
    elif [[ ${COMP_WORDS[1]} == "products" && ${COMP_WORDS[2]} == "config" ]]; then
        COMPREPLY=($(compgen -W "copy diff" -- "${cur}"))
    fi
}

complete -F _nimbu_completions nimbu
complete -F _nimbu_completions nb`)
	return nil
}

func writeZshCompletion(_ *kong.Kong) error {
	_, _ = fmt.Fprintln(os.Stdout, `#compdef nimbu nb

# Zsh completion for nimbu
# Add this to your ~/.zshrc:
#   eval "$(nimbu completion --shell=zsh)"

_nimbu() {
    local -a commands
    commands=(
        'auth:Authentication and credentials'
        'init:Bootstrap a local theme project'
        'sites:Manage sites'
        'channels:Manage channels and entries'
        'pages:Manage pages'
        'menus:Manage navigation menus'
        'products:Manage products'
        'collections:Manage collections'
        'coupons:Manage coupons'
        'domains:Manage custom domains'
        'orders:Manage orders'
        'customers:Manage customers'
        'mails:Sync notification templates to local files'
        'accounts:Manage accounts'
        'notifications:Manage notifications'
        'roles:Manage roles'
        'redirects:Manage redirects'
        'functions:Execute cloud functions'
        'jobs:Execute cloud jobs'
        'apps:Manage OAuth apps'
        'senders:Manage email sender domains'
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

    _nimbu_dynamic_complete() {
        local -a rows
        rows=("${(@f)$(nimbu __complete --shell zsh --current="${words[CURRENT]}" -- "${words[@]:1:$((CURRENT-1))}" 2>/dev/null)}")
        if (( ${#rows} == 0 )); then
            return 1
        fi
        local -a values
        local -a matches
        local -a descriptions
        local row value description
        local no_space=0
        for row in "${rows[@]}"; do
            value="${row%%$'\t'*}"
            if [[ "${value}" == */ ]]; then
                no_space=1
            fi
            if [[ "${row}" == *$'\t'* ]]; then
                description="${row#*$'\t'}"
                values+=("${value}:${description}")
                descriptions+=("${description}")
            else
                values+=("${value}")
                descriptions+=("")
            fi
            matches+=("${value}")
        done
        if [[ "${words[CURRENT]}" == --*=* ]]; then
            local flag_prefix="${words[CURRENT]%%=*}="
            local i
            for i in {1..${#values}}; do
                values[$i]="${flag_prefix}${values[$i]}"
                matches[$i]="${flag_prefix}${matches[$i]}"
            done
        fi
        if (( no_space )); then
            compadd -S '' -d descriptions -a matches
            return
        fi
        _describe -t nimbu-dynamic 'value' values
    }

    _nimbu_flag_complete() {
        local -a rows
        rows=("${(@f)$(nimbu __complete --shell zsh --flag-names --current="${words[CURRENT]}" -- "${words[@]:1:$((CURRENT-1))}" 2>/dev/null)}")
        if (( ${#rows} == 0 )); then
            return 1
        fi
        local -a values
        local row value description
        for row in "${rows[@]}"; do
            value="${row%%$'\t'*}"
            if [[ "${row}" == *$'\t'* ]]; then
                description="${row#*$'\t'}"
                values+=("${value}:${description}")
            else
                values+=("${value}")
            fi
        done
        _describe -t nimbu-flags 'flag' values
    }

    _nimbu_command_complete() {
        local -a rows
        rows=("${(@f)$(nimbu __complete --shell zsh --command-names --current="${words[CURRENT]}" -- "${words[@]:1:$((CURRENT-1))}" 2>/dev/null)}")
        if (( ${#rows} == 0 )); then
            return 1
        fi
        local -a values
        local row value description
        for row in "${rows[@]}"; do
            value="${row%%$'\t'*}"
            if [[ "${row}" == *$'\t'* ]]; then
                description="${row#*$'\t'}"
                values+=("${value}:${description}")
            else
                values+=("${value}")
            fi
        done
        _describe -t nimbu-commands 'command' values
    }

    if [[ "${words[CURRENT]}" == --site=* || "${words[CURRENT]}" == --from=* || "${words[CURRENT]}" == --to=* || "${words[CURRENT]}" == --channel=* || "${words[CURRENT]}" == --theme=* || "${words[CURRENT-1]}" == "--site" || "${words[CURRENT-1]}" == "--from" || "${words[CURRENT-1]}" == "--to" || "${words[CURRENT-1]}" == "--channel" || "${words[CURRENT-1]}" == "--theme" ]]; then
        _nimbu_dynamic_complete && return
    fi
    if [[ "${words[CURRENT]}" == --* && "${words[CURRENT]}" != --*=* ]]; then
        _nimbu_flag_complete && return
    fi
    if [[ "${words[CURRENT]}" != -* ]]; then
        _nimbu_command_complete && return
    fi

    if (( CURRENT == 2 )); then
        _describe -t commands 'command' commands
        return
    fi

    if (( CURRENT == 4 )); then
        case "${words[2]} ${words[3]}" in
            "channels entries")
                local -a channel_entry_commands
                channel_entry_commands=(
                    'list:List channel entries'
                    'get:Get channel entry'
                    'create:Create channel entry from JSON'
                    'update:Update channel entry'
                    'delete:Delete channel entry'
                    'count:Count channel entries'
                    'copy:Copy channel entries between sites'
                )
                _describe -t channel-entry-commands 'channel entry command' channel_entry_commands
                ;;
            "channels fields")
                local -a channel_field_commands
                channel_field_commands=(
                    'list:List channel fields'
                    'add:Add a channel field'
                    'update:Update a channel field'
                    'delete:Delete a channel field'
                    'apply:Apply channel fields from JSON'
                    'replace:Replace channel fields from JSON'
                    'diff:Diff channel fields between sites'
                )
                _describe -t channel-field-commands 'channel field command' channel_field_commands
                ;;
            "customers config"|"products config")
                local -a config_copy_commands
                config_copy_commands=(
                    'copy:Copy customizations between sites'
                    'diff:Diff customizations between sites'
                )
                _describe -t config-copy-commands 'config command' config_copy_commands
                ;;
            "themes layouts"|"themes templates"|"themes snippets"|"themes assets")
                local -a theme_section_commands
                theme_section_commands=(
                    'list:List section items'
                    'get:Get section item'
                    'create:Create or update section item'
                    'delete:Delete section item'
                )
                _describe -t theme-section-commands 'theme section command' theme_section_commands
                ;;
            "themes files")
                local -a theme_file_commands
                theme_file_commands=(
                    'list:List theme files'
                    'get:Get theme file'
                    'put:Create or update theme file'
                    'delete:Delete theme file'
                )
                _describe -t theme-file-commands 'theme file command' theme_file_commands
                ;;
            "blogs posts"|"blogs articles")
                local -a blog_post_commands
                blog_post_commands=(
                    'list:List blog posts'
                    'get:Get blog post'
                    'create:Create blog post from JSON'
                    'update:Update blog post'
                    'delete:Delete blog post'
                    'count:Count blog posts'
                )
                _describe -t blog-post-commands 'blog post command' blog_post_commands
                ;;
            "apps code")
                local -a app_code_commands
                app_code_commands=(
                    'list:List app code files'
                    'create:Create or update an app code file'
                )
                _describe -t app-code-commands 'app code command' app_code_commands
                ;;
        esac
        return
    fi

    if (( CURRENT != 3 )); then
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
                'count:Count accessible sites'
                'settings:Get site settings'
                'copy:Copy site configuration and content between sites'
            )
            _describe -t sites-commands 'sites command' sites_commands
            ;;
        mails)
            local -a mails_commands
            mails_commands=(
                'pull:Download notification templates to local files'
                'push:Upload local notification templates'
            )
            _describe -t mails-commands 'mails command' mails_commands
            ;;
        notifications)
            local -a notifications_commands
            notifications_commands=(
                'list:List notifications'
                'get:Get notification details'
                'create:Create notification from JSON'
                'update:Update notification'
                'delete:Delete notification'
                'count:Count notifications'
                'pull:Download notification templates to local files'
                'push:Upload local notification templates'
            )
            _describe -t notifications-commands 'notifications command' notifications_commands
            ;;
        channels)
            local -a channels_commands
            channels_commands=(
                'list:List channels'
                'get:Get channel details'
                'info:Show rich channel info'
                'copy:Copy channel configuration between sites'
                'diff:Diff channel configuration between sites'
                'empty:Empty a channel'
                'entries:Manage channel entries'
                'fields:Manage channel fields'
            )
            _describe -t channels-commands 'channels command' channels_commands
            ;;
        customers)
            local -a customers_commands
            customers_commands=(
                'list:List customers'
                'get:Get customer details'
                'create:Create customer from JSON'
                'update:Update customer'
                'delete:Delete customer'
                'count:Count customers'
                'copy:Copy customers between sites'
                'fields:Show customer field schema'
                'config:Copy or diff customer customizations'
                'reset-password:Reset a customer password'
                'resend-confirmation:Resend customer confirmation'
            )
            _describe -t customers-commands 'customers command' customers_commands
            ;;
        products)
            local -a products_commands
            products_commands=(
                'list:List products'
                'get:Get product details'
                'create:Create product from JSON'
                'update:Update product'
                'delete:Delete product'
                'count:Count products'
                'fields:Show product field schema'
                'config:Copy or diff product customizations'
                'copy:Copy products between sites'
            )
            _describe -t products-commands 'products command' products_commands
            ;;
        themes)
            local -a themes_commands
            themes_commands=(
                'list:List themes'
                'get:Get theme details'
                'cdn-root:Print the resolved theme CDN root'
                'pull:Download managed remote theme files'
                'diff:Show local vs remote liquid differences'
                'copy:Copy a theme between sites'
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
                'banner:Pick a banner theme interactively'
            )
            _describe -t config-commands 'config command' config_commands
            ;;
        apps)
            local -a apps_commands
            apps_commands=(
                'list:List apps'
                'get:Get app details'
                'config:Configure local cloud code app mapping'
                'push:Push local cloud code files'
                'code:Manage app code files'
            )
            _describe -t apps-commands 'apps command' apps_commands
            ;;
        domains)
            local -a domains_commands
            domains_commands=(
                'list:List domains'
                'get:Get domain details'
                'create:Create a domain'
                'update:Update a domain'
                'delete:Delete a domain'
                'make-primary:Make a domain primary'
            )
            _describe -t domains-commands 'domains command' domains_commands
            ;;
        senders)
            local -a senders_commands
            senders_commands=(
                'list:List sender domains'
                'get:Get sender domain details'
                'create:Create a sender domain'
                'verify-ownership:Verify sender domain ownership'
                'verify:Verify sender domain DNS'
            )
            _describe -t senders-commands 'senders command' senders_commands
            ;;
        completion)
            local -a completion_commands
            completion_commands=('--shell')
            _describe -t shells 'shell option' completion_commands
            ;;
    esac
}

compdef _nimbu nimbu
compdef _nimbu nb`)
	return nil
}

func writeFishCompletion(_ *kong.Kong) error {
	_, _ = fmt.Fprintln(os.Stdout, `# Fish completion for nimbu
# Add this to your ~/.config/fish/config.fish:
#   nimbu completion --shell=fish | source

function __fish_nimbu_dynamic_complete
    set -l current (commandline -ct)
    set -l tokens (commandline -opc)
    command nimbu __complete --shell fish --current="$current" -- $tokens 2>/dev/null
end

function __fish_nimbu_complete_flags
    set -l current (commandline -ct)
    set -l tokens (commandline -opc)
    command nimbu __complete --shell fish --flag-names --current="$current" -- $tokens 2>/dev/null
end

function __fish_nimbu_complete_commands
    set -l current (commandline -ct)
    set -l tokens (commandline -opc)
    command nimbu __complete --shell fish --command-names --current="$current" -- $tokens 2>/dev/null
end

function __fish_nimbu_dynamic_flag
    set -l current (commandline -ct)
    set -l tokens (commandline -opc)
    set -l previous ""
    if test (count $tokens) -gt 0
        set previous $tokens[-1]
    end
    string match -q -- "--site=*" "$current"; or string match -q -- "--from=*" "$current"; or string match -q -- "--to=*" "$current"; or string match -q -- "--channel=*" "$current"; or string match -q -- "--theme=*" "$current"; or test "$previous" = "--site"; or test "$previous" = "--from"; or test "$previous" = "--to"; or test "$previous" = "--channel"; or test "$previous" = "--theme"
end

function __fish_nimbu_flag_name
    set -l current (commandline -ct)
    string match -q -- "--*" "$current"; and not string match -q -- "--*=*" "$current"
end

complete -c nimbu -n "__fish_nimbu_dynamic_flag" -f -a "(__fish_nimbu_dynamic_complete)"
complete -c nb -n "__fish_nimbu_dynamic_flag" -f -a "(__fish_nimbu_dynamic_complete)"
complete -c nimbu -n "__fish_nimbu_flag_name" -f -a "(__fish_nimbu_complete_flags)"
complete -c nb -n "__fish_nimbu_flag_name" -f -a "(__fish_nimbu_complete_flags)"
complete -c nimbu -f -a "(__fish_nimbu_complete_commands)"
complete -c nb -f -a "(__fish_nimbu_complete_commands)"

# Main commands
complete -c nimbu -n "__fish_use_subcommand" -a "auth" -d "Authentication and credentials"
complete -c nimbu -n "__fish_use_subcommand" -a "init" -d "Bootstrap a local theme project"
complete -c nimbu -n "__fish_use_subcommand" -a "sites" -d "Manage sites"
complete -c nimbu -n "__fish_use_subcommand" -a "channels" -d "Manage channels and entries"
complete -c nimbu -n "__fish_use_subcommand" -a "pages" -d "Manage pages"
complete -c nimbu -n "__fish_use_subcommand" -a "menus" -d "Manage navigation menus"
complete -c nimbu -n "__fish_use_subcommand" -a "products" -d "Manage products"
complete -c nimbu -n "__fish_use_subcommand" -a "collections" -d "Manage collections"
complete -c nimbu -n "__fish_use_subcommand" -a "coupons" -d "Manage coupons"
complete -c nimbu -n "__fish_use_subcommand" -a "domains" -d "Manage custom domains"
complete -c nimbu -n "__fish_use_subcommand" -a "orders" -d "Manage orders"
complete -c nimbu -n "__fish_use_subcommand" -a "customers" -d "Manage customers"
complete -c nimbu -n "__fish_use_subcommand" -a "mails" -d "Sync notification templates to local files"
complete -c nimbu -n "__fish_use_subcommand" -a "accounts" -d "Manage accounts"
complete -c nimbu -n "__fish_use_subcommand" -a "notifications" -d "Manage notifications"
complete -c nimbu -n "__fish_use_subcommand" -a "roles" -d "Manage roles"
complete -c nimbu -n "__fish_use_subcommand" -a "redirects" -d "Manage redirects"
complete -c nimbu -n "__fish_use_subcommand" -a "functions" -d "Execute cloud functions"
complete -c nimbu -n "__fish_use_subcommand" -a "jobs" -d "Execute cloud jobs"
complete -c nimbu -n "__fish_use_subcommand" -a "apps" -d "Manage OAuth apps"
complete -c nimbu -n "__fish_use_subcommand" -a "senders" -d "Manage email sender domains"
complete -c nimbu -n "__fish_use_subcommand" -a "themes" -d "Manage themes"
complete -c nimbu -n "__fish_use_subcommand" -a "uploads" -d "Manage uploads"
complete -c nimbu -n "__fish_use_subcommand" -a "blogs" -d "Manage blogs"
complete -c nimbu -n "__fish_use_subcommand" -a "webhooks" -d "Manage webhooks"
complete -c nimbu -n "__fish_use_subcommand" -a "translations" -d "Manage translations"
complete -c nimbu -n "__fish_use_subcommand" -a "server" -d "Run local simulator proxy with child dev server"
complete -c nimbu -n "__fish_use_subcommand" -a "config" -d "Manage configuration"
complete -c nimbu -n "__fish_use_subcommand" -a "api" -d "Raw API access"
complete -c nimbu -n "__fish_use_subcommand" -a "completion" -d "Generate shell completions"

# Auth subcommands
complete -c nimbu -n "__fish_seen_subcommand_from auth" -a "login" -d "Log in to Nimbu"
complete -c nimbu -n "__fish_seen_subcommand_from auth" -a "logout" -d "Log out"
complete -c nimbu -n "__fish_seen_subcommand_from auth" -a "status" -d "Show authentication status"
complete -c nimbu -n "__fish_seen_subcommand_from auth" -a "whoami" -d "Show current user"
complete -c nimbu -n "__fish_seen_subcommand_from auth" -a "scopes" -d "Show active token scopes"
complete -c nimbu -n "__fish_seen_subcommand_from auth" -a "token" -d "Print access token"
complete -c nimbu -n "__fish_seen_subcommand_from auth" -a "keyring" -d "Manage keyring"
complete -c nimbu -n "__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from keyring; and not __fish_seen_subcommand_from show set" -a "show set" -d "Keyring commands"

# Themes subcommands
complete -c nimbu -n "__fish_seen_subcommand_from themes" -a "list get cdn-root pull diff copy push sync layouts templates snippets assets files" -d "Theme commands"
complete -c nimbu -n "__fish_seen_subcommand_from sites" -a "list get current count settings copy" -d "Site commands"
complete -c nimbu -n "__fish_seen_subcommand_from channels" -a "list get info copy diff empty fields entries" -d "Channel commands"
complete -c nimbu -n "__fish_seen_subcommand_from channels entries" -a "list get create update delete count copy" -d "Channel entry commands"
complete -c nimbu -n "__fish_seen_subcommand_from channels fields" -a "list add update delete apply replace diff" -d "Channel field commands"
complete -c nimbu -n "__fish_seen_subcommand_from customers" -a "list get create update delete count copy fields config reset-password resend-confirmation" -d "Customer commands"
complete -c nimbu -n "__fish_seen_subcommand_from customers config" -a "copy diff" -d "Customer config commands"
complete -c nimbu -n "__fish_seen_subcommand_from products" -a "list get create update delete count fields config copy" -d "Product commands"
complete -c nimbu -n "__fish_seen_subcommand_from products config" -a "copy diff" -d "Product config commands"
complete -c nimbu -n "__fish_seen_subcommand_from pages" -a "list get create update delete count copy" -d "Page commands"
complete -c nimbu -n "__fish_seen_subcommand_from menus" -a "list get create update delete count copy" -d "Menu commands"
complete -c nimbu -n "__fish_seen_subcommand_from blogs" -a "list get create update delete count posts articles copy" -d "Blog commands"
complete -c nimbu -n "__fish_seen_subcommand_from apps; and not __fish_seen_subcommand_from list get config push code" -a "list get config push code" -d "App commands"
complete -c nimbu -n "__fish_seen_subcommand_from mails" -a "pull push" -d "Mail commands"
complete -c nimbu -n "__fish_seen_subcommand_from notifications" -a "list get pull push create update delete count copy" -d "Notification commands"
complete -c nimbu -n "__fish_seen_subcommand_from translations" -a "list get create update delete count copy" -d "Translation commands"
complete -c nimbu -n "__fish_seen_subcommand_from collections" -a "list get create update delete count copy" -d "Collection commands"
complete -c nimbu -n "__fish_seen_subcommand_from coupons" -a "list get create update delete count" -d "Coupon commands"
complete -c nimbu -n "__fish_seen_subcommand_from domains" -a "list get create update delete make-primary" -d "Domain commands"
complete -c nimbu -n "__fish_seen_subcommand_from orders" -a "list get update count pay finish cancel reopen archive" -d "Order commands"
complete -c nimbu -n "__fish_seen_subcommand_from accounts" -a "list count" -d "Account commands"
complete -c nimbu -n "__fish_seen_subcommand_from roles" -a "list get create update delete copy" -d "Role commands"
complete -c nimbu -n "__fish_seen_subcommand_from redirects" -a "list get create update delete copy" -d "Redirect commands"
complete -c nimbu -n "__fish_seen_subcommand_from functions" -a "run" -d "Run function"
complete -c nimbu -n "__fish_seen_subcommand_from jobs" -a "run" -d "Run job"
complete -c nimbu -n "__fish_seen_subcommand_from senders" -a "list get create verify-ownership verify" -d "Sender commands"
complete -c nimbu -n "__fish_seen_subcommand_from webhooks" -a "list get delete" -d "Webhook commands"

# Theme section subcommands
complete -c nimbu -n "__fish_seen_subcommand_from layouts" -a "list get create delete" -d "Manage layouts"
complete -c nimbu -n "__fish_seen_subcommand_from templates" -a "list get create delete" -d "Manage templates"
complete -c nimbu -n "__fish_seen_subcommand_from snippets" -a "list get create delete" -d "Manage snippets"
complete -c nimbu -n "__fish_seen_subcommand_from assets" -a "list get create delete" -d "Manage assets"
complete -c nimbu -n "__fish_seen_subcommand_from files" -a "list get put delete" -d "Manage theme files"
complete -c nimbu -n "__fish_seen_subcommand_from apps; and __fish_seen_subcommand_from code; and not __fish_seen_subcommand_from list create" -a "list create" -d "Manage app code files"

# Config subcommands
complete -c nimbu -n "__fish_seen_subcommand_from config" -a "list" -d "List all config values"
complete -c nimbu -n "__fish_seen_subcommand_from config" -a "get" -d "Get a config value"
complete -c nimbu -n "__fish_seen_subcommand_from config" -a "set" -d "Set a config value"
complete -c nimbu -n "__fish_seen_subcommand_from config" -a "unset" -d "Unset a config value"
complete -c nimbu -n "__fish_seen_subcommand_from config" -a "path" -d "Print config file path"
complete -c nimbu -n "__fish_seen_subcommand_from config" -a "banner" -d "Pick a banner theme interactively"

# Completion shells
complete -c nimbu -n "__fish_seen_subcommand_from completion" -l shell -d "Shell to generate completions for"

# Alias for nb
complete -c nb -w nimbu`)
	return nil
}
