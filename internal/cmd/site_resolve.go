package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/config"
)

// Site sources, as reported by `sites current`.
const (
	siteSourceFlag        = "flag"        // --site or NIMBU_SITE
	siteSourceEnvironment = "environment" // --env selected a named project environment
	siteSourceProject     = "project"     // site in nimbu.yml
	siteSourceConfig      = "config"      // default_site in the global config
)

// siteResolution is the site a command targets and where it came from.
type siteResolution struct {
	Site   string
	Source string
	// ProjectErr is the nimbu.yml lookup error when no project file supplied
	// the site; it describes where the lookup searched.
	ProjectErr error
}

// readProjectSite reads the site from nimbu.yml; swapped in tests.
var readProjectSite = func() (string, error) {
	proj, err := config.ReadProjectConfig()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(proj.Site), nil
}

// resolveSite picks the target site. Precedence, highest first:
//
//  1. --env <name>: the named project environment sets site and API URL together
//  2. --site / NIMBU_SITE
//  3. site in the nearest nimbu.yml
//  4. default_site in the global config
//
// The project file beats the global default so a write run inside a project
// never lands on whatever site was last made the global default.
func resolveSite(flags *RootFlags, cfg *config.Config) siteResolution {
	if flags != nil {
		if site := strings.TrimSpace(flags.Site); site != "" {
			source := siteSourceFlag
			if flags.Environment != "" {
				source = siteSourceEnvironment
			}
			return siteResolution{Site: site, Source: source}
		}
	}

	projectSite, projErr := readProjectSite()
	if projErr == nil && projectSite != "" {
		return siteResolution{Site: projectSite, Source: siteSourceProject}
	}
	if projErr != nil && !errors.Is(projErr, config.ErrNotFound) {
		// A nimbu.yml exists but cannot be read: falling back to the global
		// default would silently target a different site.
		return siteResolution{ProjectErr: projErr}
	}
	if projErr == nil {
		projErr = errProjectHasNoSite
	}

	if cfg != nil {
		if site := strings.TrimSpace(cfg.DefaultSite); site != "" {
			return siteResolution{Site: site, Source: siteSourceConfig}
		}
	}
	return siteResolution{ProjectErr: projErr}
}

var errProjectHasNoSite = errors.New("nimbu.yml has no site")

// siteRequiredError is the usage error for a command that needs a site and
// found none in any source.
func siteRequiredError(projErr error) error {
	if projErr != nil && !errors.Is(projErr, config.ErrNotFound) && !errors.Is(projErr, errProjectHasNoSite) {
		return newHintedError(
			fmt.Errorf("cannot read the site from %s: %w", config.ProjectFileName, projErr),
			errorSiteRequired, ExitUsage,
			fmt.Sprintf("fix %s, or pass --site <site> to choose the site explicitly", config.ProjectFileName),
		)
	}
	lookup := config.ProjectFileName
	if !errors.Is(projErr, errProjectHasNoSite) {
		lookup = config.DescribeProjectLookup(projErr)
	}
	return newHintedError(
		errors.New("no site selected"),
		errorSiteRequired, ExitUsage,
		fmt.Sprintf("pass --site <site>, set NIMBU_SITE, add site: to %s, or set default_site with `nimbu config set --key default_site --value <site>`", lookup),
	)
}
