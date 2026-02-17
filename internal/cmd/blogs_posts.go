package cmd

// BlogPostsCmd manages blog posts.
type BlogPostsCmd struct {
	List BlogPostsListCmd `cmd:"" help:"List blog posts"`
	Get  BlogPostsGetCmd  `cmd:"" help:"Get blog post details"`
}
