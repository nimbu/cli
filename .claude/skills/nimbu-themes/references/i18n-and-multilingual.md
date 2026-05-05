# i18n & Multilingual

Most Nimbu sites ship in 2+ languages. There are two layers of localization to keep separate:

1. **Content translations** — pages, articles, products, channel entries each have a per-locale version, edited by the content team in the admin UI. Liquid surfaces them automatically when the request matches that locale's URL prefix.
2. **Theme strings** — copy that lives in templates ("Read more", "Sign in", "Add to cart"). Translated through the `{% translate %}` tag against translation YAML files managed by the `nimbu` CLI.

> Live docs: <https://docs.nimbu.io/themes/other/multilingual.md>

## `{% translate %}`

```liquid
{% translate 'home.hero.title', default: 'Welcome' %}
{% translate 'cart.empty', default: 'Your cart is empty' %}
{% translate 'product.add_to_cart', default: 'Add to cart' %}
```

Always pass `default:` — without it, missing keys render as the literal key (`home.hero.title`), which leaks into production.

### Interpolation

```liquid
{% translate 'greeting.signed_in', name: customer.first_name, default: 'Hi %{name}!' %}
```

Use `%{name}` placeholders in the default; pass each placeholder as a named option to the tag.

### `assign:` to capture the result

```liquid
{% translate 'forms.errors.required', default: 'This field is required', assign: 'err_required' %}
<input data-error="{{ err_required }}">
```

Most real themes use the tag form exclusively (a filter form is also documented but rarely seen — confirm against [multilingual.md](https://docs.nimbu.io/themes/other/multilingual.md) before relying on it).

### Where the strings live

`{% translate %}` reads from translation YAML files maintained server-side. Manage them with the CLI (see `nimbu` skill):

```bash
nimbu translations index
nimbu translations push
nimbu translations pull
```

The keys you reference in Liquid (`home.hero.title`) map to nested YAML in the `translations/<locale>.yml` files.

## Locale drops

| Drop | Use |
|------|-----|
| `locale` | Current locale code, e.g. `'nl'`, `'fr'`, `'en'`. |
| `default_locale` | The site's default locale. |
| `locale_url_prefix` | Prefix for URLs in the current locale (e.g. `/fr` or empty for default). |

```liquid
<html lang="{{ locale }}">

{% if locale != default_locale %}
  <link rel="alternate" hreflang="{{ default_locale }}" href="{% localized_path default_locale %}">
{% endif %}
```

## Language switcher

Two pieces:

1. `page.translations` — array of locale objects available for the current page.
2. `{% localized_path 'fr' %}` — resolves the current page's URL in another locale.

```liquid
<nav class="language-switcher" aria-label="Language">
  {% for translation in page.translations %}
    {% if translation.locale != locale %}
      <a href="{% localized_path translation.locale %}" hreflang="{{ translation.locale }}">
        {{ translation.locale | upcase }}
      </a>
    {% endif %}
  {% endfor %}
</nav>
```

Notes:
- `page.translations` only includes locales where the page actually exists. If a page hasn't been translated yet, that locale won't appear.
- The current locale may or may not appear in `page.translations` depending on how the page is set up — filter with `if translation.locale != locale` to be safe.
- `localized_path` works for the current page; for arbitrary URLs, you'll need to construct the path yourself with `locale_url_prefix`.

## Localized dates

Use `localized_date` rather than the standard `date` filter — it respects the request locale:

```liquid
{{ now             | localized_date: '%d %b %Y' }}
{{ article.posted_at | localized_date: 'long' }}
{{ event.starts_at  | localized_date: '%A %d %B' }}
```

Named formats (`'short'`, `'long'`, etc.) are configurable per site. `strftime` patterns work too.

## Locale-aware URLs

For asset URLs (`asset_url`, `theme_image_url`), no extra work — the CDN serves the same files everywhere.

For content URLs:

- `entry.url` and `page.url` are already locale-aware. Use them. (`._url` works as an alias.)
- `url.current_path` gives the current path **with** locale prefix.
- `url.language_independent_path` gives the path **without** locale prefix — useful when you need a stable identifier.

```liquid
<link rel="canonical" href="{{ site.url }}{{ url.current_path }}">
{% for tr in page.translations %}
  <link rel="alternate" hreflang="{{ tr.locale }}" href="{% localized_path tr.locale %}">
{% endfor %}
```

## Patterns worth memorizing

### Pluralization

Liquid doesn't natively support pluralization. Pass a `count` and pick a key:

```liquid
{% if cart.item_count == 0 %}
  {% translate 'cart.items.zero', default: 'Your cart is empty' %}
{% elsif cart.item_count == 1 %}
  {% translate 'cart.items.one', default: '1 item' %}
{% else %}
  {% translate 'cart.items.other', count: cart.item_count, default: '%{count} items' %}
{% endif %}
```

(Some projects abstract this in a `t_count` snippet — match what's already there.)

### Locale-conditional content

```liquid
{% case locale %}
  {% when 'nl' %}
    <p>Bel ons gerust.</p>
  {% when 'fr' %}
    <p>N'hésitez pas à nous appeler.</p>
  {% else %}
    <p>Feel free to call us.</p>
{% endcase %}
```

Prefer `{% translate %}` for short copy. Fall back to `{% case locale %}` for whole sections that diverge structurally between languages (rare).

### Hreflang and canonical in the layout

```liquid
<head>
  <link rel="canonical" href="https://{{ site.subdomain }}{{ url.current_path }}">

  {% for tr in page.translations %}
    <link rel="alternate" hreflang="{{ tr.locale }}" href="{% localized_path tr.locale %}">
  {% endfor %}
  <link rel="alternate" hreflang="x-default" href="{% localized_path default_locale %}">
</head>
```

## Common gotchas

1. **Always pass `default:`** — missing translations otherwise leak the dot-key into production.
2. **Interpolation uses `%{name}`, not `{{name}}` or `:name`**. Pass the value as a named argument: `{% translate 'k', name: x, default: 'Hi %{name}' %}`.
3. **`localized_date` ≠ `date`**. The standard `date` filter doesn't respect locale, so you get English month names regardless of `locale`.
4. **Locale URL prefix isn't always present** for the default locale (e.g. `/` vs. `/nl/`). When concatenating manually, use `locale_url_prefix` rather than hard-coding.
5. **Content translations are independent of theme strings** — translating an article in the admin doesn't affect `{% translate 'article.read_more' %}` text, and vice-versa.
6. **Translation YAML keys are case-sensitive and dot-nested**. `home.HeroTitle` and `home.hero_title` are different keys.

## Reference

- <https://docs.nimbu.io/themes/other/multilingual.md> — multilingual overview
- <https://docs.nimbu.io/themes/filters/dates-time.md> — `localized_date`
- <https://docs.nimbu.io/themes/liquid-context.md> — `locale`, `default_locale`, `locale_url_prefix`
- Companion `nimbu` skill — `nimbu translations push/pull`
