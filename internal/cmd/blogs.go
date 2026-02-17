package cmd

// BlogsCmd manages blogs.
type BlogsCmd struct {
	List   BlogsListCmd   `cmd:"" help:"List blogs"`
	Get    BlogsGetCmd    `cmd:"" help:"Get blog details"`
	Create BlogsCreateCmd `cmd:"" help:"Create a blog"`
	Update BlogsUpdateCmd `cmd:"" help:"Update a blog"`
	Delete BlogsDeleteCmd `cmd:"" help:"Delete a blog"`
	Count  BlogsCountCmd  `cmd:"" help:"Count blogs"`
	Posts  BlogPostsCmd   `cmd:"" help:"Manage blog articles" aliases:"articles"`
}
