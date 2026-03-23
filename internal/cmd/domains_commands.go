package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

var domainMutationFields = map[string]struct{}{
	"domain":          {},
	"redirect_domain": {},
	"default_locale":  {},
	"default_country": {},
}

type DomainsListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

func (c *DomainsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}

	var domains []api.Domain
	var meta listFooterMeta
	if c.All {
		domains, err = api.List[api.Domain](ctx, client, "/domains", opts...)
		if err != nil {
			return fmt.Errorf("list domains: %w", err)
		}
		meta = allListFooterMeta(len(domains))
	} else {
		paged, err := api.ListPage[api.Domain](ctx, client, "/domains", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list domains: %w", err)
		}
		domains = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(domains))
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, domains)
	}
	plainFields := []string{"id", "domain", "primary", "dns_check", "ssl_enabled"}
	tableFields := plainFields
	tableHeaders := []string{"ID", "DOMAIN", "PRIMARY", "DNS_CHECK", "SSL_ENABLED"}
	if mode.Plain {
		return output.PlainFromSlice(ctx, domains, listOutputFields(flags, plainFields))
	}
	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, domains, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "domains", meta)
}

type DomainsGetCmd struct {
	Domain string `arg:"" help:"Domain ID or hostname"`
}

func (c *DomainsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	domain, err := resolveDomainRef(ctx, client, c.Domain)
	if err != nil {
		return fmt.Errorf("get domain: %w", err)
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, domain)
	}
	if mode.Plain {
		return output.Plain(ctx, domain.ID, domain.Domain, domain.Primary, domain.DNSCheck, domain.SSLEnabled)
	}
	fmt.Printf("ID:          %s\n", domain.ID)
	fmt.Printf("Domain:      %s\n", domain.Domain)
	fmt.Printf("Primary:     %t\n", domain.Primary)
	fmt.Printf("DNS Check:   %t\n", domain.DNSCheck)
	fmt.Printf("SSL Enabled: %t\n", domain.SSLEnabled)
	if domain.RedirectDomain != "" {
		fmt.Printf("Redirect:    %s\n", domain.RedirectDomain)
	}
	if domain.DefaultLocale != "" {
		fmt.Printf("Locale:      %s\n", domain.DefaultLocale)
	}
	if domain.DefaultCountry != "" {
		fmt.Printf("Country:     %s\n", domain.DefaultCountry)
	}
	return nil
}

type DomainsCreateCmd struct {
	Domain      string   `arg:"" help:"Hostname"`
	File        string   `help:"Read domain JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. redirect_domain=www.example.com, default_locale=nl)"`
}

func (c *DomainsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create domain"); err != nil {
		return err
	}
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	body, err := readJSONBodyInput(c.File, c.Assignments)
	if err != nil {
		if !errors.Is(err, errNoJSONInput) {
			return err
		}
		body = map[string]any{}
	}
	body, err = mergeJSONBodies(body, map[string]any{"domain": c.Domain})
	if err != nil {
		return fmt.Errorf("merge domain value: %w", err)
	}
	if err := validateAllowedTopLevelKeys(body, domainMutationFields, "create domain"); err != nil {
		return err
	}
	var domain api.Domain
	if err := client.Post(ctx, "/domains", body, &domain); err != nil {
		return fmt.Errorf("create domain: %w", err)
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, domain)
	}
	if mode.Plain {
		return output.Plain(ctx, domain.ID, domain.Domain)
	}
	fmt.Printf("Created domain %s (%s)\n", domain.Domain, domain.ID)
	return nil
}

type DomainsUpdateCmd struct {
	Domain      string   `arg:"" help:"Domain ID or hostname"`
	File        string   `help:"Read domain JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. redirect_domain=www.example.com, default_locale=nl)"`
}

func (c *DomainsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update domain"); err != nil {
		return err
	}
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	domain, err := resolveDomainRef(ctx, client, c.Domain)
	if err != nil {
		return fmt.Errorf("resolve domain: %w", err)
	}
	body, err := readJSONBodyInput(c.File, c.Assignments)
	if err != nil {
		return err
	}
	if err := validateAllowedTopLevelKeys(body, domainMutationFields, "update domain"); err != nil {
		return err
	}
	var updated api.Domain
	path := "/domains/" + url.PathEscape(domain.ID)
	if err := client.Put(ctx, path, body, &updated); err != nil {
		return fmt.Errorf("update domain: %w", err)
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, updated)
	}
	if mode.Plain {
		return output.Plain(ctx, updated.ID, updated.Domain)
	}
	fmt.Printf("Updated domain %s (%s)\n", updated.Domain, updated.ID)
	return nil
}

type DomainsDeleteCmd struct {
	Domain string `arg:"" help:"Domain ID or hostname"`
}

func (c *DomainsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete domain"); err != nil {
		return err
	}
	if err := requireForce(flags, "domain "+c.Domain); err != nil {
		return err
	}
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	domain, err := resolveDomainRef(ctx, client, c.Domain)
	if err != nil {
		return fmt.Errorf("resolve domain: %w", err)
	}
	path := "/domains/" + url.PathEscape(domain.ID)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete domain: %w", err)
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("deleted"))
	}
	if mode.Plain {
		return output.Plain(ctx, domain.ID, "deleted")
	}
	fmt.Printf("Deleted domain %s\n", domain.Domain)
	return nil
}

type DomainsMakePrimaryCmd struct {
	Domain string `arg:"" help:"Domain ID or hostname"`
}

func (c *DomainsMakePrimaryCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "make domain primary"); err != nil {
		return err
	}
	if err := requireForce(flags, "domain "+c.Domain); err != nil {
		return err
	}
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	domain, err := resolveDomainRef(ctx, client, c.Domain)
	if err != nil {
		return fmt.Errorf("resolve domain: %w", err)
	}
	var result api.ActionStatus
	path := "/domains/" + url.PathEscape(domain.ID) + "/make_primary"
	if err := client.Post(ctx, path, map[string]any{}, &result); err != nil {
		return fmt.Errorf("make domain primary: %w", err)
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		return output.Plain(ctx, domain.ID, result.Status, result.Primary)
	}
	fmt.Printf("Made domain primary: %s\n", domain.Domain)
	return nil
}
