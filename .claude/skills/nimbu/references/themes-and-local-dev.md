# Themes and Local Development

For Liquid template syntax and theme concepts, see https://docs.nimbu.io/themes/introduction/overview.html.
For cloud code APIs and patterns, see https://docs.nimbu.io/cloud-code/overview.html.

## Managed resource kinds

Theme sync operates on four resource kinds, each mapped to local directories via `sync.roots` in `nimbu.yml`:

| Kind       | Default local roots                              | API collection |
|------------|--------------------------------------------------|----------------|
| `layout`   | `layouts/`                                       | layouts        |
| `template` | `templates/`                                     | templates      |
| `snippet`  | `snippets/`                                      | snippets       |
| `asset`    | `images/`, `fonts/`, `javascripts/`, `stylesheets/` | assets      |

Layouts, templates, and snippets are Liquid files. Assets are binary/text files served from the theme CDN.

Override defaults with `sync.roots` in `nimbu.yml` -- setting any key replaces all defaults for that kind:

```yaml
sync:
  roots:
    assets: [dist/images, dist/fonts, dist/js, dist/css]
    snippets: [snippets, partials]
```

### Excluded directories

`code/` and `content/` are rejected as sync roots (`validateRootPath` returns an error). They are not theme resources -- `code/` holds cloud code files managed by `nimbu apps push`, and `content/` is for content exports.

## Push vs sync vs pull

| Command             | Uploads | Deletes remote | Default scope           |
|---------------------|---------|----------------|-------------------------|
| `nimbu themes push` | Yes     | Never          | Git-changed + generated |
| `nimbu themes sync` | Yes     | Only with `--prune` | Git-changed + generated |
| `nimbu themes pull` | N/A     | N/A            | Downloads all remote    |

By default, push and sync use **git-based change detection**: `git diff --name-status HEAD` plus `git ls-files --others --exclude-standard`. Files matching `sync.generated` patterns are always included (default: `javascripts/**`, `stylesheets/**`, `snippets/webpack_*.liquid`). If the project is not a git repo or has no HEAD commit, all files are uploaded.

**Selection flags bypass git detection.** When `--only` or any category filter (`--liquid-only`, `--css-only`, etc.) is used without `--since`, the CLI pushes all matching files regardless of git status. This means `nimbu themes push --only templates/page.liquid` always pushes that file, even if it has no uncommitted changes.

The `--since` flag changes the git comparison ref. When combined with a category filter, it intersects: `--css-only --since origin/main` pushes only CSS files changed since origin/main.

| Flags | Scope | Behavior |
|-------|-------|----------|
| *(none)* | git diff HEAD | Push uncommitted changes |
| `--since <ref>` | git diff REF | Push changes since ref (e.g. committed but not pushed) |
| `--all` | all files | Push everything |
| `--only <path>[,<path>...]` | all files | Push specific files (bypasses git) |
| `--css-only` | all files | Push all CSS files (bypasses git) |
| `--css-only --since <ref>` | git diff REF | Push CSS files changed since ref |

### Key flags (push and sync)

- `--all` -- upload every managed local file, ignore git status
- `--build` -- run `sync.build.command` before collecting files
- `--dry-run` -- print planned operations without executing
- `--since <ref>` -- compare against this git ref instead of HEAD (e.g. `origin/main`); useful for pushing committed-but-not-pushed changes
- `--only <path>[,<path>...]` -- push specific project-relative files; commas split multiple selectors and the flag is repeatable; bypasses git change detection; path must be inside a managed root. Note: `--since` is ignored when `--only` is set
- `--theme <id>` -- override theme from `nimbu.yml`
- `--force` -- skip confirmation prompts (from global `--force`)

### Filter flags (push, sync, pull, copy)

Category filters restrict operations to subsets of resources. Multiple filters combine (OR logic across asset categories). **On push/sync, category filters bypass git detection** and push all matching files unless `--since` is also set, in which case they filter the git diff results.

- `--liquid-only` -- layouts, templates, snippets only (no assets)
- `--css-only` -- assets matching `*.css, *.scss, *.sass, *.less`
- `--js-only` -- assets matching `*.js, *.mjs, *.cjs, *.jsx, *.ts, *.tsx`
- `--images-only` -- assets matching `*.png, *.jpg, *.jpeg, *.gif, *.webp, *.svg, *.avif, *.ico`
- `--fonts-only` -- assets matching `*.woff, *.woff2, *.ttf, *.otf, *.eot`

Pull only supports `--liquid-only`. Copy supports `--liquid-only`.

### Sync-only flags

- `--prune` -- delete remote files that are missing locally (only files within managed scope)

## Build step

Configure `sync.build` in `nimbu.yml` to run a build before push/sync:

```yaml
sync:
  build:
    command: npm
    args: [run, build]
    cwd: frontend     # relative to project root
    env:
      NODE_ENV: production
```

Triggered only when `--build` is passed. Errors abort the operation.

## Ignore and generated patterns

```yaml
sync:
  ignore: [node_modules/**, "*.map"]
  generated: [dist/js/**, dist/css/**]
```

`ignore` excludes files from collection. `generated` marks files that are always included in push/sync even without git changes (overrides the defaults listed above).

## Theme diff and copy

`nimbu themes diff` -- compares local liquid files against remote, outputs status per file (no asset comparison).

`nimbu themes copy --from <site>[/<theme>] --to <site>[/<theme>]` -- copies theme content between sites. Theme defaults to `default-theme` when omitted. Supports `--from-host` / `--to-host` for cross-environment copies. Supports `--liquid-only`.

## CDN root

`nimbu themes cdn-root` -- prints the resolved CDN root URL for the configured theme. Useful for build tools that need the asset base URL.

## CRUD subcommands

Each resource kind has `list`, `get`, `create`, `delete` subcommands:
- `nimbu themes layouts {list,get,create,delete}`
- `nimbu themes templates {list,get,create,delete}`
- `nimbu themes snippets {list,get,create,delete}`
- `nimbu themes assets {list,get,create,delete}`
- `nimbu themes files {list,get,put,delete}` -- low-level theme file API (uses `put` instead of `create`)

## Local dev server

`nimbu server` starts a proxy + child dev server. The proxy intercepts requests, renders Nimbu Liquid templates for matching routes, and forwards everything else to the child server.

### nimbu.yml dev configuration

```yaml
dev:
  server:
    command: npx          # required -- child executable
    args: [vite, --port, "3001"]
    cwd: frontend         # relative to project root
    ready_url: http://localhost:3001
    env:
      VITE_API: /api
  proxy:
    host: 127.0.0.1       # default
    port: 4568            # default
    template_root: .      # where to find liquid templates
    watch: true           # filesystem watcher for template changes
    watch_scan_interval: 3s
    max_body_mb: 64
  routes:
    include: []           # glob or "METHOD glob" patterns
    exclude: []           # requests matching exclude skip the proxy
```

### How the proxy works

1. Proxy listens on `host:port` (default `127.0.0.1:4568`)
2. Child dev server starts with `NIMBU_PROXY_URL`, `NIMBU_PROXY_HOST`, `NIMBU_PROXY_PORT`, and `NIMBU_SITE` injected into its environment
3. Proxy waits for child readiness via `ready_url` (timeout default: 60s)
4. Incoming requests are matched against route include/exclude rules
5. Matching requests are rendered server-side by the Nimbu API using local liquid templates
6. Non-matching requests are forwarded to the child dev server

CLI flags (`--cmd`, `--proxy-port`, `--no-watch`, etc.) override nimbu.yml values.

## Cloud code (apps)

### Configuration

`nimbu apps config` -- interactive wizard that adds an app entry to `nimbu.yml`:

```yaml
apps:
  - id: my-app-key       # API app key
    name: my_app          # local reference name
    dir: code             # local directory containing source files
    glob: "**/*.js"       # file pattern to push
    host: api.nimbu.io    # scoped to specific API host
    site: my-site         # scoped to specific site
```

Apps are filtered by active host and site -- only matching entries are visible.

### Push

`nimbu apps push [--app <name>] [--only <file>[,<file>...]]...` -- uploads cloud code files.

- Without `--app`, works if exactly one app is configured for the active host/site
- Files are topologically sorted by `require()`/`import`/`export` dependencies before upload (dependencies first)
- `--sync` deletes remote files missing locally (cannot combine with explicit file list)
- `--only` project-relative paths restrict the upload set; values can be comma-separated and the flag can be repeated

Example: `nimbu apps push --app storefront --only code/main.js,code/hooks.js --only code/jobs/*.js`

### Dependency ordering

The CLI parses `require("./path")`, `import ... from "./path"`, and `export ... from "./path"` to build a dependency graph. Files are topologically sorted so dependencies are pushed before dependents. Circular dependencies cause an error.
