package cmd

func pagesAssignmentHelp() string {
	return `
Supported top-level fields:

  title, slug, template, parent
  published (bool, published:=true)
  seo_title, seo_description, seo_keywords
  security_mechanism (none|humans|customers)

Use key=value for strings and key:=json for typed values. --file and inline
assignments are mutually exclusive.
`
}

func (c *PagesCreateCmd) Help() string {
	return pagesAssignmentHelp() + `
Create accepts these fields via --file or inline assignments.
`
}

func (c *PagesUpdateCmd) Help() string {
	return pagesAssignmentHelp() + `
Update --file accepts these fields. Inline assignments are limited to title,
template, published, locale, and a complete top-level translations object.

Only the fields you pass are sent; the API merges them. The fetched document
is never written back: its top-level fields and translations map are
locale-resolved with fallbacks, so echoing them would copy default-locale
text into other locales and revert localized slugs. fullpath, public_url,
and depth are server-derived and are always stripped from --file input.

To set a localized slug, send a minimal document for that locale:

  echo '{"slug":"about-us"}' | nimbu pages update --page over-ons --locale en --file -

or an explicit translations block: translations:='{"en":{"slug":"about-us"}}'.
`
}

func (c *PagesBatchCmd) Help() string {
	return `
--file takes {"operations":[...]} or a bare array, max 10 operations.

Ops and required keys:

  set     path, value    replace an editable or page field value
  insert  path, value    add a repeatable; value is {slug, items}
  delete  path           remove a repeatable
  move    path, after    reorder a repeatable

after (insert and move): omit it to append, null to place first, or a
sibling repeatable id to place right after that sibling.

Paths are human (Blokken[0].Title, Blokken[id=<id>].Items) or raw
(/items/Blokken/repeatables/<id>/items/Title). They end at the editable
name; never append /content. insert paths address the canvas itself:
Blokken, Blokken[id=<id>].Items, or /items/Blokken/repeatables. A raw
path may use a position where an id belongs; it is rewritten to the id.
Raw paths are checked against the page before posting; shortened id paths
are expanded to the full form.

Example ops.json:

  {"operations":[
    {"op":"set","path":"Blokken[0].Title","value":"Hello"},
    {"op":"insert","path":"Blokken","after":null,
     "value":{"slug":"proof_strip","items":{"Quote":"Hi"}}},
    {"op":"move","path":"Blokken[1]","after":null},
    {"op":"delete","path":"Blokken[slug=proof_strip]"}
  ]}

The bare array form is the same list without the wrapper:

  [{"op":"set","path":"title","value":"About"}]
`
}
