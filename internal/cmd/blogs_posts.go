package cmd

// BlogPostsCmd manages blog posts (articles).
type BlogPostsCmd struct {
	List   BlogPostsListCmd   `cmd:"" help:"List blog articles"`
	Get    BlogPostsGetCmd    `cmd:"" help:"Get blog article"`
	Create BlogPostsCreateCmd `cmd:"" help:"Create a blog article"`
	Update BlogPostsUpdateCmd `cmd:"" help:"Update a blog article"`
	Delete BlogPostsDeleteCmd `cmd:"" help:"Delete a blog article"`
	Count  BlogPostsCountCmd  `cmd:"" help:"Count blog articles"`
}
