package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

var bsonObjectIDRE = regexp.MustCompile(`^[0-9a-fA-F]{24}$`)

func requireExactConfirm(confirm, expected, target string) error {
	confirm = strings.TrimSpace(confirm)
	expected = strings.TrimSpace(expected)
	if confirm == "" {
		return fmt.Errorf("use --confirm %q to confirm %s", expected, target)
	}
	if confirm != expected {
		return fmt.Errorf("--confirm must exactly match %q to confirm %s", expected, target)
	}
	return nil
}

func looksLikeDomainRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	return strings.Contains(ref, ".")
}

func looksLikeObjectID(ref string) bool {
	return bsonObjectIDRE.MatchString(strings.TrimSpace(ref))
}

func resolveDomainRef(ctx context.Context, client *api.Client, ref string) (api.Domain, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return api.Domain{}, &api.Error{StatusCode: http.StatusNotFound, Message: "domain reference is required"}
	}

	if looksLikeDomainRef(ref) {
		return findDomainByName(ctx, client, ref)
	}

	var domain api.Domain
	path := "/domains/" + url.PathEscape(ref)
	if err := client.Get(ctx, path, &domain); err == nil {
		return domain, nil
	} else {
		var apiErr *api.Error
		if !strings.Contains(ref, ".") || !errorAsNotFound(err, &apiErr) {
			return api.Domain{}, err
		}
	}

	return findDomainByName(ctx, client, ref)
}

func findDomainByName(ctx context.Context, client *api.Client, name string) (api.Domain, error) {
	domains, err := api.List[api.Domain](ctx, client, "/domains")
	if err != nil {
		return api.Domain{}, err
	}
	for _, domain := range domains {
		if strings.EqualFold(domain.Domain, name) {
			return domain, nil
		}
	}
	return api.Domain{}, &api.Error{StatusCode: http.StatusNotFound, Message: fmt.Sprintf("domain %q not found", name)}
}

func resolveSenderRef(ctx context.Context, client *api.Client, ref string) (api.SenderDomain, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return api.SenderDomain{}, &api.Error{StatusCode: http.StatusNotFound, Message: "sender reference is required"}
	}

	if looksLikeDomainRef(ref) {
		return findSenderByDomain(ctx, client, ref)
	}

	var sender api.SenderDomain
	path := "/senders/" + url.PathEscape(ref)
	if err := client.Get(ctx, path, &sender); err == nil {
		return sender, nil
	} else {
		var apiErr *api.Error
		if !errorAsNotFound(err, &apiErr) {
			return api.SenderDomain{}, err
		}
	}

	return findSenderByDomain(ctx, client, ref)
}

func findSenderByDomain(ctx context.Context, client *api.Client, domainName string) (api.SenderDomain, error) {
	senders, err := api.List[api.SenderDomain](ctx, client, "/senders")
	if err != nil {
		return api.SenderDomain{}, err
	}
	for _, sender := range senders {
		if strings.EqualFold(sender.Domain, domainName) {
			return sender, nil
		}
	}
	return api.SenderDomain{}, &api.Error{StatusCode: http.StatusNotFound, Message: fmt.Sprintf("sender domain %q not found", domainName)}
}

func errorAsNotFound(err error, target **api.Error) bool {
	if err == nil {
		return false
	}
	var apiErr *api.Error
	if ok := errors.As(err, &apiErr); ok && apiErr != nil && apiErr.IsNotFound() {
		if target != nil {
			*target = apiErr
		}
		return true
	}
	return false
}

func validateAllowedTopLevelKeys(body map[string]any, allowed map[string]struct{}, command string) error {
	for key := range body {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("%s: unknown field %q", command, key)
		}
	}
	return nil
}
