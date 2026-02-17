package cmd

// BlogsCmd manages blogs.
type BlogsCmd struct {
	List  BlogsListCmd `cmd:"" help:"List blogs"`
	Get   BlogsGetCmd  `cmd:"" help:"Get blog details"`
	Posts BlogPostsCmd `cmd:"" help:"Manage blog posts"`
}
