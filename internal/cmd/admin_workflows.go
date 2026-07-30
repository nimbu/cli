package cmd

import (
	"context"
	"net/http"
)

type AnnouncementsCmd struct {
	List   AnnouncementsListCmd   `cmd:"" help:"List announcements"`
	Get    AnnouncementsGetCmd    `cmd:"" help:"Get an announcement"`
	Create AnnouncementsCreateCmd `cmd:"" help:"Create an announcement"`
	Update AnnouncementsUpdateCmd `cmd:"" help:"Update an announcement"`
	Delete AnnouncementsDeleteCmd `cmd:"" help:"Delete an announcement (requires --force)"`
}

type AnnouncementsListCmd struct {
	QueryFlags `embed:""`
	All        bool `help:"Fetch all pages"`
	Page       int  `default:"1" help:"Page number"`
	PerPage    int  `default:"25" help:"Items per page"`
}

func (c *AnnouncementsListCmd) Run(ctx context.Context) error {
	return auditedGlobalList(ctx, "/announcements", &c.QueryFlags, c.All, c.Page, c.PerPage)
}

type AnnouncementsGetCmd struct {
	ID string `required:"" help:"Announcement ID"`
}

func (c *AnnouncementsGetCmd) Run(ctx context.Context) error {
	return auditedGlobalGet(ctx, "/announcements"+escaped(c.ID))
}

type AnnouncementsCreateCmd struct {
	File        string   `help:"Read JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline announcement field assignments"`
}

func (c *AnnouncementsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	return auditedGlobalWrite(ctx, flags, http.MethodPost, "/announcements", c.File, c.Assignments)
}

type AnnouncementsUpdateCmd struct {
	ID          string   `required:"" help:"Announcement ID"`
	File        string   `help:"Read JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline announcement field assignments"`
}

func (c *AnnouncementsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	return auditedGlobalWrite(ctx, flags, http.MethodPatch, "/announcements"+escaped(c.ID), c.File, c.Assignments)
}

type AnnouncementsDeleteCmd struct {
	ID string `required:"" help:"Announcement ID"`
}

func (c *AnnouncementsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return auditedGlobalDelete(ctx, flags, "/announcements"+escaped(c.ID))
}

type DomainRegistrationsCmd struct {
	List   DomainRegistrationsListCmd   `cmd:"" help:"List domain registrations"`
	Get    DomainRegistrationsGetCmd    `cmd:"" help:"Get a domain registration"`
	Count  DomainRegistrationsCountCmd  `cmd:"" help:"Count domain registrations"`
	Upsert DomainRegistrationsUpsertCmd `cmd:"" help:"Upsert registrar-managed facts by domain"`
	Update DomainRegistrationsUpdateCmd `cmd:"" help:"Update commercial fields by domain"`
}

type DomainRegistrationsListCmd struct {
	QueryFlags `embed:""`
	All        bool `help:"Fetch all pages"`
	Page       int  `default:"1" help:"Page number"`
	PerPage    int  `default:"25" help:"Items per page"`
}

func (c *DomainRegistrationsListCmd) Run(ctx context.Context) error {
	return auditedGlobalList(ctx, "/domain_registrations", &c.QueryFlags, c.All, c.Page, c.PerPage)
}

type DomainRegistrationsGetCmd struct {
	ID string `required:"" help:"Domain registration short ID"`
}

func (c *DomainRegistrationsGetCmd) Run(ctx context.Context) error {
	return auditedGlobalGet(ctx, "/domain_registrations"+escaped(c.ID))
}

type DomainRegistrationsCountCmd struct {
	CountQueryFlags `embed:""`
}

func (c *DomainRegistrationsCountCmd) Run(ctx context.Context) error {
	opts, err := countRequestOptions(&c.CountQueryFlags, false)
	if err != nil {
		return err
	}
	return auditedGlobalGet(ctx, "/domain_registrations/count", opts...)
}

type DomainRegistrationsUpsertCmd struct {
	File        string   `help:"Read JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline registrar fact assignments"`
}

func (c *DomainRegistrationsUpsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	return auditedGlobalWrite(ctx, flags, http.MethodPut, "/domain_registrations", c.File, c.Assignments)
}

type DomainRegistrationsUpdateCmd struct {
	File        string   `help:"Read JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline commercial field assignments"`
}

func (c *DomainRegistrationsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	return auditedGlobalWrite(ctx, flags, http.MethodPatch, "/domain_registrations", c.File, c.Assignments)
}
