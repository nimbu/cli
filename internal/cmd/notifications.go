package cmd

// NotificationsCmd manages notifications.
type NotificationsCmd struct {
	List   NotificationsListCmd   `cmd:"" help:"List notifications"`
	Get    NotificationsGetCmd    `cmd:"" help:"Get notification details"`
	Create NotificationsCreateCmd `cmd:"" help:"Create notification from JSON"`
	Update NotificationsUpdateCmd `cmd:"" help:"Update notification"`
	Delete NotificationsDeleteCmd `cmd:"" help:"Delete notification"`
	Count  NotificationsCountCmd  `cmd:"" help:"Count notifications"`
}
