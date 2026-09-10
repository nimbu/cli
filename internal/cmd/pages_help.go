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
`
}
