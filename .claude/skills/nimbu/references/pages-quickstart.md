# Pages Quickstart

Read this instead of the full page reference; every line below is a working invocation. Details, edge cases and the FileRef table live in [pages-menus-content.md](pages-menus-content.md).

Set the site once per session (`export NIMBU_SITE=my-site`) so nothing depends on the CWD.

## Read

```bash
nimbu pages get --page about/team --outline              # ids, paths, content preview
nimbu pages get --page about/team --outline --locale en  # the same outline, EN content
nimbu pages items --page about/team --path 'Blokken[0]'  # one block, readable HTML
nimbu pages schema --page about/team --json              # only when you need select options
```

`--outline` prints page fields first, then one line per repeatable and editable:

```
title (page): Our Team
Blokken[0] hero_stage 6a6d0123456789abcdef0001
  Blokken[0].Title (text): Bouw mee aan…
```

`--outline --json` gives a flat array: `path` (valid for `pages set --path`), `raw_path`, `id`, `slug`, `type`, and the untruncated `content`.

When you really need the document, trim it — never paste a raw `get --json` into context:

```bash
nimbu pages get --page about/team --json --compact --fields title,items   # ~3x smaller
nimbu pages get --page about/team --outline --draft   # every read flag works on the draft
nimbu pages draft get --page about/team --json        # same shape as get, plus `draft`
```

## Write

One field:

```bash
nimbu pages set --page about/team --path 'Blokken[0].Title' --dry-run --diff "Fast"
nimbu pages set --page about/team --path 'Blokken[0].Title' "Fast"
nimbu pages set --page about/team --path 'Blokken[0].Title' --locale en "Fast"
```

Several changes atomically (max 10 ops; `op` is `set`, `insert`, `delete` or `move`):

```json
{"operations":[
  {"op":"set","path":"Blokken[0].Title","value":"Hello"},
  {"op":"insert","path":"Blokken","after":null,
   "value":{"slug":"proof_strip","items":{"Quote":"Hi"}}},
  {"op":"move","path":"Blokken[1]","after":null},
  {"op":"delete","path":"Blokken[slug=proof_strip]"}
]}
```

```bash
nimbu pages batch --page about/team --file ops.json --dry-run   # then again with --diff
```

`after`: omit to append, `null` for first, or a sibling repeatable **id**. `insert` paths address the canvas (`Blokken`, `Blokken[id=<id>].Items`). Dry-run resolves exactly like the write.

QA loop — edit the draft, look at it, publish:

```bash
nimbu pages batch --page about/team --file ops.json --draft
nimbu pages draft preview-url --page about/team
nimbu pages draft publish --page about/team
```

## Path grammar

```
title | slug | seo_title | seo_description | seo_keywords | published | og_image | template
Blokken[2].Title              0-based index into a canvas
Blokken[id=6a6d…].Title       ObjectId selector (stable across reorders)
Blokken[slug=hero].Title      unique sibling slug, else the error lists candidates
Blokken[0].Photos[1].Image    nested canvas, two levels max
/items/Blokken/repeatables/<id>/items/Title   raw API path, passed verbatim
```

## Gotchas

1. `--locale en` writes **only** EN; the default (NL) content stays as it is. Reads project the locale too, including `--json`.
2. **Do not combine `--draft` with `--locale`** until the API fix lands — it reports ok but stores the NL text in `translations.en`. Write EN on the live page after `draft publish`.
3. `pages insert` creates the default-locale copy only. Add EN with a follow-up `pages set --locale en`.
4. EN slug: `nimbu pages update --page about/team --locale en --file <(echo '{"slug":"team"}')` or `translations:='{"en":{"slug":"team"}}'`. Never echo a fetched `translations` map back: fallbacks come back as real values.
5. Always pass `--site` (or export `NIMBU_SITE`) in subagent briefs; `nimbu.yml` is found by walking up to the git root, but a scratchpad CWD is outside it.
6. Run `nimbu commands --json` once for the whole flag contract instead of `--help` per verb.
7. Never dump `pages get --json` into context (270 KB for a landing page). Use `--outline`, `--compact`, or `pages items`.
8. Paths end at the editable name — never append `/content`. Repeatable slugs come from the theme (`{% repeatable 'row' %}` → `slug: "row"`), so confirm them in the outline instead of guessing `item`.
