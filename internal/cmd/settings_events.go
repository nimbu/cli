package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

type SettingsCmd struct {
	Get     SettingsGetCmd     `cmd:"" help:"Get a settings section"`
	Update  SettingsUpdateCmd  `cmd:"" help:"Update a settings section"`
	Consent SettingsConsentCmd `cmd:"" help:"Manage consent resources"`
}

type SettingsGetCmd struct {
	Section string `required:"" enum:"checkout,notifications,taxes,shipping,shop,email" help:"Settings section"`
}

func (c *SettingsGetCmd) Run(ctx context.Context) error {
	return auditedGet(ctx, "/settings"+escaped(c.Section))
}

type SettingsUpdateCmd struct {
	Section     string   `required:"" enum:"checkout,notifications,taxes,shipping,shop" help:"Settings section"`
	File        string   `help:"Read JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline settings assignments"`
}

func (c *SettingsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	return auditedWrite(ctx, flags, http.MethodPatch, "/settings"+escaped(c.Section), c.File, c.Assignments)
}

type SettingsConsentCmd struct {
	List   SettingsConsentListCmd   `cmd:"" help:"List consent resources"`
	Get    SettingsConsentGetCmd    `cmd:"" help:"Get a consent resource"`
	Create SettingsConsentCreateCmd `cmd:"" help:"Create a consent resource"`
	Update SettingsConsentUpdateCmd `cmd:"" help:"Update a consent resource"`
	Delete SettingsConsentDeleteCmd `cmd:"" help:"Delete a consent resource (requires --force)"`
}

func consentPath(kind, key string) (string, error) {
	switch kind {
	case "purposes", "cookies", "applications":
	default:
		return "", fmt.Errorf("unsupported consent kind %q", kind)
	}
	path := "/settings/consent/" + kind
	if key != "" {
		path += escaped(key)
	}
	return path, nil
}

type SettingsConsentListCmd struct {
	Kind string `required:"" enum:"purposes,cookies,applications" help:"Consent resource kind"`
}

func (c *SettingsConsentListCmd) Run(ctx context.Context) error {
	path, err := consentPath(c.Kind, "")
	if err != nil {
		return err
	}
	return auditedArray(ctx, path, nil)
}

type SettingsConsentGetCmd struct {
	Kind string `required:"" enum:"purposes,cookies,applications" help:"Consent resource kind"`
	Key  string `required:"" help:"Purpose/application key or cookie ID"`
}

func (c *SettingsConsentGetCmd) Run(ctx context.Context) error {
	path, err := consentPath(c.Kind, c.Key)
	if err != nil {
		return err
	}
	return auditedGet(ctx, path)
}

type SettingsConsentCreateCmd struct {
	Kind        string   `required:"" enum:"purposes,cookies,applications" help:"Consent resource kind"`
	File        string   `help:"Read JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline consent resource assignments"`
}

func (c *SettingsConsentCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	path, err := consentPath(c.Kind, "")
	if err != nil {
		return err
	}
	return auditedWrite(ctx, flags, http.MethodPost, path, c.File, c.Assignments)
}

type SettingsConsentUpdateCmd struct {
	Kind        string   `required:"" enum:"purposes,cookies,applications" help:"Consent resource kind"`
	Key         string   `required:"" help:"Purpose/application key or cookie ID"`
	File        string   `help:"Read JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline consent resource assignments"`
}

func (c *SettingsConsentUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	path, err := consentPath(c.Kind, c.Key)
	if err != nil {
		return err
	}
	return auditedWrite(ctx, flags, http.MethodPatch, path, c.File, c.Assignments)
}

type SettingsConsentDeleteCmd struct {
	Kind string `required:"" enum:"purposes,cookies,applications" help:"Consent resource kind"`
	Key  string `required:"" help:"Purpose/application key or cookie ID"`
}

func (c *SettingsConsentDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	path, err := consentPath(c.Kind, c.Key)
	if err != nil {
		return err
	}
	return auditedDelete(ctx, flags, path)
}

type EventsCmd struct {
	Track  EventsTrackCmd  `cmd:"" help:"Track a single named event"`
	Ingest EventsIngestCmd `cmd:"" help:"Ingest Count.ly-compatible events and metrics"`
}

type EventsTrackCmd struct {
	Event       string   `required:"" help:"Event name"`
	File        string   `help:"Read JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline event payload assignments"`
}

func (c *EventsTrackCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := "/events" + escaped(strings.TrimPrefix(c.Event, "/"))
	if c.File == "" && len(c.Assignments) == 0 {
		return auditedWriteBody(ctx, flags, http.MethodPost, path, nil)
	}
	return auditedWrite(ctx, flags, http.MethodPost, path, c.File, c.Assignments)
}

type EventsIngestCmd struct {
	DeviceID        string `required:"" name:"device-id" help:"Analytics device identifier"`
	Events          string `help:"JSON array of events"`
	Metrics         string `help:"JSON metrics object"`
	Timestamp       string `help:"Unix timestamp"`
	BeginSession    bool   `name:"begin-session" help:"Mark the beginning of a session"`
	EndSession      bool   `name:"end-session" help:"Mark the end of a session"`
	SessionDuration string `name:"session-duration" help:"Session duration in seconds"`
}

func (c *EventsIngestCmd) Run(ctx context.Context) error {
	opts := []api.RequestOption{api.WithParam("device_id", c.DeviceID)}
	for key, value := range map[string]string{
		"events": c.Events, "metrics": c.Metrics, "timestamp": c.Timestamp,
		"session_duration": c.SessionDuration,
	} {
		if value != "" {
			opts = append(opts, api.WithParam(key, value))
		}
	}
	if c.BeginSession {
		opts = append(opts, api.WithParam("begin_session", "true"))
	}
	if c.EndSession {
		opts = append(opts, api.WithParam("end_session", "true"))
	}
	return auditedGet(ctx, "/events", opts...)
}
