package cmd

// FunctionsCmd executes cloud functions.
type FunctionsCmd struct {
	Run FunctionsRunCmd `cmd:"" help:"Execute cloud function"`
}
