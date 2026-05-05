---
name: nimbu-cloud-code
description: >
  Nimbu Cloud Code — server-side JavaScript that runs on the Nimbu platform
  using the runtime-injected Nimbu SDK. Use when authoring, modifying, or
  debugging cloud functions, background jobs, channel callbacks, routes, or
  extensions inside a Nimbu site's `code/` directory, or whenever the code
  references `Nimbu.Cloud.*`, `Nimbu.Query`, `Nimbu.Object`, `Nimbu.Site`, or
  the `nimbu-js-sdk`. For the CLI side (channel schemas, deploy with
  `nimbu apps push`), use the companion `nimbu` skill.
---

# Nimbu Cloud Code

Cloud Code is server-side JavaScript that runs in Nimbu's V8 sandbox. It extends a Nimbu site with custom logic — data validation, integrations, custom HTTP endpoints, scheduled jobs, and admin extensions — without operating any infrastructure. The `Nimbu` SDK is pre-injected; no `require('nimbu-js-sdk')` is needed inside cloud code.

## Docs are the source of truth

**Always fetch the relevant doc page before writing or modifying cloud code.** This skill summarizes shape, conventions, and gotchas; the docs at `docs.nimbu.io` are authoritative and change more often than this skill.

**Use the `.md` variant of every URL.** Every Nimbu doc page has a sibling markdown file at the same path with a `.md` extension instead of `.html`. WebFetch returns these as plain markdown — much cheaper and more accurate to parse than the rendered HTML.

```
docs.nimbu.io/cloud-code/functions.html   →   docs.nimbu.io/cloud-code/functions.md
docs.nimbu.io/sdk/queries.html            →   docs.nimbu.io/sdk/queries.md
```

### Cloud Code docs

| Topic | URL |
|-------|-----|
| Overview | https://docs.nimbu.io/cloud-code/overview.md |
| Key Concepts | https://docs.nimbu.io/cloud-code/key-concepts.md |
| Callbacks | https://docs.nimbu.io/cloud-code/callbacks.md |
| Cloud Functions | https://docs.nimbu.io/cloud-code/functions.md |
| Routes | https://docs.nimbu.io/cloud-code/routes.md |
| Extensions | https://docs.nimbu.io/cloud-code/extensions.md |
| Background Jobs | https://docs.nimbu.io/cloud-code/jobs.md |
| Available Modules | https://docs.nimbu.io/cloud-code/modules.md |
| Runtime API | https://docs.nimbu.io/cloud-code/runtime-api.md |

### SDK docs

| Topic | URL |
|-------|-----|
| Overview | https://docs.nimbu.io/sdk/overview.md |
| Getting Started | https://docs.nimbu.io/sdk/getting-started.md |
| Environments | https://docs.nimbu.io/sdk/environments.md |
| Objects & Channels | https://docs.nimbu.io/sdk/objects.md |
| Queries | https://docs.nimbu.io/sdk/queries.md |
| Collections | https://docs.nimbu.io/sdk/collections.md |
| Relations & Field Types | https://docs.nimbu.io/sdk/relations-field-types.md |
| Customers & Sessions | https://docs.nimbu.io/sdk/customers-sessions.md |
| System Resources | https://docs.nimbu.io/sdk/system-resources.md |
| Direct API & Cloud Functions | https://docs.nimbu.io/sdk/api-cloud-functions.md |
| Recipes | https://docs.nimbu.io/sdk/recipes.md |
| Reference | https://docs.nimbu.io/sdk/reference.md |

**Agent rule: when in doubt, fetch the `.md` doc first.** Don't guess SDK signatures.

## What Cloud Code can do

| Capability | Registration | Fires |
|-----------|--------------|-------|
| **Cloud Function** | `Nimbu.Cloud.define(name, handler)` | Called explicitly by name from theme, client, or external API. |
| **Background Job** | `Nimbu.Cloud.job(name, handler)` | Run on a schedule or enqueued via `Nimbu.Cloud.schedule(name, params, timing)`. |
| **Before Callback** | `Nimbu.Cloud.before('channel.entries.{created,updated,deleted}', slug, handler)` | Pre-save hook. Mutate or reject data via `response.error`. Also `customer.*`, `product.*`, `coupon.*`, `device.*` events. |
| **After Callback** | `Nimbu.Cloud.after('channel.entries.{created,updated,deleted}', slug, handler)` | Post-save hook. Side effects only. Also `order.*` (after-only), `customer.*`, `product.*`, etc. |
| **HTTP Route** | `Nimbu.Cloud.get/post/put/patch/delete('/path', handler)` | Custom HTTP routes mounted at `/cloud/routes/...` on the site. |
| **Admin Extension** | `Nimbu.Cloud.extend('channel.entries.{show,list}', slug, {...})` | Custom actions in the Nimbu admin UI. |

Generic shape of a cloud function:

```js
Nimbu.Cloud.define('archiveArticle', async (request, response) => {
  const { id } = request.params;
  if (!request.customer) return response.error(401, 'auth required');

  const article = await new Nimbu.Query('articles').get(id);
  article.set('archived', true);
  await article.save();

  response.success({ id: article.id, archived: true });
});
```

Generic shape of a before-callback:

```js
Nimbu.Cloud.before('channel.entries.created', 'articles', async (request, response) => {
  const entry = request.object;
  if (!entry.get('title')) return response.error('title is required');
  entry.set('slug', slugify(entry.get('title')));
  response.success();
});
```

## Runtime modules

The Nimbu sandbox is **fixed** — `Nimbu` is the only truly pre-injected global. The other built-in modules are available either via `require('name')` at the top of the file, or implicitly as globals (most projects do both — declare them in `.eslintrc.json` *and* `require` them where used). Don't try to `require('nimbu-js-sdk')` — it isn't on the module path.

| Module | How to use | Docs |
|--------|-----------|------|
| `Nimbu` | Pre-injected global. `Nimbu.Query`, `Nimbu.Object`, `Nimbu.Cloud`, `Nimbu.Site`, etc. | [sdk/overview.md](https://docs.nimbu.io/sdk/overview.md) |
| `Mail` | `const Mail = require('mail');` — `Mail.send({to, subject, html})`. | [cloud-code/modules.md](https://docs.nimbu.io/cloud-code/modules.md) |
| `HTTP` | `const HTTP = require('http');` — `HTTP.get(url, opts)`, `HTTP.post(url, body, opts)`. | [cloud-code/modules.md](https://docs.nimbu.io/cloud-code/modules.md) |
| `crypto` | `const crypto = require('crypto');` — hashing, HMAC. | [cloud-code/modules.md](https://docs.nimbu.io/cloud-code/modules.md) |
| `jwt` | `const jwt = require('jwt');` — sign / verify JWTs. | [cloud-code/modules.md](https://docs.nimbu.io/cloud-code/modules.md) |
| `I18n` | `const I18n = require('I18n');` — translations for the active locale. | [cloud-code/modules.md](https://docs.nimbu.io/cloud-code/modules.md) |
| `lodash` | `const _ = require('lodash');` or `const { compact } = require('lodash');` | — |
| `moment` | `const moment = require('moment');` | — |

The set of available modules is sandbox-fixed. Adding a package to `package.json` does **not** make it loadable at runtime — see [project-layout.md](references/project-layout.md).

## SDK essentials

A minimum mental map of the SDK an agent should hold. See [references/sdk-cheatsheet.md](references/sdk-cheatsheet.md) for the full surface.

### Queries

```js
const articles = await new Nimbu.Query('articles')
  .equalTo('published', true)
  .greaterThan('published_at', cutoffDate)
  .descending('published_at')
  .limit(20)
  .find();
```

Common methods: `.find()`, `.findAll()`, `.first()`, `.get(id)`, `.count()`, `.equalTo`, `.notEqualTo`, `.greaterThan`/`.lessThan`, `.containedIn`, `.include('rel')`, `.limit`, `.page`/`.per`, `.descending`/`.ascending`, `Nimbu.Query.or(q1, q2)`.

### Objects

```js
const article = await new Nimbu.Query('articles').get(id);
article.set('archived', true);
await article.save();
```

`.get(field)`, `.set(field, value)`, `.has(field)`, `.unset(field)`, `.increment(field, n)`, `.save()`, `.destroy()`. Create with either `new Nimbu.Object('articles', {...})` or `Nimbu.Object.extend('articles')`.

**Relations**: prefer `.include('rel')` on the query, or `await article.fetchWithInclude(['rel'])` after the fact:

```js
const article = await new Nimbu.Query('articles').include('author').get(id);
console.log(article.get('author').get('email'));   // already materialized
```

### Cloud hooks

| Pattern | Purpose |
|---------|---------|
| `Nimbu.Cloud.define(name, fn)` | Define a cloud function |
| `Nimbu.Cloud.job(name, fn)` | Define a background job |
| `Nimbu.Cloud.before(event, slug, fn)` | Pre-save hook |
| `Nimbu.Cloud.after(event, slug, fn)` | Post-save hook |
| `Nimbu.Cloud.schedule(jobName, params, timing)` | Enqueue a job. `timing` is an **object**: `{}` or `null` for now, `{at: Date}`, `{in: '30m'}`, `{every: '0 9 * * *'}` |
| `Nimbu.Cloud.get/post/put/patch/delete(path, fn)` | HTTP route on `/cloud/routes/...` |
| `Nimbu.Cloud.extend(action, slug, opts)` | Admin UI extension (e.g. `'channel.entries.show'`) |

### Handler request/response context

Most handlers are `(request, response) => {...}` (sync or async). What's on `request` depends on the kind of handler:

| Field | Functions | Jobs | Callbacks | Routes |
|-------|:---------:|:----:|:---------:|:------:|
| `request.params` | ✓ | ✓ | — | ✓ (URL params + query + body, merged) |
| `request.meta` | ✓ | — | — | — |
| `request.customer` | ✓ | — | ✓ | ✓ |
| `request.object` | — | — | ✓ (the entry) | — |
| `request.changes` / `lastUpdatedAt` | — | — | ✓ (updates only) | — |
| `request.actor` / `request.user` | — | — | ✓ | — |
| `request.headers` / `request.body` / `request.path` / `request.host` / `request.locale` / `request.session` | — | — | — | ✓ |

| `response` method | Use |
|-------------------|-----|
| `response.success(data)` | Return success payload |
| `response.error('msg')` or `response.error(code, 'msg')` | Return an error — do **not** `throw` |
| `response.error('field', 'msg')` | (callbacks) field-level validation error |
| `response.json(obj)` / `response.html(str)` / `response.redirect_to(url)` / `response.render(tpl, vars)` / `response.send(data, opts)` | (routes) richer responses |

### Site & env

```js
Nimbu.Site.subdomain                  // e.g. 'my-site-staging'
Nimbu.Site.env.get('STRIPE_KEY')      // site-level env var
```

Set env vars via `nimbu apps config` (see the companion `nimbu` skill) before reading them.

## Project conventions

Cloud-code files live in the **site's `code/` directory**, alongside the theme. The CLI's `themes push` deliberately **skips** `code/` — deploys go through `nimbu apps push`. See [references/project-layout.md](references/project-layout.md) for full conventions.

Highlights:

- **One file per domain**, not a single `index.js`. Typical names: `articles.js`, `orders.js`, `customers.js`, `helpers.js`, `environment.js`. Each file registers its own `Cloud.define` / `Cloud.job` / `Cloud.before` etc.
- **`.eslintrc.json`** declares the runtime globals (`Nimbu`, `Mail`, `HTTP`, `crypto`, `jwt`, `I18n`) so lint doesn't flag them.
- **Optional build pipeline**: a `Makefile` may compile CoffeeScript with `coffee -b -c -o dist *.coffee` and copy plain JS into `dist/`; `nimbu apps push` then deploys from `dist/`.
- **`package.json` does NOT control the runtime.** The sandbox is fixed. Any `package.json` in a cloud-code project exists only for dev tooling (`@nimbu/testing`, eslint, jest).
- **Environment branching** by `Nimbu.Site.subdomain` or `Nimbu.Site.env.get(...)`. Keep both branches present in code.

## Running and deploying

These are CLI commands — they belong to the companion `nimbu` skill, but two are essential here:

```bash
nimbu apps push                       # deploy code/ (or dist/) to the site
nimbu apps push --only file.js        # push a single file
nimbu functions run myFn --site ...   # invoke a cloud function remotely
nimbu jobs run myJob --site ...       # invoke a job remotely
```

For the full CLI surface (auth, sites, channel introspection, deployment flags) see the companion `nimbu` skill — especially `nimbu channels fields list --channel <slug> --json` to pull the exact field schema before writing code against a channel.

## Testing

Cloud-code projects test with **Jest + `@nimbu/testing`**. The package provides `setup`, `mockAPI`, `mockRequest`, `getCloudFunctionHandler`, and `customerFromFixture` helpers so handlers can run in isolation against fixtures. See [references/testing.md](references/testing.md).

```js
const { getCloudFunctionHandler, mockRequest } = require('@nimbu/testing');

test('archiveArticle marks the article archived', async () => {
  const handler = getCloudFunctionHandler('archiveArticle');
  const result = await handler(mockRequest({ params: { id: 'abc' }, customer: someCustomer }));
  expect(result.archived).toBe(true);
});
```

Verify the exact `@nimbu/testing` API against the package's README for the project at hand — it evolves independently of `docs.nimbu.io`.

## Common gotchas

1. **`query.find()` returns ONE page, not the full collection.** Default page size is ~30. To get every match, use `query.findAll()` (loads all pages into an array), `query.collection().fetch()` (returns a `Nimbu.Collection`), or `query.each(async (entry) => {...})` / `query.eachBatch(async (batch) => {...})` (streams results — useful for live progress on long jobs). Reaching for `.find()` and silently truncating at 30 items is one of the most common cloud-code bugs.
2. **Relations**: prefer `.include('author')` on the query, or `await article.fetchWithInclude(['author'])` after the fact. Both materialize related objects in one round-trip; `article.fetch({ include: ['author'] })` is the equivalent options form.
3. **Event name is `deleted`, not `destroyed`.** `channel.entries.deleted` — even though the method to delete an object is `.destroy()`, the callback event uses `deleted`.
4. **`Nimbu.Cloud.schedule` takes a timing OBJECT, not a cron string.** Use `{every: '0 9 * * *'}`, `{at: new Date(...)}`, `{in: '30m'}`, or `{}`/`null` for immediate. Passing the bare string `'0 9 * * *'` will silently fail or schedule incorrectly.
5. **`request.customer` is `null` when unauthenticated.** Gate explicitly — never assume a customer.
6. **Errors go through `response.error(...)`, not `throw`.** A thrown error becomes a generic 500. Use `response.error('msg')`, `response.error(code, 'msg')`, or `response.error('field', 'msg')` for callback field errors.
7. **Async style**: prefer `async/await`. Older projects mix `Nimbu.Future.when(...)` and `.then().fail()` chains — match the existing file's style and don't mix within one file.
8. **Subdomain env branches**: when you see `Nimbu.Site.subdomain === 'foo-staging'` in code, both branches must exist or the prod path silently breaks on staging (and vice versa).
9. **New env vars**: set them via `nimbu apps config` (companion CLI skill) **before** the code that reads them is deployed; otherwise `Nimbu.Site.env.get` returns `undefined`.
10. **`package.json` does NOT control the runtime.** Anything you `require` at runtime must be a Nimbu-injected global or one of the conventionally available modules (`lodash`, `moment`). Adding to `package.json` does not make a module available in the sandbox.
11. **`code/` is not theme territory.** `nimbu themes push/sync` skips it — deploy with `nimbu apps push`.
12. **Callbacks vs functions**: pre-save hooks can `response.error(...)` to reject the write. After-callbacks cannot reject — their return values are ignored. `order.*` events are after-only.

## See also

- **[references/sdk-cheatsheet.md](references/sdk-cheatsheet.md)** — full SDK surface (queries, objects, hooks, modules) with per-section doc links.
- **[references/project-layout.md](references/project-layout.md)** — file layout, build pipeline, eslint globals, env-detection patterns.
- **[references/testing.md](references/testing.md)** — `@nimbu/testing` + Jest setup and patterns.
- **Companion `nimbu` skill** — CLI for channels, schemas, deploys, env config, theme sync.
- **Companion `nimbu-themes` skill** — Liquid templates, drops, tags, filters, forms, editables, i18n in `layouts/`/`templates/`/`snippets/`.
- **Live docs (use `.md` variant)** — [Cloud Code overview](https://docs.nimbu.io/cloud-code/overview.md) and [SDK overview](https://docs.nimbu.io/sdk/overview.md).
