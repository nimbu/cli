# Liquid Cheatsheet (Nimbu)

A reference of the **Nimbu-specific** Liquid tags, filters, and drops. Standard Liquid (Shopify) — `{% if %}`, `{% for %}`, `{% assign %}`, `forloop`, `upcase`, `downcase`, `truncate`, `date`, etc. — is documented at <https://shopify.dev/docs/api/liquid> and not repeated here.

> Always confirm against the live docs: <https://docs.nimbu.io/docs/themes/filters-tags.md> and <https://docs.nimbu.io/docs/themes/liquid-context.md>. The lists here track those pages.

## Tags

### Theme rendering

| Tag | Shape | Purpose |
|-----|-------|---------|
| `layout` | `{% layout 'name' %}` | Pick a layout file. Place at the top of a template. |
| `include` | `{% include 'snippet', key: value %}` | Render a snippet, optional named params. **The canonical form** — real themes use this exclusively. |
| `snippet` | `{% snippet 'name', key: value %}` | Documented alias for `include`. Rarely used in practice — prefer `{% include %}`. |
| `paginate` | `{% paginate channels.x by 20 %}…{% endpaginate %}` | Wrap a query drop to enable pagination. **Iterate `paginate.collection` inside, not the original drop.** Inside the block, `paginate` is in scope for `default_pagination`. |
| `cache` | `{% cache 'key' %}…{% endcache %}` | Cache a Liquid fragment. Use a deterministic key. |

### Channel/product/collection queries

These work on the server-side query drops (`channels.x`, `products`, `collections`). They have no effect on plain arrays.

| Tag | Shape | Purpose |
|-----|-------|---------|
| `scope` | `{% scope field == 'value' AND other == 'x' %}…{% endscope %}` | Restrict the query to entries matching the expression. **The standard form**. Logical operators (`AND`, `OR`, `and`, `or`) are supported; both `==` and `=` work. |
| `with_scope` | `{% with_scope field == 'value' %}…{% endwith_scope %}` | Documented alternative to `scope`; rarely seen in real themes. |
| `sort` | `{% sort updated_at desc, title asc %}…{% endsort %}` | Apply server-side ordering. |
| `condition` | `{% condition %}` | Compose complex logical query expressions. |
| `set` | `{% set hash, key, value %}` | Build/extend a hash dynamically (used inside scope/sort). |

### Navigation & structure

| Tag | Shape | Purpose |
|-----|-------|---------|
| `nav` | `{% nav 'menu_slug', class: 'main-menu', depth: 2 %}` | Render a menu by slug from the `menus` drop. Supports `cache:`. |
| `breadcrumbs` | `{% breadcrumbs %}` | Render breadcrumbs for the current page hierarchy. |
| `tree` | `{% tree %}` | Render a page tree. |
| `search` | `{% search %}` | Execute a configured search query. |
| `localized_path` | `{% localized_path 'nl' %}` | Resolve the current page's URL for another locale. |

### Editable content (page editor)

The `{% editable_* %}` family registers regions that the marketing team can edit through the Nimbu admin without touching code. The Liquid signature is what the editor reads to discover editable points.

| Tag | Shape |
|-----|-------|
| `editable_field` | `{% editable_field 'key', label: 'Hero title' %}Default value{% endeditable_field %}` |
| `editable_text` | `{% editable_text 'key' %}<p>Default body</p>{% endeditable_text %}` (WYSIWYG) |
| `editable_file` | `{% editable_file 'key' %}https://placehold.it/600x400{% endeditable_file %}` |
| `editable_select` | `{% editable_select 'key', options: 'a\|b', labels: 'A\|B' %}a{% endeditable_select %}` |
| `editable_switch` | `{% editable_switch 'key' %}true{% endeditable_switch %}` (boolean) |
| `editable_reference` | `{% editable_reference 'Featured', to: 'products', assign: 'featured' %}{% endeditable_reference %}` |
| `editable_group` | `{% editable_group 'name' %}…{% endeditable_group %}` (collapsible section) |
| `editable_canvas` | `{% editable_canvas 'name' %}…{% endeditable_canvas %}` (nested-content zone) |
| `repeatable` | `{% repeatable 'name' %}…{% endrepeatable %}` (add/reorder/remove blocks) |

All editable tags accept `label`, `hint`, and `assign`. See [forms-and-editables.md](forms-and-editables.md).

### Forms

`{% form %}` injects CSRF (`auth_token`), validation, and flash handling. Don't roll your own `<form>` tag.

| Tag | Shape |
|-----|-------|
| `form` | `{% form channels.contact, class: 'contact-form' %}…{% endform %}` |
| `form_tag` / `simple_form_tag` | Generic wrappers when not bound to a channel |
| `input` | `{% input 'name', label: 'Name', required: true %}` |
| `text_area` | `{% text_area 'message' %}` |
| `password_field`, `hidden_field`, `file_field` | Same shape as `input` |
| `check_box`, `select_tag`, `collection_select`, `date_select`, `time_select`, `multi_date_tag` | Specialized inputs |
| `inputs_for_fields` | Auto-generates inputs from a channel's schema |
| `error_messages_for` | `{% error_messages_for object %}` |
| `submit_tag` | `{% submit_tag 'Send', class: 'btn btn-primary' %}` |
| `input_tag_template` | Customize widget rendering |

See [forms-and-editables.md](forms-and-editables.md) for full patterns.

### i18n

| Tag | Shape |
|-----|-------|
| `translate` | `{% translate 'home.hero.title', default: 'Welcome' %}` |
| `localized_path` | `{% localized_path 'fr' %}` |

See [i18n-and-multilingual.md](i18n-and-multilingual.md).

### Auth & integrations

| Tag | Shape | Purpose |
|-----|-------|---------|
| `login_with` | `{% login_with provider: 'facebook' %}` | Start an OAuth flow. |
| `unlink_from` | `{% unlink_from provider: 'twitter' %}` | Disconnect an OAuth provider. |
| `oauth2_consent_form` | `{% oauth2_consent_form %}` | Render the OAuth consent UI. |
| `consent_manager` | `{% consent_manager %}` | Render the consent-manager entry point. |
| `safari_push_js` | `{% safari_push_js %}` | Safari push-notifications helper. |
| `google_analytics` | `{% google_analytics %}` | Inject the GA snippet manually (most projects use `analytics`). |
| `analytics` | `{% analytics %}` | Generic analytics loader. |
| `theme_liquid_version` | `{% theme_liquid_version %}` | Expose the active Liquid runtime semantics. |
| `consume` | `{% consume %}` | Read from deferred content for reuse. |

## Filters

Standard Shopify Liquid filters work as expected. Listed here are the **Nimbu additions** plus filters that look standard but have Nimbu-specific behavior.

### Assets & CDN

| Filter | Example |
|--------|---------|
| `asset_url` | `{{ 'app.css' \| asset_url }}` → CDN URL |
| `theme_image_url` | `{{ 'logo.svg' \| theme_image_url }}` → CDN URL for a theme image |
| `stylesheet_tag` | `{{ 'app.css' \| stylesheet_tag, media: 'screen,print' }}` → `<link>` |
| `javascript_tag` | `{{ 'app.js' \| javascript_tag }}` → `<script>` |
| `auto_discovery_link_tag` | `{{ 'feed.xml' \| auto_discovery_link_tag }}` → RSS/Atom `<link>` |
| `download` | `{{ file_url \| download }}` → appends `?dl=1` for `Content-Disposition: attachment` |

### Image transforms

Compose like `{{ image.url | filter: 'resize', width: 800 | grayscale }}`.

| Filter | Use |
|--------|-----|
| `filter` | Generic transform: `width`, `height`, cropping (`crop: 'fill'`), focal point. |
| `grayscale`, `sepia`, `vignette` | Visual effects. |

### Numbers, money, weight

| Filter | Example |
|--------|---------|
| `money_with_currency` | `{{ 1995 \| money_with_currency }}` → `€19.95` |
| `money_without_currency` | `{{ 1995 \| money_without_currency }}` → `19.95` |
| `number_to_currency` | Custom format with `unit:`, `precision:`, `delimiter:`, `separator:` |
| `weight` | grams → kilograms |
| `weight_with_unit` | grams → `"1.5 kg"` |
| `number_to_human` | `1500` → `"1.5 thousand"` |
| `number_to_human_size` | `2048` → `"2 KB"` |
| `number_to_percentage` | `{{ 0.245 \| number_to_percentage, precision: 1 }}` → `"24.5%"` |
| `number_to_phone` | Format as phone number. |
| `number_with_delimiter` | Insert thousands separator. |
| `number_with_precision` | Round/pad to precision. |

### Dates & time

| Filter | Example |
|--------|---------|
| `localized_date` | `{{ now \| localized_date: '%d %b %Y' }}` (also accepts named formats) |
| `time_ago_in_words` | `{{ comment.created_at \| time_ago_in_words }}` → `"3 minutes ago"` |
| `to_datetime` | Parse a string into a DateTime. |

### Arrays & collections

Beyond standard Liquid (`size`, `first`, `last`, `join`):

| Filter | Use |
|--------|-----|
| `where`, `where_exp` | Filter by property or expression |
| `find`, `find_exp` | First match by property or expression |
| `group_by`, `group_by_exp` | Group entries into a hash |
| `sort`, `numeric_sort` | Sort by property (dot notation OK) |
| `intersection`, `union` | Set ops on arrays |
| `push`, `pop`, `shift`, `unshift` | Stack/queue ops |
| `keys`, `values` | Hash → array |
| `is_empty` | Whitespace-aware emptiness |

### Strings & text

| Filter | Use |
|--------|-----|
| `markdown`, `textile` | Convert source markup to HTML. |
| `strip_html`, `strip` | Sanitize / trim. |
| `parameterize` | Slugify (URL-safe). |
| `transliterate` | ASCII-fold accented characters. |
| `replace_by_regex` | Regex substitution. |
| `emoticize` | Replace `:)` etc. with emoji images. |
| `truncate`, `truncatewords` | Length limits. |
| `concat` | String/array concatenation. |
| `uri_encode`, `uri_decode` | Percent-encode/-decode. |

### JSON & API

| Filter | Use |
|--------|-----|
| `json` | Serialize a Liquid object to JSON (e.g. for inline `<script>`). Accepts `omit:` to drop keys. |
| `from_json` | Parse a JSON string. |
| `api_sso_info` | Build SSO meta headers for a token. |

### Commerce

| Filter | Use |
|--------|-----|
| `add_to_cart` | Render an add-to-cart form for a product (variant + quantity). |
| `update_cart_item` | Render a quantity-update form for a cart line item. |
| `delete_cart_item` | Render a remove-from-cart button. |
| `delete_cart_group` | Remove a grouped cart item set. |
| `checkout_button` | Render the checkout button. Confirm exact arg shape against [commerce.md](https://docs.nimbu.io/docs/themes/filters/commerce.md). |
| `payment_form` | Render a payment-method selection form. |
| `auto_apply_coupon` | Apply a coupon code on page load. |

### Special-purpose

| Filter | Use |
|--------|-----|
| `default_pagination` | `{{ paginate \| default_pagination: 'previous_label:«', 'next_label:»', 'style:bootstrap4' }}` — **args are colon-separated strings, not Liquid kwargs.** Only inside `{% paginate %}`. |
| `qr`, `qr_datauri`, `qr_svg`, `qr_html` | QR code rendering (PNG / data URI / SVG / email-safe HTML). |
| `country` | ISO code → country object. |
| `gravatar` | Email → Gravatar URL. |
| `is_geopoint`, `to_geopoint` | Geo-coordinate detection/conversion. |
| `add_query_params` | Append params to a URL. |

### Hashing & encoding

| Filter | Use |
|--------|-----|
| `md5`, `sha1`, `sha224`, `sha256`, `sha384`, `sha512` | Hash a string. |
| `hmac256` | HMAC-SHA-256 signature. |
| `base64_encode`, `base64_decode` | Base-64 round trip. |

### Type checks & coercion

| Filter | Use |
|--------|-----|
| `to_i`, `to_f` | Coerce to integer / float. |
| `is_number`, `is_float` | Type guards. |
| `random` | Random integer (optional bounds). |
| `modulo` | Modulo with offset. |

### Analytics

| Filter | Use |
|--------|-----|
| `google_analytics_tag` | Build the GA snippet for a tracking ID. |
| `google_analytics_ecommerce_code` | GA e-commerce conversion code. |

## Drops

These objects are populated automatically by Nimbu and available everywhere:

### Site & request

| Drop | Notes |
|------|-------|
| `site` | The current site. Has `channels`, `products`, `settings`, locale info. |
| `template` / `template_name` | Slugified identifiers for the active template (useful for `<body class="{{ template_name }}">`). |
| `theme_version` | Version string — useful for cache busting. |
| `now` | Current UTC timestamp at render. |
| `today` | Site-local date at render. |
| `params` | Sanitized query/form params (strings, arrays, hashes) excluding reserved keys. |
| `path` | Resolved request path (honours simulator overrides). |
| `url` | Has `current`, `current_path`, `language_independent_path`, `query_string`. |
| `auth_token` | CSRF token used automatically by `{% form %}`. |
| `flash` | Flash messages from previous requests. |
| `seo` | Site-level SEO defaults (`description`, `keywords`). |

### Localization

| Drop | Notes |
|------|-------|
| `locale` | Current locale code (e.g. `'nl'`). |
| `default_locale` | The site's default locale. |
| `locale_url_prefix` | Prefix for the current locale (e.g. `/nl`). |
| `config` | Per-site config (locales, countries, shipping methods, …). |

### Auth

| Drop | Notes |
|------|-------|
| `customer` | The logged-in customer. **`null` when anonymous** — gate explicitly. |

### Content

| Drop | Notes |
|------|-------|
| `page` | Current page (title, content, og_image, translations, locale-aware URL). |
| `channels` | Custom collections by slug — `channels.articles`, `channels.events`, etc. Supports `.all`, `.first`, `.last`, `.where`, `.group_by`, custom-field access. |
| `menus` | Navigation trees by slug — used by `{% nav %}`. |
| `blogs` | Blog and article aggregation; `blogs.posts`, etc. |

### Webshop

| Drop | Notes |
|------|-------|
| `cart` | The current open order/cart, if a session exists. |
| `products` | Entire product catalogue (with filters). |
| `collections` | Product groupings by slug. |
| `product_types`, `product_vendors` | Product taxonomy lists. |
| `orders` | Order history (used on customer-account pages). |
| `customers` | Customer querying interface. |

### Misc

| Drop | Notes |
|------|-------|
| `consent_manager` | User consent preferences. |

## Patterns worth memorizing

### Paginated channel listing

```liquid
{% paginate channels.articles by 12 %}
  {% for article in paginate.collection %}
    {% include 'cards/article', article: article %}
  {% endfor %}

  {{ paginate | default_pagination: 'previous_label:«', 'next_label:»', 'style:bootstrap4' }}
{% endpaginate %}
```

`paginate.collection` is the page slice. `paginate` itself exposes `current_page`, `total_pages`, `total_entries`, `per_page`, `next.url`, `previous.url`.

### Scoped query

```liquid
{% scope status == 'published' AND published_at <= site.today %}
  {% sort published_at desc %}
    {% for post in channels.posts limit: 5 %}
      <h2>{{ post.title }}</h2>
    {% endfor %}
  {% endsort %}
{% endscope %}
```

### Authenticated-only block

```liquid
{% if customer %}
  <p>Hi {{ customer.first_name }}.</p>
{% else %}
  <a href="/login">Sign in</a>
{% endif %}
```

### Asset reference with cache busting

```liquid
{{ 'app.css' | stylesheet_tag, media: 'screen' }}
{{ 'app.js'  | javascript_tag, defer: true }}
<img src="{{ 'logo.svg' | theme_image_url }}" alt="">
```

### Inline JSON for client-side JS

Liquid has no hash-literal syntax, so emit each field individually with the `json` filter:

```liquid
<script>
  window.__BOOT__ = {
    customer: {{ customer | json }},
    locale:   {{ locale   | json }},
    page_id:  {{ page.id  | json }}
  };
</script>
```

## References

- <https://docs.nimbu.io/docs/themes/filters-tags.md> — full tags + filters
- <https://docs.nimbu.io/docs/themes/liquid-context.md> — drops
- <https://docs.nimbu.io/docs/themes/filters-tags.md> — overview of tags and filters; use the split-by-purpose filter pages linked from there for details
- <https://shopify.dev/docs/api/liquid> — standard Liquid (Shopify)
