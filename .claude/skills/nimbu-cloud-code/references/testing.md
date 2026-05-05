# Testing Cloud Code

Cloud-code projects test with **Jest** + **`@nimbu/testing`** — Zenjoy's in-house package that loads cloud-code source files in-process, mocks the SDK and the Nimbu HTTP API, and lets handlers run against fixtures.

> `@nimbu/testing` is not on `docs.nimbu.io`. The patterns below are taken from real Zenjoy theme repos (current as of `@nimbu/testing` `^0.2.x`). Confirm exact exports against `node_modules/@nimbu/testing` for the project at hand.

## Layout

Both layouts are common:

```
code/
├── articles.js
├── articles.spec.js                # alongside source
└── orders.js
```

```
code/
├── articles.js
└── test/
    ├── articles.spec.js            # in a test/ subdirectory
    └── fixtures/
        ├── customer.json
        └── articles.json
```

`package.json` (dev tooling only — see [project-layout.md](project-layout.md)):

```json
{
  "scripts": { "test": "jest" },
  "devDependencies": {
    "@nimbu/testing": "^0.2.2",
    "jest": "^29",
    "babel-jest": "^29"
  }
}
```

## Bootstrap

```js
const fs = require('fs');
const path = require('path');
const {
  mockAPI,
  setup,
  mockRequest,
  CloudCodeHandleType,
  getCloudFunctionHandler,
  getBeforeCallbackHandler,
  customerFromFixture,
  objectFromFixture,
} = require('@nimbu/testing');

jest.useFakeTimers().setSystemTime(new Date('2021-01-01').getTime());

setup().catch((error) => {
  console.error('Error:', error);
});

require('../articles.js');   // side-effect import: registers Cloud.define / before / after
```

`setup()` returns a Promise — handle the rejection or tests can hang. Side-effect-import the source files you're testing so their `Nimbu.Cloud.*` calls register handlers before the tests run.

## Helpers

| Export | Purpose |
|--------|---------|
| `setup()` | Initialize the in-process Nimbu sandbox. Call once per file. Returns a Promise. |
| `mockAPI` | Mock the Nimbu HTTP API. Use `mockAPI.get(url, body)` etc. to register expected calls. |
| `mockRequest(handleType, options?)` | Build a `{request, response}` pair. `handleType` is `CloudCodeHandleType.Function`/`Callback`/`Job`. |
| `CloudCodeHandleType` | Enum: `Function`, `Callback`, `Job`. |
| `getCloudFunctionHandler(name)` | Retrieve the function registered with `Nimbu.Cloud.define(name, ...)`. |
| `getBeforeCallbackHandler({ event, slug })` | Retrieve a `Nimbu.Cloud.before(event, slug, ...)` handler. |
| `customerFromFixture(data)` | Wrap a JSON object as a `Nimbu.Customer` instance. |
| `objectFromFixture(slug, data)` | Wrap a JSON object as a `Nimbu.Object` for `slug`. |

`response.success` and `response.error` are **jest spies** on the returned `response` — assert with `toHaveBeenCalledWith(...)`.

## Mocking the Nimbu API

```js
mockAPI.get(
  'https://api.nimbu.io/channels/articles/entries',
  fs.readFileSync(path.resolve(__dirname, './fixtures/articles.json')),
);

// …run handler…

expect(mockAPI).toBeDone();   // assert all registered mocks were consumed
```

`mockAPI.get` / `.post` / `.put` / `.delete` register expected calls. Body argument can be a JSON object, a string, or a `Buffer` from `fs.readFileSync`. Fail the test if a registered URL is never hit (`toBeDone()`) so stale mocks don't pile up.

The exact URL matters — including query string and ordering. When in doubt, run the test once, copy the URL out of the failure message, paste it into the mock.

## Testing a cloud function

```js
describe('#archive_article', () => {
  const customerData = JSON.parse(fs.readFileSync(path.resolve(__dirname, './fixtures/customer.json')));
  const handler = getCloudFunctionHandler('archive_article');

  test('rejects unauthenticated callers', () => {
    const { request, response } = mockRequest(CloudCodeHandleType.Function);

    handler(request, response);

    expect(response.error).toHaveBeenCalledWith(401, 'Unauthorized');
  });

  test('archives an article for an admin', async () => {
    mockAPI.get('https://api.nimbu.io/channels/articles/entries/abc',
      fs.readFileSync(path.resolve(__dirname, './fixtures/article.json')));

    const { request, response } = mockRequest(CloudCodeHandleType.Function, {
      params: { id: 'abc' },
      customer: customerFromFixture({ ...customerData, roles: ['admin'] }),
    });

    await handler(request, response).asPromise();

    expect(mockAPI).toBeDone();
    expect(response.error).not.toHaveBeenCalled();
    expect(response.success).toHaveBeenCalledWith({ id: 'abc', archived: true });
  });
});
```

Key shapes to remember:

- `mockRequest(...)` returns `{ request, response }` — pass **both** into the handler.
- `await handler(request, response).asPromise()` is the idiomatic way to await an async handler returning a Nimbu future/promise.
- Assertions go on the `response` spies — not on a return value.

## Testing a before-callback

```js
describe('articles before-create', () => {
  const slug = 'articles';

  test('is registered', () => {
    expect(getBeforeCallbackHandler({ event: 'channel.entries.created', slug })).toBeDefined();
  });

  test('rejects entries without a title', () => {
    const object   = objectFromFixture(slug, { title: '' });
    const customer = customerFromFixture({ id: 'cust-1', roles: ['admin'] });

    const { request, response } = mockRequest(CloudCodeHandleType.Callback, { object, customer });

    validateArticleTitle(request, response);   // imported from ../articles.js

    expect(response.error).toHaveBeenCalled();
    expect(response.success).not.toHaveBeenCalled();
  });
});
```

For after-callbacks, retrieve the handler with the equivalent helper if your `@nimbu/testing` version ships one, or import the function directly from the source file.

## Testing a job

```js
const handler = /* import job handler from source, or use a lookup helper */;
const { request } = mockRequest(CloudCodeHandleType.Job, { params: { id: 'abc' } });

await handler(request).asPromise();
```

Cron scheduling itself is not exercised in unit tests — call the handler directly.

## Running

```bash
yarn test                 # jest, all tests
yarn test articles        # filter by filename
yarn test -- --coverage   # with coverage
```

## What you can't test in-process

- **Real channel field validation**: `mockAPI` returns whatever you tell it to, so server-side schema rejection won't fire. Validate in the function itself, not just in tests.
- **Cron scheduling**: not exercised; call the handler directly.
- **Email and HTTP side effects**: stub them; they aren't sent for real.

For end-to-end validation, deploy to a staging site (`nimbu apps push --site my-site-staging`) and exercise via `nimbu functions run` / `nimbu jobs run`.

## Reference

- Real test files in Zenjoy theme repos are the canonical reference — read the actual `@nimbu/testing` source under `node_modules/@nimbu/testing/` if a helper isn't documented.
- [Cloud Code Functions](https://docs.nimbu.io/cloud-code/functions.md) — handler return shape.
- Companion `nimbu` skill — `nimbu functions run`, `nimbu jobs run` for staging verification.
