package cmd

// DomainsCmd manages custom domains.
type DomainsCmd struct {
	List        DomainsListCmd        `cmd:"" help:"List domains"`
	Get         DomainsGetCmd         `cmd:"" help:"Get domain details"`
	Create      DomainsCreateCmd      `cmd:"" help:"Create a domain"`
	Update      DomainsUpdateCmd      `cmd:"" help:"Update a domain"`
	Delete      DomainsDeleteCmd      `cmd:"" help:"Delete a domain"`
	MakePrimary DomainsMakePrimaryCmd `cmd:"" help:"Make a domain primary"`
}
