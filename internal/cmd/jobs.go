package cmd

// JobsCmd executes cloud jobs.
type JobsCmd struct {
	Run JobsRunCmd `cmd:"" help:"Schedule cloud job"`
}
