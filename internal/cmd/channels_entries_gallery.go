package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelEntriesGalleryCmd manages gallery fields on channel entries.
type ChannelEntriesGalleryCmd struct {
	List    ChannelEntriesGalleryListCmd    `cmd:"" help:"List gallery images"`
	Set     ChannelEntriesGallerySetCmd     `cmd:"" help:"Replace gallery images"`
	Add     ChannelEntriesGalleryAddCmd     `cmd:"" help:"Append gallery images"`
	Update  ChannelEntriesGalleryUpdateCmd  `cmd:"" help:"Update one gallery image"`
	Remove  ChannelEntriesGalleryRemoveCmd  `cmd:"" help:"Remove gallery images"`
	Reorder ChannelEntriesGalleryReorderCmd `cmd:"" help:"Reorder gallery images"`
}

// ChannelEntriesGalleryListCmd lists gallery images.
type ChannelEntriesGalleryListCmd struct {
	Channel string `required:"" help:"Channel ID or slug"`
	Entry   string `required:"" help:"Entry ID or slug"`
	Field   string `required:"" help:"Gallery field name"`
	Locale  string `help:"Content locale for localized channel fields"`
}

// ChannelEntriesGallerySetCmd replaces gallery images.
type ChannelEntriesGallerySetCmd struct {
	Channel   string   `required:"" help:"Channel ID or slug"`
	Entry     string   `required:"" help:"Entry ID or slug"`
	Field     string   `required:"" help:"Gallery field name"`
	Images    []string `name:"image" help:"Local image path (repeatable)"`
	Captions  []string `name:"caption" help:"Caption for each image, matched by order"`
	Positions []int    `name:"position" help:"Position for each image, matched by order"`
	Locale    string   `help:"Content locale for localized channel fields"`
	DryRun    bool     `name:"dry-run" help:"Print the payload without updating the entry"`
}

// ChannelEntriesGalleryAddCmd appends gallery images.
type ChannelEntriesGalleryAddCmd struct {
	Channel   string   `required:"" help:"Channel ID or slug"`
	Entry     string   `required:"" help:"Entry ID or slug"`
	Field     string   `required:"" help:"Gallery field name"`
	Images    []string `name:"image" help:"Local image path (repeatable)"`
	Captions  []string `name:"caption" help:"Caption for each image, matched by order"`
	Positions []int    `name:"position" help:"Position for each image, matched by order"`
	Locale    string   `help:"Content locale for localized channel fields"`
	DryRun    bool     `name:"dry-run" help:"Print the payload without updating the entry"`
}

// ChannelEntriesGalleryUpdateCmd updates one gallery image.
type ChannelEntriesGalleryUpdateCmd struct {
	Channel  string  `required:"" help:"Channel ID or slug"`
	Entry    string  `required:"" help:"Entry ID or slug"`
	Field    string  `required:"" help:"Gallery field name"`
	ImageID  string  `name:"image-id" required:"" help:"Gallery image ID"`
	Caption  *string `help:"Updated caption"`
	Position *int    `help:"Updated position"`
	Image    string  `name:"image" help:"Replacement local image path"`
	Locale   string  `help:"Content locale for localized channel fields"`
	DryRun   bool    `name:"dry-run" help:"Print the payload without updating the entry"`
}

// ChannelEntriesGalleryRemoveCmd removes gallery images.
type ChannelEntriesGalleryRemoveCmd struct {
	Channel  string   `required:"" help:"Channel ID or slug"`
	Entry    string   `required:"" help:"Entry ID or slug"`
	Field    string   `required:"" help:"Gallery field name"`
	ImageIDs []string `name:"image-id" required:"" help:"Gallery image ID to remove (repeatable)"`
	Locale   string   `help:"Content locale for localized channel fields"`
	DryRun   bool     `name:"dry-run" help:"Print the payload without updating the entry"`
}

// ChannelEntriesGalleryReorderCmd reorders gallery images.
type ChannelEntriesGalleryReorderCmd struct {
	Channel string `required:"" help:"Channel ID or slug"`
	Entry   string `required:"" help:"Entry ID or slug"`
	Field   string `required:"" help:"Gallery field name"`
	Order   string `required:"" help:"Comma-separated gallery image IDs in desired order"`
	Locale  string `help:"Content locale for localized channel fields"`
	DryRun  bool   `name:"dry-run" help:"Print the payload without updating the entry"`
}

func (c *ChannelEntriesGalleryListCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, err := galleryClient(ctx)
	if err != nil {
		return err
	}
	if err := validateGalleryField(ctx, client, c.Channel, c.Field); err != nil {
		return err
	}
	entry, err := getGalleryEntry(ctx, client, c.Channel, c.Entry, c.Locale)
	if err != nil {
		return err
	}
	rows := galleryRowsFromEntry(entry, c.Field)
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, rows)
	}
	fields := []string{"id", "position", "caption", "filename", "url"}
	if mode.Plain {
		return output.PlainFromSlice(ctx, rows, fields)
	}
	return output.WriteTable(ctx, rows, fields, []string{"ID", "POSITION", "CAPTION", "FILENAME", "URL"})
}

func (c *ChannelEntriesGallerySetCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !c.DryRun {
		if err := requireWrite(flags, "set gallery"); err != nil {
			return err
		}
	}
	client, err := galleryClient(ctx)
	if err != nil {
		return err
	}
	if err := validateGalleryField(ctx, client, c.Channel, c.Field); err != nil {
		return err
	}
	images, err := buildNewGalleryImages(c.Images, c.Captions, c.Positions, 0)
	if err != nil {
		return err
	}
	payload := galleryPayload(c.Field, images)
	return printOrUpdateGallery(ctx, client, c.Channel, c.Entry, c.Locale, payload, c.DryRun)
}

func (c *ChannelEntriesGalleryAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !c.DryRun {
		if err := requireWrite(flags, "add gallery images"); err != nil {
			return err
		}
	}
	client, err := galleryClient(ctx)
	if err != nil {
		return err
	}
	if err := validateGalleryField(ctx, client, c.Channel, c.Field); err != nil {
		return err
	}
	entry, err := getGalleryEntry(ctx, client, c.Channel, c.Entry, c.Locale)
	if err != nil {
		return err
	}
	start := nextGalleryPosition(galleryRowsFromEntry(entry, c.Field))
	images, err := buildNewGalleryImages(c.Images, c.Captions, c.Positions, start)
	if err != nil {
		return err
	}
	payload := galleryPayload(c.Field, images)
	return printOrUpdateGallery(ctx, client, c.Channel, c.Entry, c.Locale, payload, c.DryRun)
}

func (c *ChannelEntriesGalleryUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !c.DryRun {
		if err := requireWrite(flags, "update gallery image"); err != nil {
			return err
		}
	}
	client, err := galleryClient(ctx)
	if err != nil {
		return err
	}
	if err := validateGalleryField(ctx, client, c.Channel, c.Field); err != nil {
		return err
	}
	image, err := buildGalleryImagePatch(c.ImageID, c.Caption, c.Position, c.Image)
	if err != nil {
		return err
	}
	entry, err := getGalleryEntry(ctx, client, c.Channel, c.Entry, c.Locale)
	if err != nil {
		return err
	}
	if err := validateGalleryImageIDs(galleryRowsFromEntry(entry, c.Field), galleryImageIDsFromPatches([]map[string]any{image})); err != nil {
		return err
	}
	payload := galleryPayload(c.Field, []map[string]any{image})
	return printOrUpdateGallery(ctx, client, c.Channel, c.Entry, c.Locale, payload, c.DryRun)
}

func (c *ChannelEntriesGalleryRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !c.DryRun {
		if err := requireWrite(flags, "remove gallery image"); err != nil {
			return err
		}
		if err := requireForce(flags, "gallery image"); err != nil {
			return err
		}
	}
	client, err := galleryClient(ctx)
	if err != nil {
		return err
	}
	if err := validateGalleryField(ctx, client, c.Channel, c.Field); err != nil {
		return err
	}
	images, err := buildGalleryRemovePatches(c.ImageIDs)
	if err != nil {
		return err
	}
	entry, err := getGalleryEntry(ctx, client, c.Channel, c.Entry, c.Locale)
	if err != nil {
		return err
	}
	if err := validateGalleryImageIDs(galleryRowsFromEntry(entry, c.Field), galleryImageIDsFromPatches(images)); err != nil {
		return err
	}
	payload := galleryPayload(c.Field, images)
	return printOrUpdateGallery(ctx, client, c.Channel, c.Entry, c.Locale, payload, c.DryRun)
}

func (c *ChannelEntriesGalleryReorderCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !c.DryRun {
		if err := requireWrite(flags, "reorder gallery"); err != nil {
			return err
		}
	}
	client, err := galleryClient(ctx)
	if err != nil {
		return err
	}
	if err := validateGalleryField(ctx, client, c.Channel, c.Field); err != nil {
		return err
	}
	entry, err := getGalleryEntry(ctx, client, c.Channel, c.Entry, c.Locale)
	if err != nil {
		return err
	}
	images, err := buildGalleryReorderPatches(galleryRowsFromEntry(entry, c.Field), c.Order)
	if err != nil {
		return err
	}
	payload := galleryPayload(c.Field, images)
	return printOrUpdateGallery(ctx, client, c.Channel, c.Entry, c.Locale, payload, c.DryRun)
}

func galleryClient(ctx context.Context) (*api.Client, error) {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return nil, err
	}
	return GetAPIClientWithSite(ctx, site)
}

func validateGalleryField(ctx context.Context, client *api.Client, channel, field string) error {
	fields, err := api.GetChannelCustomizations(ctx, client, channel)
	if err != nil {
		return fmt.Errorf("validate gallery field: %w", err)
	}
	for _, candidate := range fields {
		if candidate.Name != field {
			continue
		}
		if candidate.Type != "gallery" {
			return fmt.Errorf("field %q is type %s, expected gallery", field, candidate.Type)
		}
		return nil
	}
	return fmt.Errorf("field %q not found on channel %s", field, channel)
}

func getGalleryEntry(ctx context.Context, client *api.Client, channel, entry, locale string) (api.Entry, error) {
	path := "/channels/" + url.PathEscape(channel) + "/entries/" + url.PathEscape(entry)
	opts := galleryLocaleOptions(locale)
	var result api.Entry
	if err := client.Get(ctx, path, &result, opts...); err != nil {
		if !api.IsNotFound(err) {
			return api.Entry{}, fmt.Errorf("get entry: %w", err)
		}
		found, findErr := findChannelEntryBySlug(ctx, client, channel, entry, opts...)
		if findErr != nil {
			return api.Entry{}, fmt.Errorf("get entry: %w", findErr)
		}
		if found.ID == "" {
			return api.Entry{}, fmt.Errorf("get entry: %w", err)
		}
		result = found
	}
	return result, nil
}

func printOrUpdateGallery(ctx context.Context, client *api.Client, channel, entry, locale string, payload map[string]any, dryRun bool) error {
	if dryRun {
		return output.JSON(ctx, payload)
	}
	updated, err := updateGalleryEntry(ctx, client, channel, entry, locale, payload)
	if err != nil {
		return err
	}
	return output.Print(ctx, updated, []any{updated.ID}, func() error {
		_, err := output.Fprintf(ctx, "Updated entry %s\n", updated.ID)
		return err
	})
}

func updateGalleryEntry(ctx context.Context, client *api.Client, channel, entry, locale string, payload map[string]any) (api.Entry, error) {
	path := "/channels/" + url.PathEscape(channel) + "/entries/" + url.PathEscape(entry)
	opts := galleryLocaleOptions(locale)
	var updated api.Entry
	if err := client.Put(ctx, path, payload, &updated, opts...); err != nil {
		if !api.IsNotFound(err) {
			return api.Entry{}, fmt.Errorf("update gallery: %w", err)
		}
		found, findErr := findChannelEntryBySlug(ctx, client, channel, entry, opts...)
		if findErr != nil {
			return api.Entry{}, fmt.Errorf("update gallery: %w", findErr)
		}
		if found.ID == "" {
			return api.Entry{}, fmt.Errorf("update gallery: %w", err)
		}
		path = "/channels/" + url.PathEscape(channel) + "/entries/" + url.PathEscape(found.ID)
		if err := client.Put(ctx, path, payload, &updated, opts...); err != nil {
			return api.Entry{}, fmt.Errorf("update gallery: %w", err)
		}
	}
	return updated, nil
}

func galleryLocaleOptions(locale string) []api.RequestOption {
	if locale == "" {
		return nil
	}
	return []api.RequestOption{api.WithContentLocale(locale)}
}
