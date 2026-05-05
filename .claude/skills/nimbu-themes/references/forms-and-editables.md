# Forms & Editable Content

Nimbu themes have two distinct authoring surfaces:

- **`{% form %}` + form helpers** — request-time forms (contact, registration, login, custom channel inputs). Render at request time, post back to Nimbu, validate against a channel schema.
- **`{% editable_* %}` + `{% repeatable %}`** — page-editor regions that the marketing/content team edits in the admin UI without touching code.

Don't confuse them: editables don't accept user input at request time, and form helpers don't expose anything to the page editor.

> Live docs: <https://docs.nimbu.io/themes/forms-editable-content.md>, <https://docs.nimbu.io/themes/other/forms.md>

## `{% form %}`

`{% form %}` wraps an HTML `<form>` and adds the things you'd otherwise have to remember manually:

- `auth_token` (CSRF) hidden input
- Channel-bound validation (when bound to `channels.x`)
- Flash messages (`flash.notice`, `flash.error`) populated on the next request
- Auto-redirect on success (configurable via `success_url`)

### Bound to a channel

```liquid
{% form channels.contact, class: 'contact-form' %}
  {% input 'name',    label: 'Your name',  required: true %}
  {% input 'email',   label: 'Your email', type: 'email', required: true %}
  {% text_area 'message', label: 'Message', rows: 6 %}

  {% error_messages_for channels.contact %}
  {% submit_tag 'Send', class: 'btn btn-primary' %}
{% endform %}
```

When this submits, Nimbu validates `name`/`email`/`message` against the `contact` channel's field schema. Validation errors come back via `error_messages_for`; on success, flash messages or redirect kick in.

### Built-in flows by name

`{% form %}` accepts string names for built-in flows in addition to channel drops:

```liquid
{% form 'login',    class: 'customer-form' %}…{% endform %}
{% form 'register', class: 'signup-form'   %}…{% endform %}
{% form 'account',  action: '/mijn-account/customer-info' %}…{% endform %}
{% form 'order',    class: 'checkout' %}…{% endform %}
```

These wire up to Nimbu's customer/auth/order flows. Pass `action:` only when overriding the default endpoint.

### Generic form (no channel or built-in binding)

```liquid
{% form_tag '/custom-endpoint', class: 'newsletter' %}
  {% input 'email', type: 'email', required: true %}
  {% submit_tag 'Subscribe' %}
{% endform %}
```

`{% form_tag %}` (and `{% simple_form_tag %}`) close with `{% endform %}` — the same closer as `{% form %}`. Use these when posting to a custom endpoint (typically a cloud function in `code/`). You're responsible for validation server-side.

### Helper inventory

| Tag | Use |
|-----|-----|
| `input` | Text/email/number/tel/url. Pass `type:` to vary. |
| `text_area` | Multi-line. `rows:`, `cols:`. |
| `password_field` | Password input. |
| `hidden_field` | Hidden input — `{% hidden_field 'next', value: '/thanks' %}`. |
| `file_field` | File upload. |
| `check_box` | Checkbox. |
| `select_tag` | Native `<select>` with explicit options. |
| `collection_select` | `<select>` populated from a Liquid collection. |
| `date_select`, `time_select`, `multi_date_tag` | Date/time pickers. |
| `inputs_for_fields` | Auto-render all inputs from the bound channel's schema. |
| `error_messages_for` | Render validation errors. |
| `submit_tag` | Submit button. `class:`, `data-…`. |
| `input_tag_template` | Override a specific widget's render template. |

### Validation & error display

```liquid
{% form channels.contact %}
  {% error_messages_for channels.contact %}

  {% input 'email', label: 'Email', type: 'email', required: true %}
  {% submit_tag 'Send' %}
{% endform %}
```

`{% error_messages_for %}` renders all validation errors for the bound object as a single block. Many real-world themes pair this with client-side validation (Parsley, native `required`/`type=email`) so users see immediate feedback before round-tripping.

Server-side validation lives in the channel schema (manage via the `nimbu` CLI, `nimbu channels fields ...`) or in a `Cloud.before('channel.entries.created', 'contact', …)` callback in `code/` (see `nimbu-cloud-code` skill).

### Common form patterns

#### CSRF-correct AJAX submission

If you want JS-driven submission, render the form with `{% form %}` and intercept on the JS side. The `auth_token` injected by `{% form %}` is the right CSRF value to send back.

If your project uses Alpine, wrap the form in an `x-data` element rather than passing Alpine attributes through the form tag (special-character keys like `@submit` aren't valid Liquid kwargs):

```liquid
<div x-data="contactForm()" @submit.prevent="submit($event)">
  {% form channels.contact, class: 'contact-form' %}
    {% input 'email', type: 'email', required: true %}
    {% submit_tag 'Send' %}
  {% endform %}
</div>
```

#### Multi-step form

Keep state across steps with `hidden_field`:

```liquid
{% form channels.application %}
  {% hidden_field 'step', value: 'step-2' %}
  {% if params.step == 'step-1' %}
    {% include 'forms/application/step-1' %}
  {% else %}
    {% include 'forms/application/step-2' %}
  {% endif %}
{% endform %}
```

## Editable content

Editables let the marketing team change strings, images, links, and structured blocks via the Nimbu admin **without** modifying templates. The Liquid signature is what registers the editable in the page editor — change the `key` and the editable resets.

### The six editable variants

```liquid
{# Single-line text — plain string #}
{% editable_field 'hero_title', label: 'Hero title' %}Welcome{% endeditable_field %}

{# Rich text / WYSIWYG — HTML output #}
{% editable_text 'hero_body', label: 'Intro copy' %}<p>Default body</p>{% endeditable_text %}

{# File / image — outputs the URL #}
<img src="{% editable_file 'hero_image' %}{{ 'placeholder.png' | theme_image_url }}{% endeditable_file %}" alt="">

{# Dropdown #}
{% editable_select 'cta_style', options: 'ghost|solid', labels: 'Ghost|Solid' %}ghost{% endeditable_select %}

{# Boolean toggle #}
{% editable_switch 'show_banner' %}true{% endeditable_switch %}

{# Reference to another object (product, page, channel entry, …) #}
{% editable_reference 'Featured product', to: 'products', assign: 'featured_product' %}{% endeditable_reference %}
{% if featured_product %}
  <a href="{{ featured_product._url }}">{{ featured_product.name }}</a>
{% endif %}
```

All variants accept:

- `label:` — what the editor sees in the admin UI.
- `hint:` — a tooltip for context.
- `assign:` — when set, the resolved value is also assigned to a Liquid variable for use later in the template.

### Grouping & canvases

```liquid
{% editable_group 'hero', label: 'Hero section' %}
  {% editable_field 'title' %}Welcome{% endeditable_field %}
  {% editable_text  'body'  %}<p>…</p>{% endeditable_text %}
  {% editable_file  'image' %}{{ 'placeholder.png' | theme_image_url }}{% endeditable_file %}
{% endeditable_group %}
```

`editable_canvas` declares a zone where the editor can drop arbitrary blocks (snippet-based) without you predefining them.

### Repeatable blocks

```liquid
{% editable_group 'features' %}
  {% repeatable 'item', label: 'Feature' %}
    <article class="feature">
      <h3>{% editable_field 'title' %}Default{% endeditable_field %}</h3>
      <p>{% editable_text  'body'  %}<p>Default body</p>{% endeditable_text %}</p>
    </article>
  {% endrepeatable %}
{% endeditable_group %}
```

The editor can now add/reorder/remove "Feature" items, each with its own `title` + `body`.

### Patterns worth memorizing

#### Default content from a channel entry

```liquid
{% editable_reference 'Banner', to: 'channels.banners', assign: 'banner' %}{% endeditable_reference %}
{% if banner %}
  <div class="banner">
    <h2>{{ banner.title }}</h2>
    <p>{{ banner.body }}</p>
  </div>
{% endif %}
```

#### Image with placeholder

```liquid
<img
  src="{% editable_file 'photo', label: 'Photo' %}{{ 'placeholder.svg' | theme_image_url }}{% endeditable_file %}"
  alt="{% editable_field 'photo_alt', label: 'Photo alt text' %}{% endeditable_field %}">
```

#### Conditional section

```liquid
{% editable_switch 'show_testimonials' %}true{% endeditable_switch %}
{% if show_testimonials %}
  {% include 'sections/testimonials' %}
{% endif %}
```

(Use `assign:` if the boolean is needed elsewhere in the template too.)

## Common gotchas

1. **Don't change an editable `key`** without communicating with the content team — it resets the editable to its default and drops whatever they've entered.
2. **Editables wrap defaults**, not output. The content between the open/close tags is the **default** value used until someone edits in the admin UI.
3. **Form fields without `{% form %}`** miss CSRF and won't pass channel validation. Always wrap.
4. **`{% input %}` outside a `{% form %}` block** is meaningless — the helpers need form context.
5. **`error_messages_for` requires the form to be bound** to a channel (or to whatever you pass in). Generic `form_tag` forms don't get auto-validation.
6. **`editable_reference` returns `nil` until set** — guard with `{% if banner %}`.
7. **`{% editable_file %}` content is the URL string**, not an HTML tag. Always wrap it in `<img src="…">` or `<a href="…">` yourself.
8. **`{% repeatable %}` items share the surrounding scope** — variables you `{% assign %}` outside leak in, which is sometimes useful and sometimes surprising.

## Reference

- <https://docs.nimbu.io/themes/forms-editable-content.md> — combined reference
- <https://docs.nimbu.io/themes/other/forms.md> — forms
- <https://docs.nimbu.io/themes/concepts/editables.md> — editables
- Companion `nimbu` skill — `nimbu channels fields ...` for managing channel schemas
- Companion `nimbu-cloud-code` skill — `Cloud.before/after` callbacks for server-side validation
