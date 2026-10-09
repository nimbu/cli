package cmd

func menuItemsHelp() string {
	return `
Menu items (--file JSON, nested via children):

  name          label (alias: title)
  kind          url | page | channel; required by the server
  position      order among siblings
  target_blank  open in a new tab (bool)
  children      nested items

Fields per kind:

  url      target_url (alias: url), e.g. "/contact", "#visie", "https://..."
  page     target_fullpath, the page fullpath (e.g. "about/team")
  channel  links a channel entry; its target is set in the Nimbu admin, so
           keep kind and id from menus get to preserve these items

An empty kind is inferred: target_fullpath gives page, target_url or url gives
url. Any other kind fails locally with the item path (items[1].children[0].kind)
and nothing is sent.
`
}

func (c *MenusCreateCmd) Help() string {
	return menuItemsHelp()
}

func (c *MenusUpdateCmd) Help() string {
	return menuItemsHelp() + `
update --file replaces the whole item tree. The CLI reads the current menu,
keeps items whose id appears in the file, deletes every other existing item,
creates items without an id, and sets position from array order. Start from
menus get --menu <slug> --json and keep the ids to edit items in place.

If the server returns menu items without an id, update --file stops before
writing: the old items could not be removed and would be duplicated.

Inline assignments (name, handle, translations) never touch items.
`
}
