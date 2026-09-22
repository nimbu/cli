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
| `get` | `nimbu channels get --channel <slug>` | | Returns schema, customizations, ACL, dependency graph. |
| `create` | `nimbu channels create --file <ch.json>` or `nimbu channels create name=… slug=…` | `--file` | Creates a channel. `--file` (full JSON incl. `customizations`) XOR inline assignments. Use `customizations:=@fields.json` for the field array inline. Requires write mode. |
| `delete` | `nimbu channels delete --channel <slug>` | `--force` (required) | Deletes the whole channel definition (not just entries). Requires both `--force` and write mode. |
| `info` | `nimbu channels info --channel <slug or site/channel>` | `--typescript` | Accepts cross-site ref. `--typescript` emits a TS interface. |
| `fields` | `nimbu channels fields list --channel <slug>` | | Schema introspection — see detailed section below. |
| `diff` | `nimbu channels diff --from <ref> --to <ref>` | `--from-host`, `--to-host` | Compares channel attrs + field schema. Reports added/removed/updated. |
| `copy` | `nimbu channels copy --from <ref> --to <ref>` | `--all`, `--from-host`, `--to-host` | Copies channel config (not entries). `--all` copies all channels from source site. Fails under `--readonly`. |

### Schema Discovery: `fields` vs `info` vs `get`

**`channels fields list --channel <slug> --json`** is the primary tool for understanding channel data structure. Returns an array of custom field definitions with:

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

**Agent workflow**: Always run `nimbu channels fields list --channel <channel> --json` when working on themes or templates to know the exact field names, types, and relationships available in channel entries.

### `channels get` vs `channels info`

- `get` works on the current site only (no cross-site ref). Shows ACL, ordering config, dependency graph.
- `info` accepts `site/channel` ref. Adds TypeScript generation (`--typescript`). Lighter dependency summary.

Use `get --json` for full schema introspection. Use `info --typescript` for codegen.

## Entry Commands

All entry commands take the channel slug or ID via `--channel`.

| Command | Syntax | Key flags | Notes |
|---------|--------|-----------|-------|
| `list` | `nimbu channels entries list --channel <channel>` | `--all`, `--page`, `--per-page` | Displays title fallback: `title` field > `fields.title` > slug > ID. |
| `get` | `nimbu channels entries get --channel <channel> --entry <entry>` | `--locale` | Entry identified by ID or slug. JSON is the full per-locale API body. |
| `create` | `nimbu channels entries create --channel <channel> [assignments...]` | `--file` | Inline or `--file` (mutually exclusive). Requires write mode. |
| `update` | `nimbu channels entries update --channel <channel> --entry <entry> [assignments...]` | `--file`, `--locale` | Same input rules as create. Sends only assigned fields. Requires write mode. |
| `delete` | `nimbu channels entries delete --channel <channel> --entry <entry>` | `--force` (required) | Requires both `--force` and write mode. |
| `count` | `nimbu channels entries count --channel <channel>` | `--locale` (global) | Returns integer count. |
| `copy` | `nimbu channels entries copy --from <ref> --to <ref>` | See table below | Most complex command. Requires write mode (unless `--dry-run`). |
| `watch` | `nimbu channels entries watch --channel <channel>` | `--filters`, `--where`, `--once`, `--for`, `--host`, `--insecure` | Live websocket stream of entry changes. Read-only. See below. |

### Locales

Entries have no `translations` map. `--locale` is sent as `content_locale`;
top-level values are for that locale only. Read each locale with a separate
`entries get --locale`. `get --json` keeps the full raw body.

`entries update --locale en title=Hallo` sends only the assigned fields (no
full-document echo) plus `content_locale=en`. Localized fields write to that
locale and leave the default-locale value intact. Non-localized fields have one
stored value, so `--locale` cannot apply to them — updating them changes the
default document.

```bash
nimbu channels entries get --channel blog --entry welcome --locale en --json
nimbu channels entries update --channel blog --entry welcome --locale en title="Hello"
```

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

## Live Queries (`entries watch`)

`nimbu channels entries watch --channel <channel>` subscribes to a live query
over a websocket and prints entry changes as they happen. It mints a
single-use, two-minute realtime grant behind the scenes and never prints it.
`nimbu realtime grant` returns a grant for custom clients (single use, treat as
a secret; the CLI warns on stderr).

```bash
nimbu channels entries watch --channel blog --site my-site --json
nimbu channels entries watch --channel blog --filters status=published --once --json
nimbu channels entries watch --channel blog --where "published=true" --for 2m
nimbu channels entries watch --channel blog --host localhost:3000 --insecure
```

| Flag | Purpose |
|------|---------|
| `--channel` | Channel ID or slug (required; the live query is always scoped to it) |
| `--filters` | `key=value` / `key.op=value`, repeatable, same grammar as `entries list` |
| `--where` | Where expression; equivalent to `--filters where=...`. Kong splits `--filters` on commas, so any value containing a comma belongs in `--where` |
| `--once` | Exit after the first event (a `resync` does not count) |
| `--for` | Stop watching after a duration, e.g. `30s`. The global `--timeout` stays the HTTP request timeout |
| `--host` | Override the websocket host (dev: `localhost:3000`) |
| `--insecure` | Skip TLS verification for the socket (dev only) |

### What a live query cannot do

Validated locally before the socket opens; each rejection exits 2:

- pagination, sort, projection, search: `limit`, `page`, `per_page`, `skip`, `sort`, `fields`, `only`, `search`, `include_slugs`, `resolve`, `explain`, `direction`, `content_locale`, and the other reserved base keys
- `regex` (use `contains`, `start`, `end`); `near`, `geoWithin`, `geoIntersects`, `maxDistance`, `minDistance`
- `_acl` / `_owner` filters, `inverse_of_*` relations
- negated operators (`ne`, `nin`, `not_contains`) on a dotted sub-field path such as `color.title.ne` — use the positive form

Everything else passes through: `_status`, `slug`, `in`, `nin`, `all`,
`contains`, `not_contains`, `exists`, `start`, `end`, `matches`, `gt`, `gte`,
`lt`, `lte`, `ne`, and `where` with `and`/`or`/`not`. Limits: 20 keys, key
<= 100 chars, value <= 1000 chars.

### `--json` event envelope

One compact JSON object per line on stdout, exactly as the server sent it.

| Field | Type | Notes |
|-------|------|-------|
| `event_id` | string | Unique per delivery; duplicates are dropped client-side |
| `event` | string | `added`, `changed`, `removed`, `resync` |
| `resource` | string | Always `channel_entries` |
| `parent_id` | string | The channel subscribed to |
| `id` | string | Entry ID; may be absent on `resync` |
| `type` | string | `channel_entries.created` / `.updated` / `.deleted` |
| `object` | object | Full entry body; absent on `removed` |
| `changeset` | object | Changed fields only, when sent |
| `occurred_at` | string | ISO8601 |
| `revision` | number | Monotonic per entry |

Status, reconnect and control lines always go to stderr (controls only under
`--verbose`), so `--json 2>/dev/null | jq -c` stays clean.

### Delivery guarantees

Delivery is at-least-once and the socket reconnects on its own with a fresh
grant. A `resync` event means changes may have been missed: `watch` does **not**
re-read the channel, it prints the `channels entries list` command to run.

Exit codes: 0 on Ctrl-C, `--for` expiry and `--once`; 2 on a rejected query
(local or server `invalid_query`); 1 when the connection cannot be
re-established within the retry budget; grant failures use the normal auth
codes (3, 4, 7).

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
nimbu channels get --channel blog --json

# Create a channel from a full JSON file (name, slug, title_field, customizations[])
nimbu channels create --file testimonials.json --json

# Create a channel from inline assignments (customizations as typed JSON)
nimbu channels create name=Testimonials slug=testimonials title_field=author \
  customizations:=@fields.json --json

# Delete a whole channel definition (requires --force)
nimbu channels delete --channel testimonials --force

# Generate TypeScript interface from a remote site
nimbu channels info --channel staging/blog --typescript

# Diff channel config between environments
nimbu channels diff --from staging/blog --to production/blog --json

# Copy channel definition (not entries) between sites
nimbu channels copy --from staging/blog --to production/blog

# Copy all channel definitions
nimbu channels copy --all --from staging --to production

# List entries with pagination
nimbu channels entries list --channel blog --all --json

# Create entry inline
nimbu channels entries create --channel blog title="Hello World" fields.teaser="First post"

# Create entry from file
nimbu channels entries create --channel blog --file entry.json

# Update entry
nimbu channels entries update --channel blog --entry hello-world title="Updated Title"

# Delete entry (requires --force)
nimbu channels entries delete --channel blog --entry hello-world --force

# Copy entries between sites with upsert on slug
nimbu channels entries copy --from staging/blog --to production/blog --upsert slug --json

# Recursive copy with channel allowlist
nimbu channels entries copy --from staging/blog --to production/blog \
  --recursive --only authors,tags --upsert slug

# Dry-run to preview what would be copied
nimbu channels entries copy --from staging/blog --to production/blog --dry-run --json

# Watch a channel for 30 seconds, JSON lines
nimbu channels entries watch --channel blog --site my-site --json --for 30s

# Filter source entries during copy
nimbu channels entries copy --from staging/blog --to production/blog \
  --where "published=true" --upsert slug
```
