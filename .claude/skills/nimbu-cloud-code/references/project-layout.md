# Project Layout

How a Nimbu Cloud Code project is typically structured on disk. Conventions, not rules — but agents should follow what's already in place rather than inventing new structures.

## The `code/` directory

Cloud-code source lives in the **`code/`** subdirectory of a Nimbu site repo, alongside the theme (`templates/`, `layouts/`, `assets/`, etc.).

`nimbu themes push` and `nimbu themes sync` **deliberately skip `code/`** — it is not a theme resource. Deploy with:

```bash
nimbu apps push                       # deploy everything in code/ (or dist/, if built)
nimbu apps push --only file.js        # deploy a single file
```

See the companion `nimbu` skill for full deployment, env, and site-management commands.

## File naming: one file per domain

Cloud code splits by **domain**, not by capability. Typical files an agent will see:

```
code/
├── articles.js          # callbacks + functions for the 'articles' channel
├── orders.js            # commerce-side hooks
├── customers.js         # customer lifecycle
├── helpers.js           # shared utilities
├── environment.js       # env detection + per-env config
└── package.json         # dev tooling only (not runtime — see below)
```

Each file registers its own `Nimbu.Cloud.define`, `Nimbu.Cloud.job`, `Nimbu.Cloud.before`, `Nimbu.Cloud.after`. There is no central router — registration is global, and the file just needs to load. Avoid renaming `index.js`-style top-level files unless you understand the load order.

## `.eslintrc.json` for runtime modules

Most cloud-code projects ship an `.eslintrc.json` declaring the built-in modules and any project-wide helpers as globals so lint stops complaining when files use them without `require`:

```json
{
  "env": { "node": true, "es2021": true },
  "globals": {
    "require": true,
    "Nimbu": true,
    "Mail": true,
    "HTTP": true,
    "crypto": true,
    "jwt": true,
    "I18n": true,
    "_": true,
    "moment": true,
    "DM": true,
    "Helpers": true
  }
}
```

`Nimbu` is the only true pre-injected global; the others (`Mail`, `HTTP`, `crypto`, `jwt`, `I18n`, `lodash`, `moment`) are typically `require`'d at the top of each file, but the eslint globals list lets either pattern coexist. Project-specific helper namespaces (`DM`, `Helpers`, etc.) are usually populated by a top-level file (e.g. `helpers.js` or `datamodel.coffee`) and exposed as globals so other files can use them without imports.

## Optional Makefile / CoffeeScript build

Some projects predate the all-JS era and use a `Makefile` to compile CoffeeScript to JS:

```make
default:
	yarn coffee -b -c -o dist *.coffee
	cp *.js dist/

deploy: default
	nimbu apps push
```

In that case, source is `*.coffee` + `*.js`, build output is `dist/`, and `nimbu apps push` is run from `dist/` (configured via `nimbu.yml` or wd convention). For new projects: stick to plain JS unless the rest of the project is CoffeeScript.

## `package.json` does NOT control the runtime

The Nimbu sandbox is **fixed** — only `Nimbu`, `Mail`, `HTTP`, `crypto`, `jwt`, `I18n` (and a few conventionally available modules like `lodash` and `moment`) are loadable inside cloud code. Adding a dependency to `package.json` does NOT make it available in the sandbox at runtime.

`package.json` exists purely for **dev tooling**:

- `@nimbu/testing` for Jest-based unit tests
- `eslint`, `prettier`, etc.
- Build tools (CoffeeScript compiler, etc.)

If an agent is tempted to `npm install` something for runtime use, stop and check the [Available Modules docs](https://docs.nimbu.io/docs/cloud-code/modules.md) first.

## Environment branching

Two patterns are common:

### Subdomain-based

```js
const isStaging = Nimbu.Site.subdomain.endsWith('-staging');
const apiBase = isStaging
  ? 'https://api.staging.example.com'
  : 'https://api.example.com';
```

### Env-var-based via `environment.js`

A small helper module that reads `Nimbu.Site.env.get(...)` and exports typed config:

```js
// environment.js
module.exports = {
  apiBase: Nimbu.Site.env.get('EXAMPLE_API_BASE'),
  apiKey:  Nimbu.Site.env.get('EXAMPLE_API_KEY'),
  isStaging: Nimbu.Site.subdomain.endsWith('-staging'),
};
```

Other files: `const env = require('./environment');`. Both patterns work — match what's already in the project.

Set env vars with the CLI:

```bash
nimbu apps config --site my-site EXAMPLE_API_KEY=...
```

## What NOT to do

- Don't put cloud-code logic inside `templates/`, `layouts/`, or `assets/` — those are theme territory and ship via `themes push`.
- Don't `require` modules that aren't in the [Available Modules](https://docs.nimbu.io/docs/cloud-code/modules.md) list (or already in use elsewhere in the project).
- Don't mix `async/await` and `Nimbu.Future.when().then().fail()` patterns inside one file — pick whichever the file already uses.
- Don't read env vars before they're set on the site; deploy the config first.

## Reference

- [Cloud Code overview](https://docs.nimbu.io/docs/cloud-code/overview.md)
- [Available modules](https://docs.nimbu.io/docs/cloud-code/modules.md)
- [Runtime API](https://docs.nimbu.io/docs/cloud-code/runtime-api.md)
- Companion `nimbu` skill — `nimbu apps push`, `nimbu apps config`, `themes push/sync` semantics
