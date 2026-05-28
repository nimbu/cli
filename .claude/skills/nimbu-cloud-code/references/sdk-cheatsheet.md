# SDK Cheatsheet

The full surface of the runtime-injected `Nimbu` SDK as used inside Cloud Code. Examples are intentionally generic (`articles`, `orders`, `customers`); replace with the channel slugs that exist on the site you're working on. **When in doubt, fetch the matching `.md` doc page** — every section ends with the relevant URL.

## Queries

```js
const query = new Nimbu.Query('articles');
query.equalTo('published', true);
query.greaterThan('published_at', cutoffDate);
query.descending('published_at');
query.limit(20);

const articles = await query.find();    // ONE page only (default ~30 items)
const all      = await query.findAll(); // ALL pages, materialized into an array
const first    = await query.first();   // single object or undefined
const article  = await new Nimbu.Query('articles').get(id);  // by id, rejects on miss
const total    = await query.count();
const coll     = await query.collection().fetch();   // Nimbu.Collection (paginated helpers)
```

**`.find()` only returns the first page.** This is the single most common cloud-code bug — you write `query.find()`, get back 30 items out of 500, and the rest silently disappear. Reach for `findAll()` (load everything), `collection().fetch()` (collection helpers), or one of the streaming forms below.

For large result sets, stream instead of loading everything into memory — and useful in jobs where you want to log progress:

```js
await query.each(async (entry) => { /* per-entry callback */ });
await query.eachBatch(async (batch) => { /* per-batch callback */ });
```

### Constraints

| Method | Purpose |
|--------|---------|
| `.equalTo(field, val)` / `.notEqualTo(field, val)` | Equality |
| `.greaterThan(field, val)` / `.lessThan(field, val)` | Range |
| `.greaterThanOrEqualTo(...)` / `.lessThanOrEqualTo(...)` | Inclusive range |
| `.containedIn(field, [vals])` / `.notContainedIn(field, [vals])` | Set membership |
| `.containsAll(field, [vals])` | Array contains all |
| `.exists(field)` / `.doesNotExist(field)` | Field presence |
| `.contains(field, str)` / `.startsWith(field, str)` / `.endsWith(field, str)` | Substring matching |
| `.matches(field, regex)` / `.search(field, str)` | Regex / full-text |
| `.geoWithin(...)` / `.geoIntersects(...)` / `.nearCoordinates(...)` | Geo |

### Sorting, pagination, projection

| Method | Purpose |
|--------|---------|
| `.ascending(field)` / `.descending(field)` | Primary sort |
| `.addAscending(field)` / `.addDescending(field)` | Secondary sort |
| `.limit(n)` | Items per page |
| `.page(n)` / `.per(n)` | Paginate (1-based page number) |
| `.include('relationField')` | Eager-load a relation in the same request |
| `.only(['title', 'slug'])` | Limit the fields returned |

### Composing queries

```js
const a = new Nimbu.Query('articles').equalTo('featured', true);
const b = new Nimbu.Query('articles').greaterThan('views', 1000);
const popularOrFeatured = Nimbu.Query.or(a, b);
const results = await popularOrFeatured.find();
```

### Customer / system-resource queries

`Nimbu.Customer`, `Nimbu.Product`, `Nimbu.Order`, `Nimbu.Page`, `Nimbu.Role` are all `Nimbu.Object` subclasses. Use the class form so the result is typed:

```js
const customer = await new Nimbu.Query(Nimbu.Customer)
  .equalTo('email', email.toLowerCase())
  .first();
```

The slug-string form (`new Nimbu.Query('customers')`) also works but returns plain `Nimbu.Object`.

Docs: https://docs.nimbu.io/docs/sdk/queries.md, https://docs.nimbu.io/docs/sdk/collections.md

## Objects

### Reading & mutating

```js
const article = await new Nimbu.Query('articles').get(id);

article.get('title');              // read
article.has('subtitle');           // existence check
article.set('archived', true);     // single write
article.set({ archived: true, archived_at: new Date() });  // batch write
article.unset('subtitle');         // remove field
article.increment('views', 1);     // atomic increment

await article.save();
await article.save({ status: 'archived' });   // shortcut: set + save
await article.destroy();                       // delete
```

### Creating new objects

Two equivalent patterns; both are common in real code:

```js
// Direct
const draft = new Nimbu.Object('articles', { title: 'Hello', body: '…' });
await draft.save();

// Subclass
const Article = Nimbu.Object.extend('articles');
const draft2 = new Article({ title: 'Hello' });
await draft2.save();

console.log(draft.id);   // populated after save
```

### Relations

Prefer eager-loading via the query — one round-trip:

```js
const article = await new Nimbu.Query('articles').include('author').get(id);
const author  = article.get('author');     // already materialized
console.log(author.get('email'));
```

Materialize an existing object's relation after the fact:

```js
await article.fetchWithInclude(['author', 'comments.author']);
// equivalent options form:
await article.fetch({ include: ['author', 'comments.author'] });
```

### Change tracking

Useful in `before` / `after` callbacks:

```js
if (article.hasChanged('status')) {
  const prev = article.previous('status');
  // …
}
const diff = article.changedAttributes();   // {field: newValue, ...} or false
const before = article.previousAttributes();
```

Docs: https://docs.nimbu.io/docs/sdk/objects.md, https://docs.nimbu.io/docs/sdk/relations-field-types.md

## Cloud functions

```js
Nimbu.Cloud.define('archiveArticle', async (request, response) => {
  const { id } = request.params;
  if (!request.customer) return response.error(401, 'auth required');

  const article = await new Nimbu.Query('articles').get(id);
  article.set('archived', true);
  await article.save();

  response.success({ id: article.id });
});
```

| `request` field | Meaning |
|-----------------|---------|
| `request.params` | Caller-supplied arguments (object) |
| `request.meta` | Metadata about the request context |
| `request.customer` | Authenticated customer, or `null` |

| `response` method | Use |
|-------------------|-----|
| `response.success(data)` | Return success payload (any JSON-serializable value) |
| `response.error('msg')` | Generic error |
| `response.error(code, 'msg')` | With HTTP-style status code (400/401/403/404/500/…) |

**Do not `throw`** — a thrown error becomes a generic 500 with no useful payload. Always go through `response.error(...)`.

Invoke a function from elsewhere in cloud code with `Nimbu.Cloud.run('name', params)`.

Docs: https://docs.nimbu.io/docs/cloud-code/functions.md, https://docs.nimbu.io/docs/sdk/api-cloud-functions.md

## Background jobs

```js
Nimbu.Cloud.job('reindex_articles', async (request, response) => {
  const articles = await new Nimbu.Query('articles').findAll();
  for (const article of articles) {
    article.set('search_blob', buildSearchBlob(article));
    await article.save();
  }
});
```

Job handlers receive `(request, response)`. `request.params` carries whatever was passed to `schedule(name, params, ...)`.

### Scheduling

```js
Nimbu.Cloud.schedule('reindex_articles', {}, {});                            // run now (also accepts null)
Nimbu.Cloud.schedule('send_reminder', { id }, { at: new Date('2026-12-01') });
Nimbu.Cloud.schedule('cleanup', {}, { in: '30m' });                           // s / m / h
Nimbu.Cloud.schedule('reindex_articles', {}, { every: '0 3 * * *' });        // cron — daily at 03:00
```

The third argument is **always an object** (or `null`), never a bare cron string. Cron is 5-field (minute hour day month weekday) and runs in UTC.

Trigger remotely with the CLI: `nimbu jobs run reindex_articles --site <slug>`.

Docs: https://docs.nimbu.io/docs/cloud-code/jobs.md

## Channel callbacks

Pre-save hook (can reject):

```js
Nimbu.Cloud.before('channel.entries.created', 'articles', async (request, response) => {
  const entry = request.object;
  if (!entry.get('title')) return response.error('title', 'is required');
  entry.set('slug', slugify(entry.get('title')));
  response.success();
});
```

Post-save hook (side effects only — return values ignored):

```js
Nimbu.Cloud.after('channel.entries.created', 'articles', async (request) => {
  const entry = request.object;
  await Mail.send({
    to: 'editorial@example.com',
    subject: `New article: ${entry.get('title')}`,
    html: `<p>Just published: ${entry.get('title')}</p>`,
  });
});
```

### Events

| Resource | Events |
|----------|--------|
| Channel entries | `channel.entries.created`, `channel.entries.updated`, `channel.entries.deleted` |
| Customers | `customer.created`, `customer.updated`, `customer.deleted` |
| Products | `product.created`, `product.updated`, `product.deleted` |
| Coupons | `coupon.created`, `coupon.updated`, `coupon.deleted` |
| Devices | `device.created`, `device.updated`, `device.deleted` |
| Orders (after-only) | `order.created`, `order.updated`, `order.canceled`, `order.fulfilled`, `order.paid`, `order.reopened`, `order.attachments.ready`, `order.attachments.expired` |

For non-channel resources, omit the channel-slug arg: `Nimbu.Cloud.after('order.paid', handler)`.

The event name is **`deleted`**, not `destroyed` — even though the method that triggers it is `.destroy()`.

### Request fields

| Field | Meaning |
|-------|---------|
| `request.object` | The `Nimbu.Object` being created/updated/deleted |
| `request.actor` | The customer or admin user performing the action |
| `request.customer` | Alias for `request.actor` when actor is a customer |
| `request.user` | Backend admin user (when triggered from admin UI) |
| `request.changes` | Map of field changes (updates/deletes only) |
| `request.lastUpdatedAt` | Previous `updated_at` timestamp (updates only) |

### Response

- `response.success()` — allow the operation to proceed.
- `response.error('message')` — reject with a general error.
- `response.error('fieldName', 'message')` — reject with a field-level validation error (rendered next to the field in the admin UI).

Docs: https://docs.nimbu.io/docs/cloud-code/callbacks.md

## HTTP routes

Mount custom HTTP endpoints on the site under `/cloud/routes/...`:

```js
Nimbu.Cloud.get('/api/articles/:id', async (request, response) => {
  const article = await new Nimbu.Query('articles').get(request.params.id);
  response.json({ id: article.id, title: article.get('title') });
});

Nimbu.Cloud.post('/webhooks/example', async (request, response) => {
  const sig = request.headers['x-example-signature'];
  if (!verify(sig, request.body)) return response.error(401, 'bad signature');
  // …handle…
  response.success();
});
```

Methods: `Nimbu.Cloud.get/post/put/patch/delete`, plus `Nimbu.Cloud.route(method, path, handler)` for the generic form.

Path patterns: `/static`, `/dynamic/:id`, `/multi/:a/:b`, `/wildcard/*rest`.

| `request` field | Meaning |
|-----------------|---------|
| `request.params` | URL params + query string + body, merged |
| `request.path` / `request.host` / `request.locale` | Request metadata |
| `request.headers` | HTTP headers (accessed by lowercase key in real code) |
| `request.body` | Raw request body string |
| `request.customer` | Authenticated customer, or `null` |
| `request.session` | Session data accessor |

| `response` method | Use |
|-------------------|-----|
| `response.json(obj)` | Send JSON |
| `response.html(str)` | Send HTML |
| `response.redirect_to(url)` | HTTP redirect |
| `response.render(template, vars)` | Render a Liquid template |
| `response.send(fileData, opts)` | Send a file (with `type`, `filename`) |
| `response.success()` / `response.error('msg')` / `response.error(code, 'msg')` | Plain responses |

Docs: https://docs.nimbu.io/docs/cloud-code/routes.md

## Admin extensions

Add custom actions to the Nimbu admin UI:

```js
Nimbu.Cloud.extend('channel.entries.show', 'orders', {
  name: 'Resend confirmation',
  action: async (request, response) => {
    const order = request.object;
    await Mail.send({ /* … */ });
    response.success('Confirmation resent.');
  },
});
```

Common actions: `'channel.entries.show'`, `'channel.entries.list'`. The handler shape mirrors callbacks. Use sparingly — anything more than a one-shot button is usually better as a function or job.

Docs: https://docs.nimbu.io/docs/cloud-code/extensions.md

## Customers & sessions

### Querying

```js
const customer = await new Nimbu.Query(Nimbu.Customer).get(customerId);
const byEmail  = await new Nimbu.Query(Nimbu.Customer)
  .equalTo('email', email.toLowerCase())
  .first();
```

### Inside handlers

```js
const customer = request.customer;        // null if unauthenticated
const email    = customer.get('email');
const roles    = customer.get('roles');   // array of role slugs

if (!roles.includes('admin')) return response.error(403, 'admin only');
```

### Sign-up / sign-in / sessions (less common in cloud code, but available)

```js
await Nimbu.Customer.signUp('ada@example.com', 'secret', { firstname: 'Ada', lastname: 'Lovelace' });
await Nimbu.Customer.logIn('ada@example.com', 'secret');
await Nimbu.Customer.become(sessionToken, { useACL: true });
await Nimbu.Customer.requestPasswordReset('ada@example.com', { useACL: true });
await Nimbu.Customer.logOut();

const current = Nimbu.Customer.current();   // current session, if any
if (current && current.authenticated()) { /* … */ }
```

Docs: https://docs.nimbu.io/docs/sdk/customers-sessions.md

## Mail

```js
await Mail.send({
  to: 'jane@example.com',
  cc: ['team@example.com'],
  subject: 'Welcome',
  html: '<p>Welcome aboard.</p>',
  text: 'Welcome aboard.',
});
```

Templating beyond inline HTML belongs in Nimbu **notification templates** (managed via `nimbu notifications` / `nimbu mails` in the companion CLI skill).

Docs: https://docs.nimbu.io/docs/cloud-code/modules.md

## HTTP

```js
const res = await HTTP.get('https://api.example.com/items', {
  headers: { Authorization: `Bearer ${Nimbu.Site.env.get('EXAMPLE_KEY')}` },
});
const items = JSON.parse(res.body);

await HTTP.post(
  'https://api.example.com/webhook',
  { event: 'archived', id: article.id },
  { headers: { 'Content-Type': 'application/json' } },
);
```

Docs: https://docs.nimbu.io/docs/cloud-code/modules.md

## Site & env

```js
Nimbu.Site.subdomain                  // 'my-site' or 'my-site-staging'
Nimbu.Site.env.get('STRIPE_KEY')      // site-level env var, undefined if unset
```

Set env vars with `nimbu apps config --site <slug> KEY=value` (companion CLI skill) **before** the code that reads them is deployed.

Docs: https://docs.nimbu.io/docs/sdk/system-resources.md, https://docs.nimbu.io/docs/sdk/environments.md

## System resources

`Nimbu.Site`, `Nimbu.Customer`, `Nimbu.Order`, `Nimbu.Product`, `Nimbu.Page`, `Nimbu.Role` — specialized classes that follow the same `.get` / `.set` / `.save` shape as `Nimbu.Object`. Use them in queries when the site uses commerce or page resources directly.

Docs: https://docs.nimbu.io/docs/sdk/system-resources.md

## Reference

- Full SDK reference: https://docs.nimbu.io/docs/sdk/reference.md
- Recipes: https://docs.nimbu.io/docs/sdk/recipes.md
