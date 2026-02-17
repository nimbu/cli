package cmd

// CouponsCmd manages coupons.
type CouponsCmd struct {
	List   CouponsListCmd   `cmd:"" help:"List coupons"`
	Get    CouponsGetCmd    `cmd:"" help:"Get coupon details"`
	Create CouponsCreateCmd `cmd:"" help:"Create coupon from JSON"`
	Update CouponsUpdateCmd `cmd:"" help:"Update coupon"`
	Delete CouponsDeleteCmd `cmd:"" help:"Delete coupon"`
	Count  CouponsCountCmd  `cmd:"" help:"Count coupons"`
}
