# Theme Structure

How a Nimbu theme is organized on disk. Conventions, not rules — but agents should follow what's already in place rather than inventing new structures.

## Directory layout

```
my-theme/
├── layouts/                 # outer HTML wrappers
│   ├── default.liquid
│   ├── article.liquid       # specialized layouts (optional)
│   └── home.liquid
├── templates/               # per-content-type bodies
│   ├── page.liquid
│   ├── blog.liquid
│   ├── article.liquid
│   ├── search.liquid
│   ├── account/             # subdirectories OK
│   │   ├── login.liquid
│   │   └── orders.liquid
│   └── channels/            # one per channel slug
│       ├── events.liquid
│       └── event.liquid
├── snippets/                # reusable partials
│   ├── header.liquid
│   ├── footer.liquid
│   ├── navigation/
│   │   └── main.liquid
│   ├── forms/
│   │   └── contact.liquid
│   └── cards/
│       └── article.liquid
├── stylesheets/             # compiled CSS (output)
│   └── app.css
├── javascripts/             # compiled JS (output)
│   └── app.js
├── images/                  # static theme images
├── fonts/                   # static fonts
├── src/                     # source TS/SCSS — compiled to javascripts/ + stylesheets/
│   ├── app.ts
│   ├── styles/
│   └── components/
├── nimbu.yml                # site identifier + cloud-code apps
├── package.json             # dev tooling: webpack/vite/esbuild, eslint, prettier
└── code/                    # cloud code — separate concern, not part of theme
```

`code/` deploys via `nimbu apps push`; it's owned by the [`nimbu-cloud-code`](../../nimbu-cloud-code/SKILL.md) skill.

## The four building blocks

### Layouts

Files in `layouts/` define the outer HTML — `<html>`, `<head>`, `<body>`, header, footer, scripts, analytics. A typical site has one (`default.liquid`) plus a handful of specialized layouts (e.g. `article.liquid`, `landingpage.liquid`).

A template either inherits `default.liquid` automatically or declares the layout explicitly:

```liquid
{% layout 'article' %}
```

Every layout must include two markers:

- `{{ content_for_header }}` inside `<head>` — Nimbu injects analytics, meta, CSRF, and other head content here.
- `{{ content_for_body }}` where the template's body slots in (typically inside `<main>`).

```liquid
<!doctype html>
<html lang="{{ locale }}">
  <head>
    {{ content_for_header }}
    <title>{{ page.title }}</title>
  </head>
  <body>
    <main>{{ content_for_body }}</main>
  </body>
</html>
```

### Templates

Files in `templates/` render a content type. Naming is conventional:

| File | Renders |
|------|---------|
| `page.liquid` | A standard CMS page |
| `blog.liquid` | A blog index (list of articles) |
| `article.liquid` | A single article |
| `search.liquid` | Search results |
| `404.liquid`, `500.liquid` | Error pages |
| `templates/<channel-slug>.liquid` | A channel collection (list view) |
| `templates/<channel-slug>/show.liquid` | A single channel entry |
| `templates/account/<x>.liquid` | Customer-account pages |

Templates focus on the body — chrome belongs in the layout, repeated markup in snippets, editable text in editables.

### Snippets

Files in `snippets/` are reusable partials. Render with:

```liquid
{% include 'header' %}
{% include 'cards/article' %}              {# subdirectory path, no extension #}
{% include 'cards/article', article: a %}  {# pass named params #}
```

`{% snippet 'name', … %}` is a documented alias but rarely used in real themes. Stick with `{% include %}`.

Convention: organize snippets in subdirectories by domain (`navigation/`, `forms/`, `cards/`, `account/`). Avoid leading-underscore filenames (`_header.liquid`) — pick one style and be consistent within the project.

### Editables

Editable regions register a Liquid tag that the page editor reads. They are stored in templates and snippets, not in dedicated files. See [forms-and-editables.md](forms-and-editables.md).

## `nimbu.yml`

Minimal shape:

```yaml
theme: default-theme           # theme directory name (or 'default')
site:  my-site-slug            # site identifier
apps:                          # cloud-code apps for this site (optional)
  - name: production
    id:   <app-uuid>
    dir:  code/main/dist       # where compiled cloud code lives
    glob: '*.js'
```

Multi-environment setups often check in just one base file and override per env via CLI flags or env-specific YAML files (`nimbu.staging.yml`). Confirm against the `nimbu` skill — that's the CLI side.

## Asset pipeline

Most modern themes compile from `src/` to `stylesheets/` + `javascripts/`. Common stacks:

| Tool | Notes |
|------|-------|
| **Webpack 5** | Most common; output to `stylesheets/app.css` + `javascripts/app.js` |
| **Vite / esbuild** | Newer projects; same output convention |
| **Compass / SCSS** | Older themes pre-dating Webpack |

Typical `package.json` scripts (varies per project):

```json
{
  "scripts": {
    "start":            "nimbu themes serve",
    "build":            "webpack --mode development --watch",
    "build:production": "NIMBU_ENV=production webpack --mode production",
    "release:production": "yarn build:production && nimbu themes push"
  }
}
```

### Referencing assets from Liquid

Always go through the Nimbu filters — they resolve to the CDN, set correct `Cache-Control`, and respect environment overrides:

```liquid
<link rel="stylesheet" href="{{ 'app.css' | asset_url }}">
{{ 'app.css' | stylesheet_tag, media: 'screen' }}
{{ 'app.js'  | javascript_tag, defer: true }}

<img src="{{ 'logo.svg' | theme_image_url }}" alt="Logo">
```

Avoid hard-coded paths like `<link href="/stylesheets/app.css">`. They skip the CDN and break cache busting.

### Cache busting

Either rely on the build's content-hash filename, or scope a fragment with `theme_version`:

```liquid
{% cache 'main-nav', theme_version %}
  {% nav 'main' %}
{% endcache %}
```

## Channel templates

When a channel (Nimbu's term for a custom content collection — analogous to a Shopify "metaobject" or a Sanity "document type") needs custom rendering:

1. **List view**: `templates/<channel-slug>.liquid` (e.g. `templates/events.liquid`).
2. **Detail view**: `templates/<channel-slug>/show.liquid` or `templates/<channel-slug-singular>.liquid` — match what the project already does.

Inside the detail template, the entry is exposed via the channel's singular drop (e.g. `event` for `channels.events`); the field set is whatever the channel schema defines (manage with the `nimbu` CLI).

## Channels in Liquid

```liquid
{% comment %} List the 5 upcoming published events {% endcomment %}
{% scope published == true AND starts_at >= site.today %}
  {% sort starts_at asc %}
    {% for event in channels.events limit: 5 %}
      <article>
        <h2><a href="{{ event.url }}">{{ event.title }}</a></h2>
        <time>{{ event.starts_at | localized_date: '%d %b' }}</time>
      </article>
    {% endfor %}
  {% endsort %}
{% endscope %}
```

Notes:
- `entry.url` (or its alias `entry._url`) gives the canonical URL — never hard-code paths.
- Custom fields are exposed by their schema slug. Check the channel schema with `nimbu channels fields list <slug>`.

## What NOT to do

- Don't put cloud-code logic in `templates/` or `snippets/`. Liquid runs at request time on Nimbu's renderer; long-running or sensitive logic belongs in `code/` (cloud functions/jobs/callbacks).
- Don't reference theme assets via raw URLs — use `asset_url` / `theme_image_url`.
- Don't create a `templates/<channel>/index.liquid` AND a `templates/<channel>.liquid` — pick one and stick with the project's convention.
- Don't push the theme without building first — pushing source `src/` files instead of compiled `stylesheets/` + `javascripts/` ships nothing usable.
- Don't edit `stylesheets/app.css` or `javascripts/app.js` directly — those are build outputs and will be overwritten.

## Reference

- [Layouts](https://docs.nimbu.io/themes/concepts/layouts.md)
- [Templates](https://docs.nimbu.io/themes/concepts/templates.md)
- [Snippets](https://docs.nimbu.io/themes/concepts/snippets.md)
- [Channels in Liquid](https://docs.nimbu.io/themes/content/channels.md)
- [Theme config](https://docs.nimbu.io/themes/other/theme-config.md)
- [Performance & caching](https://docs.nimbu.io/themes/other/performance.md)
- Companion `nimbu` skill — `themes push/sync/diff/serve`, channel schemas
