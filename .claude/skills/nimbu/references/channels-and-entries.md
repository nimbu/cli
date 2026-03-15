# Channels & Entries Reference

## Ref Syntax

Cross-site commands (`copy`, `diff`, `info`) use a `site/channel` ref format:

```
--from staging/blog --to production/blog
```

If only `channel` is given (no `/`), the default site is used.
For different API hosts, add `--from-host` / `--to-host` (bare domain or full URL; `api.` prefix is auto-added).

## Channel Commands

| Command | Syntax | Key flags | Notes |
|---------|--------|-----------|-------|
| `list` | `nimbu channels list` | `--all`, `--page`, `--per-page`, `--no-entry-count` | Entry counts fetched by default (6 parallel workers). Use `--no-entry-count` to skip. |
| `get` | `nimbu channels get <slug>` | | Returns schema, customizations, ACL, dependency graph. |
| `info` | `nimbu channels info <slug or site/channel>` | `--typescript` | Accepts cross-site ref. `--typescript` emits a TS interface. |
| `fields` | `nimbu channels fields <slug>` | | Schema introspection — see detailed section below. |
| `diff` | `nimbu channels diff --from <ref> --to <ref>` | `--from-host`, `--to-host` | Compares channel attrs + field schema. Reports added/removed/updated. |
| `copy` | `nimbu channels copy --from <ref> --to <ref>` | `--all`, `--from-host`, `--to-host` | Copies channel config (not entries). `--all` copies all channels from source site. Requires `--write` (not readonly). |

### Schema Discovery: `fields` vs `info` vs `get`

**`channels fields <slug> --json`** is the primary tool for understanding channel data structure. Returns an array of custom field definitions with:

| Property | Purpose |
|----------|---------|
| `name` | Field key — used in entry data and templates |
| `label` | Human-readable display name |
| `type` | Data type: `string`, `text`, `file`, `image`, `date`, `boolean`, `integer`, `float`, `select`, `belongs_to`, `has_many`, `geo`, etc. |
| `required` | Whether the field is mandatory |
| `unique` | Whether values must be unique across entries |
| `localized` | Whether the field has per-locale values |
| `encrypted` | Whether values are stored encrypted |
| `private_storage` | Whether file uploads use private storage |
| `reference` | For relationship fields (`belongs_to`/`has_many`): target channel slug |
| `select_options` | For `select` fields: available options with `id`, `name`, `slug` |
| `hint` | Field help text / description |
| `required_expression` | Conditional requirement expression |
| `calculated_expression` | Auto-calculated field formula |
| `geo_type` | For `geo` fields: geometry subtype |

**Agent workflow**: Always run `nimbu channels fields <channel> --json` when working on themes or templates to know the exact field names, types, and relationships available in channel entries.

### `channels get` vs `channels info`

- `get` works on the current site only (no cross-site ref). Shows ACL, ordering config, dependency graph.
- `info` accepts `site/channel` ref. Adds TypeScript generation (`--typescript`). Lighter dependency summary.

Use `get --json` for full schema introspection. Use `info --typescript` for codegen.

## Entry Commands

All entry commands take `<channel>` as the first positional arg (slug or ID).

| Command | Syntax | Key flags | Notes |
|---------|--------|-----------|-------|
| `list` | `nimbu channels entries list <channel>` | `--all`, `--page`, `--per-page` | Displays title fallback: `title` field > `fields.title` > slug > ID. |
| `get` | `nimbu channels entries get <channel> <entry>` | `--locale` (global) | Entry identified by ID or slug. |
| `create` | `nimbu channels entries create <channel> [assignments...]` | `--file` | Inline or `--file` (mutually exclusive). Requires write mode. |
| `update` | `nimbu channels entries update <channel> <entry> [assignments...]` | `--file` | Same input rules as create. Requires write mode. |
| `delete` | `nimbu channels entries delete <channel> <entry>` | `--force` (required) | Requires both `--force` and write mode. |
| `count` | `nimbu channels entries count <channel>` | `--locale` (global) | Returns integer count. |
| `copy` | `nimbu channels entries copy --from <ref> --to <ref>` | See table below | Most complex command. Requires write mode (unless `--dry-run`). |

### Entries Copy Flags

| Flag | Type | Purpose |
|------|------|---------|
| `--from` | `site/channel` | Source ref (required) |
| `--to` | `site/channel` | Target ref (required) |
| `--from-host` | string | Source API host override |
| `--to-host` | string | Target API host override |
| `--recursive` | bool | Follow and copy referenced channel entries |
| `--only` | CSV | Channel allowlist when using `--recursive` (e.g. `authors,tags`) |
| `--query` | string | Raw query string appended to source entry list request |
| `--where` | string | Where expression for source entry filtering |
| `--per-page` | int | Page size for source fetching |
| `--upsert` | CSV | Match fields for upsert instead of create. Supports `channel:field` scoping (e.g. `slug` or `blog:slug,tags:name`). |
| `--copy-customers` | bool | Also copy related customers (owner/customer fields) |
| `--allow-errors` | bool | Continue on per-item validation errors instead of aborting |
| `--dry-run` | bool | Report planned selection without writing |

## Gotchas

1. **Write guard**: `create`, `update`, `delete`, `copy` all check `requireWrite`. If `--readonly` is set or `NIMBU_READONLY=1`, they fail immediately. Delete additionally requires `--force`.

2. **`--dry-run` skips write guard**: On entries copy, `--dry-run` bypasses the write check. Safe for read-only agents to plan copies.

3. **Channel copy copies config, not entries**: `channels copy` syncs the channel definition (fields, settings). To copy data, use `channels entries copy`.

4. **Recursive copy depth**: `--recursive` follows `reference` fields to other channels. Without `--only`, it copies all referenced channels. This can cascade widely -- always scope with `--only` or use `--dry-run` first.

5. **Upsert scoping**: Bare `--upsert slug` applies to all channels in a recursive copy. Use `channel:field` syntax (e.g. `blog:slug,authors:email`) to scope per channel.

6. **Entry title fallback**: List display picks title from: `entry.Title` > `entry.Fields["title"]` > slug > ID. The API `title` field may be empty if the channel uses a custom title field.

7. **Count uses separate endpoint**: `entries count` hits `/channels/{slug}/entries/count`, not a paginated list. It respects `--locale`.

## Examples

```bash
# List all channels with entry counts
nimbu channels list --all --json

# Inspect channel schema
nimbu channels get blog --json

# Generate TypeScript interface from a remote site
nimbu channels info staging/blog --typescript

# Diff channel config between environments
nimbu channels diff --from staging/blog --to production/blog --json

# Copy channel definition (not entries) between sites
nimbu channels copy --from staging/blog --to production/blog

# Copy all channel definitions
nimbu channels copy --all --from staging --to production

# List entries with pagination
nimbu channels entries list blog --all --json

# Create entry inline
nimbu channels entries create blog title="Hello World" fields.teaser="First post"

# Create entry from file
nimbu channels entries create blog --file entry.json

# Update entry
nimbu channels entries update blog hello-world title="Updated Title"

# Delete entry (requires --force)
nimbu channels entries delete blog hello-world --force

# Copy entries between sites with upsert on slug
nimbu channels entries copy --from staging/blog --to production/blog --upsert slug --json

# Recursive copy with channel allowlist
nimbu channels entries copy --from staging/blog --to production/blog \
  --recursive --only authors,tags --upsert slug

# Dry-run to preview what would be copied
nimbu channels entries copy --from staging/blog --to production/blog --dry-run --json

# Filter source entries during copy
nimbu channels entries copy --from staging/blog --to production/blog \
  --where "published=true" --upsert slug
```
