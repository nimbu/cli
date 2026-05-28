---
name: nimbu-themes
description: >
  Nimbu Liquid themes — the templating layer of a Nimbu site (`layouts/`,
  `templates/`, `snippets/`, `stylesheets/`, `javascripts/`). Use when
  authoring or modifying `.liquid` files in a Nimbu theme; when the code
  uses Nimbu-specific Liquid tags like `{% form %}`, `{% editable_* %}`,
  `{% translate %}`, `{% nav %}`, `{% scope %}`, `{% paginate %}`, `{% snippet %}`,
  `{% repeatable %}`; or when working with Nimbu drops like `site`, `customer`,
  `cart`, `channels`, `products`, `page`, `menus`. For deploying themes
  (`nimbu themes push/sync`), use the companion `nimbu` skill. For server-side
  JavaScript in `code/`, use `nimbu-cloud-code`.
metadata:
  version: "0.2.0"
---

# Nimbu Themes

Nimbu themes are written in **Liquid** (Shopify's templating language) plus a Nimbu-specific layer of tags, filters, and drops. A theme is a directory of `.liquid` files plus assets that ships to a Nimbu site via `nimbu themes push` and renders pages, blogs, channel collections, customer accounts, and the storefront.

## Docs are the source of truth

**Always fetch the relevant doc page before writing or modifying a theme.** This skill summarizes shape, conventions, and gotchas; the docs at `docs.nimbu.io/docs/themes/` are authoritative and change more often than this skill.

**Use the `/docs/... .md` variant of every URL.** Every Nimbu doc page has a sibling markdown file at the same path. WebFetch returns these as plain markdown — far cheaper and more accurate to parse than rendered HTML.

```
docs.nimbu.io/docs/themes/concepts/templates   →   docs.nimbu.io/docs/themes/concepts/templates.md
docs.nimbu.io/docs/themes/filters-tags         →   docs.nimbu.io/docs/themes/filters-tags.md
```

### Themes docs

| Topic | URL |
|-------|-----|
| Overview | https://docs.nimbu.io/docs/themes/introduction/overview.md |
| Getting started | https://docs.nimbu.io/docs/themes/introduction/getting-started.md |
| Layouts | https://docs.nimbu.io/docs/themes/concepts/layouts.md |
| Templates | https://docs.nimbu.io/docs/themes/concepts/templates.md |
| Snippets | https://docs.nimbu.io/docs/themes/concepts/snippets.md |
| Editables | https://docs.nimbu.io/docs/themes/concepts/editables.md |
| Pages | https://docs.nimbu.io/docs/themes/content/pages.md |
| Navigation | https://docs.nimbu.io/docs/themes/content/navigation.md |
| Channels (custom collections) | https://docs.nimbu.io/docs/themes/content/channels.md |
| Webshop | https://docs.nimbu.io/docs/themes/content/webshop.md |
| Gift cards | https://docs.nimbu.io/docs/themes/content/gift-cards.md |
| Multilingual & i18n | https://docs.nimbu.io/docs/themes/other/multilingual.md |
| Forms | https://docs.nimbu.io/docs/themes/other/forms.md |
| Performance & caching | https://docs.nimbu.io/docs/themes/other/performance.md |
| Theme config | https://docs.nimbu.io/docs/themes/other/theme-config.md |
| Global variables | https://docs.nimbu.io/docs/themes/other/global-variables.md |
| Advanced | https://docs.nimbu.io/docs/themes/other/advanced.md |
| Forms & editable content (combined) | https://docs.nimbu.io/docs/themes/forms-editable-content.md |
| Filters & tags reference | https://docs.nimbu.io/docs/themes/filters-tags.md |
| Liquid context (drops) | https://docs.nimbu.io/docs/themes/liquid-context.md |

### Filter reference (by purpose)

| Topic | URL |
|-------|-----|
| Text formatting | https://docs.nimbu.io/docs/themes/filters/text-formatting.md |
| Numbers & money | https://docs.nimbu.io/docs/themes/filters/numbers-money.md |
| Dates & time | https://docs.nimbu.io/docs/themes/filters/dates-time.md |
| Assets & CDN | https://docs.nimbu.io/docs/themes/filters/assets-cdn.md |
| Commerce | https://docs.nimbu.io/docs/themes/filters/commerce.md |
| Arrays & collections | https://docs.nimbu.io/docs/themes/filters/arrays-collections.md |
| API & JSON | https://docs.nimbu.io/docs/themes/filters/api-json.md |
| Special-purpose (QR, hashing, …) | https://docs.nimbu.io/docs/themes/filters/special-purpose.md |

### Standard Liquid (Shopify)

Nimbu's Liquid is a superset of Shopify's. For the **standard** tags (`{% if %}`, `{% for %}`, `{% assign %}`, `forloop`, etc.) and standard filters (`upcase`, `downcase`, `truncate`, `date`, …), use Shopify's reference:

| Resource | URL |
|----------|-----|
| Shopify Liquid reference | https://shopify.dev/docs/api/liquid |
| Shopify Liquid markdown | https://shopify.dev/docs/api/liquid.md |
| Shopify llms.txt index | https://shopify.dev/llms.txt |
| Shopify-published Liquid skills plugin | https://github.com/Shopify/liquid-skills |

Shopify's `liquid-skills` plugin is a useful companion for richer baseline Liquid tooling. Install it alongside this skill if you want LSP, accessibility, and theme-standards coverage out of the box.

## What a Nimbu theme contains

| Folder | Purpose | Notes |
|--------|---------|-------|
| `layouts/` | Outer HTML wrappers (e.g. `default.liquid`) | A template selects its layout via `{% layout 'name' %}` |
| `templates/` | Per-page-type templates (`page.liquid`, `blog.liquid`, `channel-x.liquid`) | One per content type or specialized URL |
| `snippets/` | Reusable Liquid partials | Render via `{% include 'name' %}` (canonical; `{% snippet %}` exists but is rarely used) |
| `stylesheets/` | Compiled CSS (output of build) | Reference via `{{ 'app.css' | stylesheet_tag }}` |
| `javascripts/` | Compiled JS (output of build) | Reference via `{{ 'app.js' | javascript_tag }}` |
| `images/`, `fonts/` | Static theme assets | Reference via `{{ 'logo.svg' | theme_image_url }}` |
| `nimbu.yml` | Site identifier + cloud-code app registry | Read by the CLI; checked into git |
| `code/` | Cloud Code (server-side JS) — **not** part of the theme | Deployed separately via `nimbu apps push`; see `nimbu-cloud-code` |

For full directory conventions, build pipeline patterns, and `nimbu.yml` shape, see [theme-structure.md](references/theme-structure.md).

## The four building blocks

| Block | What it is | How to render |
|-------|-----------|---------------|
| **Layout** | The page's outer HTML — `<html>`, `<head>`, `<body>`, header/footer | A template either inherits `default.liquid` or declares `{% layout 'name' %}` |
| **Template** | Per-content-type body (`page.liquid`, `blog.liquid`, `articles/show.liquid`) | Selected automatically by URL/content type |
| **Snippet** | Reusable partial | `{% include 'navigation/header' %}` (pass params with `{% include 'card', product: p %}`) |
| **Editable** | Author-editable region inside a layout/template | `{% editable_text 'hero_body' %}<p>…</p>{% endeditable_text %}` |

Pick the smallest block that fits: editables for "the editor should be able to change this," snippets for repeated markup, templates for whole content types, layouts only for chrome.

## Liquid context: the drops you'll meet

These are populated automatically and available in every Liquid file:

| Drop | Represents |
|------|-----------|
| `site` | The current site (settings, channels, products, locales) |
| `page` | The current page object (title, content, og_image, translations, …) |
| `customer` | The logged-in customer (`null` when anonymous) |
| `cart` | The current open cart/order, if any |
| `channels` | Collections by slug — `channels.articles`, `channels.events`, etc. |
| `products`, `collections`, `product_types`, `product_vendors` | Webshop catalogue |
| `menus` | Navigation trees by slug — used by `{% nav %}` |
| `blogs` | Blog and article aggregation |
| `params` | Sanitized request parameters |
| `path`, `url` | Request path utilities (`url.current`, `url.current_path`) |
| `locale`, `default_locale`, `locale_url_prefix` | Active language |
| `seo` | Site-level SEO defaults (`description`, `keywords`) |
| `auth_token` | CSRF token (used automatically by `{% form %}`) |
| `flash` | Flash messages from form submissions |
| `config` | Per-site configuration (locales, countries, shipping methods, …) |
| `now`, `today` | UTC timestamp / site-local date at render time |
| `template`, `template_name` | Active template identifiers |
| `theme_version` | Useful for cache busting |

Full property lists and examples: [liquid-cheatsheet.md § Drops](references/liquid-cheatsheet.md).

## Tags you should know

Nimbu adds a large number of tags on top of standard Liquid. The most-used:

| Tag | Use |
|-----|-----|
| `{% layout 'name' %}` | Pick a layout from a template |
| `{% include 'snippet' %}` / `{% include 'snippet', x: y %}` | Render a snippet (with optional params) |
| `{% paginate collection.items by 20 %}…{% endpaginate %}` | Paginate a list |
| `{% scope %}` / `{% with_scope %}` / `{% sort %}` / `{% condition %}` | Server-side query builder over channels/products/collections |
| `{% nav 'menu_slug' %}` | Render a menu from `menus` |
| `{% breadcrumbs %}` | Page-hierarchy breadcrumbs |
| `{% tree %}` | Render a page tree |
| `{% search %}` | Run a configured search query |
| `{% translate 'key', default: 'Fallback' %}` | i18n string with fallback |
| `{% localized_path 'nl' %}` | Resolve the current page's URL for another locale |
| `{% set var, key, value %}` | Build a hash dynamically |
| `{% cache key %}…{% endcache %}` | Cache a fragment |
| `{% form channels.contact %}…{% endform %}` | A bound form with CSRF + validation |
| `{% editable_text/_field/_file/_select/_switch/_reference %}` | Author-editable regions |
| `{% repeatable %}…{% endrepeatable %}` | Add/reorder/remove blocks of content |
| `{% editable_group %}` / `{% editable_canvas %}` | Group editables / declare canvas zones |

For the full list, with shapes and examples, see [liquid-cheatsheet.md § Tags](references/liquid-cheatsheet.md).
For form helpers (`input`, `text_area`, `submit_tag`, …) and the full editable family, see [forms-and-editables.md](references/forms-and-editables.md).
For i18n patterns (`{% translate %}`, `localized_date`, language switching), see [i18n-and-multilingual.md](references/i18n-and-multilingual.md).

## Filters worth memorizing

A handful of Nimbu-specific filters come up in nearly every theme:

| Filter | Purpose |
|--------|---------|
| `theme_image_url`, `asset_url` | Resolve theme assets to CDN URLs |
| `stylesheet_tag`, `javascript_tag` | Build `<link>` / `<script>` tags |
| `filter`, `grayscale`, `sepia`, `vignette` | On-the-fly image transforms (resize, crop, effects) |
| `localized_date` | Locale-aware date formatting (`"%d %b %Y"` etc.) |
| `time_ago_in_words` | "3 minutes ago" |
| `money_with_currency`, `money_without_currency`, `number_to_currency` | Money formatting |
| `markdown`, `textile`, `strip_html` | Body-copy rendering |
| `parameterize`, `transliterate` | Slug-friendly strings |
| `json`, `from_json` | (De)serialize for inline JS or API payloads |
| `add_query_params` | Build URLs with extra params |

Plus `where`, `where_exp`, `find`, `find_exp`, `group_by`, `group_by_exp`, `intersection`, `union`, `push`, `pop`, `shift`, `unshift` for array work — same names as Jekyll/Liquid extensions, so they look familiar.

## Project structure at a glance

```
my-theme/
├── layouts/
│   └── default.liquid
├── templates/
│   ├── page.liquid
│   ├── blog.liquid
│   ├── article.liquid
│   └── …                  # one per content type
├── snippets/
│   ├── header.liquid
│   ├── footer.liquid
│   └── …                  # subdirectories OK: navigation/, forms/, cards/
├── stylesheets/app.css
├── javascripts/app.js
├── images/, fonts/
├── src/                   # source TS/SCSS — compiled to javascripts/ + stylesheets/
├── nimbu.yml              # site id + cloud-code apps
└── code/                  # cloud code — separate skill, not part of the theme
```

Cloud code in `code/` is **not** shipped by `themes push/sync` — it's a separate concern handled by `nimbu apps push`. See the `nimbu-cloud-code` skill.

For the full directory contract and asset pipeline, see [theme-structure.md](references/theme-structure.md).

## Local development & deployment

Use the companion `nimbu` skill for the CLI:

```bash
nimbu themes serve                    # local preview
nimbu themes push                     # deploy theme
nimbu themes sync                     # bidirectional sync (rare)
nimbu themes diff                     # show what would change
```

Deployment specifics live in the `nimbu` skill — this skill focuses on authoring.

## Common gotchas

1. **Use `{% include %}`, not `{% snippet %}`**: both are documented, but real Nimbu themes overwhelmingly use `{% include 'name', param: value %}`. `{% snippet %}` is a rarely-used alias.
2. **Pagination iterates over `paginate.collection`, not the original drop**. `{% paginate channels.articles by 12 %}` then `{% for article in paginate.collection %}` — iterating `channels.articles` directly inside the block gives you all entries unpaginated.
3. **Forms need `{% form %}`** for CSRF + validation + flash. Hand-rolled `<form>` tags will not get an `auth_token` injected automatically.
4. **`customer` is `null` when anonymous** — gate with `{% if customer %}…{% endif %}` before reading properties.
5. **Editables vs hard-coded copy**: if the marketing team needs to edit it, wrap it in `{% editable_text %}` / `{% editable_field %}` from the start. Retrofitting is painful because the Liquid signature is what registers the editable in the page editor.
6. **Don't reference assets with raw `/stylesheets/app.css`** — use `{{ 'app.css' | stylesheet_tag }}` or `{{ 'app.css' | asset_url }}` so caching/CDN/cache-busting work.
7. **`{% scope %}` / `{% sort %}` only apply to server-side query drops** (`channels.x`, `products`, `collections`). They don't work on arbitrary arrays — for those use the `where` / `sort` filters.
8. **`{% translate 'key' %}` always wants a `default:`** — without it, missing keys render as the literal key, which leaks into production.
9. **Don't mix layout selection mechanisms**: either let Nimbu pick `default.liquid` automatically, or declare `{% layout 'x' %}` at the top of the template. Mixing causes confusion when someone moves a template.
10. **Channel slugs in Liquid are dynamic**: prefer `{{ entry.url }}` over hard-coded paths; if the slug changes server-side, hard-coded paths silently 404. (`entry._url` works as an alias.)
11. **Cloud Code in `code/` does not ship via `themes push`** — push themes and apps separately. See the `nimbu` and `nimbu-cloud-code` skills.

## See also

- **Companion skills in this plugin:**
  - `nimbu` — CLI for `themes push/sync`, `apps push`, channel schemas, env vars
  - `nimbu-cloud-code` — server-side JS in `code/`, the `Nimbu` SDK, callbacks, jobs, functions
- **Reference files in this skill:**
  - [liquid-cheatsheet.md](references/liquid-cheatsheet.md) — full tag/filter/drop inventory
  - [theme-structure.md](references/theme-structure.md) — directory layout, asset pipeline, `nimbu.yml`
  - [forms-and-editables.md](references/forms-and-editables.md) — `{% form %}`, helpers, full `editable_*` family, `repeatable`
  - [i18n-and-multilingual.md](references/i18n-and-multilingual.md) — `{% translate %}`, locale switching, `localized_date`
- **External:**
  - [Nimbu Themes overview](https://docs.nimbu.io/docs/themes/introduction/overview.md)
  - [Filters & tags reference](https://docs.nimbu.io/docs/themes/filters-tags.md)
  - [Shopify Liquid reference](https://shopify.dev/docs/api/liquid) — for standard syntax not specific to Nimbu
