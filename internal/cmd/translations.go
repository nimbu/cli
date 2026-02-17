package cmd

// TranslationsCmd manages translations.
type TranslationsCmd struct {
	List   TranslationsListCmd   `cmd:"" help:"List translations"`
	Get    TranslationsGetCmd    `cmd:"" help:"Get translation by key"`
	Create TranslationsCreateCmd `cmd:"" help:"Create a translation"`
	Update TranslationsUpdateCmd `cmd:"" help:"Update a translation"`
	Delete TranslationsDeleteCmd `cmd:"" help:"Delete a translation"`
	Count  TranslationsCountCmd  `cmd:"" help:"Count translations"`
}
