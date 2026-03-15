# Site Migration Reference

## sites copy -- full site migration

`nimbu sites copy` is the mega-command that orchestrates a complete site migration.
It runs stages sequentially in this fixed order:

1. **Channels** -- schema definitions (always all)
2. **Uploads** -- media files, builds a media-mapping for later stages
3. **Channel Entries** -- only if `--entry-channels` is provided
4. **Customer Config** -- custom field definitions for customers
5. **Product Config** -- custom field definitions for products
6. **Roles** -- customer roles
7. **Products** -- including variants; uses media mapping
8. **Collections** -- uses media + product mapping
9. **Theme** -- active theme assets (skipped with warning on error)
10. **Pages** -- all pages (`*`); uses media mapping
11. **Menus** -- all menus; uses media mapping
12. **Blogs** -- all blogs + posts; uses media mapping
13. **Notifications** -- all notification templates; uses media mapping
14. **Redirects** -- URL redirects
15. **Translations** -- all translation keys; uses media mapping

The media mapping built in step 2 is passed to all subsequent stages that handle
rich content, so image/file references in entries, pages, blogs, etc. are
automatically rewritten to point at the target site's uploads.

### Flags

| Flag | Type | Required | Description |
|------|------|----------|-------------|
| `--from` | string | yes | Source site subdomain or ID |
| `--to` | string | yes | Target site subdomain or ID |
| `--from-host` | string | no | Source API host (cross-API migrations) |
| `--to-host` | string | no | Target API host (cross-API migrations) |
| `--entry-channels` | string | no | Comma-separated channel slugs whose entries to copy |
| `--recursive` | bool | no | Recursively follow referenced channel entries |
| `--only` | string | no | Comma-separated channel allowlist when using `--recursive` |
| `--upsert` | string | no | Comma-separated upsert fields for entry copy (e.g. `slug` or `channel:field`) |
| `--copy-customers` | bool | no | Copy related customers during entry copy |
| `--allow-errors` | bool | no | Continue past item-level validation errors |
| `--dry-run` | bool | no | Preview without writing to target |
| `--force` | bool | no | Root-level flag; skips confirmation prompts, forces theme overwrite |

### Key behaviors

- Without `--entry-channels`, channel entries are **skipped entirely** (stage reports "no channels specified").
- `--dry-run` gates writes globally; each stage respects it.
- `--force` is a root flag (`nimbu --force sites copy ...`), not a sites-copy flag.
- Theme copy resolves the **active theme** on both sites automatically. If either side has no theme, it logs a warning and continues.
- Uploads are copied first so all downstream stages can remap media references.

## Per-resource copy commands

Each resource type has its own standalone copy command for targeted operations.

### Ref format

- **Site-level commands** use `--from <site> --to <site>` where `<site>` is a subdomain or ID.
- **Channel-level commands** use `--from <site>/<channel> --to <site>/<channel>`. If a default site is configured, the site part can be omitted (just `<channel>`).
- **Theme copy** uses `--from <site>[/<theme>] --to <site>[/<theme>]`. Theme defaults to `default-theme` if omitted.

### Available commands

| Command | Ref format | Extra flags |
|---------|-----------|-------------|
| `channels copy` | site/channel or `--all` with site | `--all` copies all channels at once |
| `channels entries copy` | site/channel | `--recursive`, `--only`, `--upsert`, `--query`, `--where`, `--per-page`, `--copy-customers`, `--allow-errors`, `--dry-run` |
| `pages copy [fullpath]` | site | Positional arg: fullpath, `prefix*`, or `*` (default `*`) |
| `products copy` | site | `--allow-errors` |
| `collections copy` | site | `--allow-errors` |
| `customers copy` | site | `--query`, `--where`, `--per-page`, `--upsert` (default `email`), `--password-length` (default 12), `--allow-errors` |
| `roles copy` | site | -- |
| `menus copy [slug]` | site | Positional arg: slug or `*` (default `*`); prompts before overwrite unless `--force` |
| `blogs copy [handle]` | site | Positional arg: handle or `*` (default `*`) |
| `notifications copy [slug]` | site | Positional arg: slug or `*` (default `*`) |
| `redirects copy` | site | -- |
| `translations copy [query]` | site | Positional arg: key, `prefix*`, or `*`; `--since` (RFC3339 or relative like `1d`); `--dry-run` |
| `themes copy` | site[/theme] | `--liquid-only` copies only liquid resources |

All per-resource commands also accept `--from-host` / `--to-host` for cross-API use.

## Cross-API operations

Use `--from-host` and `--to-host` when source and target live on different API
endpoints (e.g. different Nimbu regions or a staging API vs production API).

The host value can be:
- A bare domain: `nimbu.io` -- normalized to `https://api.nimbu.io`
- An `api.` prefixed domain: `api.nimbu.io` -- normalized to `https://api.nimbu.io`
- A full URL: `https://api.nimbu.io` -- used as-is

Both `--from-host` and `--to-host` are independently optional. When omitted, the
CLI's configured API URL is used (from `--api-url` flag or config).

## Workflow: staging-to-production full site copy

```bash
# 1. Dry-run to preview what will be copied
nimbu sites copy \
  --from staging-site --to production-site \
  --entry-channels articles,events \
  --recursive --allow-errors \
  --dry-run

# 2. Execute the full copy
nimbu --force sites copy \
  --from staging-site --to production-site \
  --entry-channels articles,events \
  --recursive --allow-errors

# 3. Cross-API variant (staging API -> production API)
nimbu --force sites copy \
  --from staging-site --from-host api.staging.nimbu.io \
  --to production-site --to-host api.nimbu.io \
  --entry-channels articles,events \
  --recursive --allow-errors
```

## Gotchas

- `sites copy` does **not** copy customers by default. Add `--copy-customers` explicitly.
- `--entry-channels` requires channel **slugs**, not display names.
- `--upsert` accepts channel-scoped syntax: `channel:field` (e.g. `articles:slug,events:external_id`).
- The `--only` flag filters which channels are followed during `--recursive` expansion, not which root channels are copied.
- Per-resource copy commands do **not** build a media mapping -- only `sites copy` does. Running `pages copy` standalone will not remap upload URLs.
- All write commands require `--force` or interactive confirmation. In non-interactive (CI) contexts, always pass `--force` as a root flag.
