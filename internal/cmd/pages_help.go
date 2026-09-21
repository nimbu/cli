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
