package cmd

// SitesCmd manages sites.
type SitesCmd struct {
	List    SitesListCmd    `cmd:"" help:"List accessible sites"`
	Get     SitesGetCmd     `cmd:"" help:"Get site details"`
	Current SitesCurrentCmd `cmd:"" help:"Show current site context"`
}
