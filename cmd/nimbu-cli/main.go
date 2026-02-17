package main

import (
	"os"

	"github.com/nimbu/cli/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute(os.Args[1:]))
}
