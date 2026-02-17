package cmd

// UploadsCmd manages uploads.
type UploadsCmd struct {
	List   UploadsListCmd   `cmd:"" help:"List uploads"`
	Get    UploadsGetCmd    `cmd:"" help:"Get upload details"`
	Create UploadsCreateCmd `cmd:"" help:"Upload a file"`
	Delete UploadsDeleteCmd `cmd:"" help:"Delete an upload"`
	Count  UploadsCountCmd  `cmd:"" help:"Count uploads"`
}
