package cmd

// SitesCmd manages sites.
type SitesCmd struct {
	List     SitesListCmd     `cmd:"" help:"List accessible sites"`
	Get      SitesGetCmd      `cmd:"" help:"Get site details"`
	Current  SitesCurrentCmd  `cmd:"" help:"Show current site context"`
	Count    SitesCountCmd    `cmd:"" help:"Count accessible sites"`
	Settings SitesSettingsCmd `cmd:"" help:"Get site settings"`
}
