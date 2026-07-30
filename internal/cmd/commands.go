package cmd

import (
	"context"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/nimbu/cli/internal/output"
)

// CommandsCmd exports the machine-readable CLI contract used by docs and skills.
type CommandsCmd struct{}

func (c *CommandsCmd) Run(ctx context.Context) error {
	parser, _, err := newParser()
	if err != nil {
		return err
	}
	return output.JSON(ctx, buildCommandContract(parser.Model))
}

type CommandContract struct {
	SchemaVersion int                      `json:"schema_version"`
	CLI           string                   `json:"cli"`
	GlobalFlags   []CommandContractFlag    `json:"global_flags,omitempty"`
	Commands      []CommandContractCommand `json:"commands"`
}

type CommandContractCommand struct {
	Path      string                    `json:"path"`
	Help      string                    `json:"help,omitempty"`
	Aliases   []string                  `json:"aliases,omitempty"`
	Arguments []CommandContractArgument `json:"arguments,omitempty"`
	Flags     []CommandContractFlag     `json:"flags,omitempty"`
}

type CommandContractArgument struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Help     string `json:"help,omitempty"`
}

type CommandContractFlag struct {
	Name     string   `json:"name"`
	Short    string   `json:"short,omitempty"`
	Required bool     `json:"required"`
	Default  string   `json:"default,omitempty"`
	Help     string   `json:"help,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
}

func buildCommandContract(application *kong.Application) CommandContract {
	contract := CommandContract{SchemaVersion: 1, CLI: application.Name}
	for _, flag := range application.Flags {
		if !flag.Hidden {
			contract.GlobalFlags = append(contract.GlobalFlags, commandContractFlag(flag))
		}
	}
	var visit func(*kong.Node, []string)
	visit = func(node *kong.Node, parentPath []string) {
		path := parentPath
		if node.Type == kong.CommandNode {
			path = append(append([]string{}, parentPath...), node.Name)
			if !node.Hidden {
				command := CommandContractCommand{
					Path:    strings.Join(path, " "),
					Help:    node.Help,
					Aliases: node.Aliases,
				}
				for _, argument := range node.Positional {
					command.Arguments = append(command.Arguments, CommandContractArgument{
						Name: argument.Name, Required: argument.Required, Help: argument.Help,
					})
				}
				for _, flag := range node.Flags {
					if !flag.Hidden {
						command.Flags = append(command.Flags, commandContractFlag(flag))
					}
				}
				contract.Commands = append(contract.Commands, command)
			}
		}
		for _, child := range node.Children {
			if child.Type == kong.CommandNode {
				visit(child, path)
			}
		}
	}
	visit(application.Node, []string{application.Name})
	return contract
}

func commandContractFlag(flag *kong.Flag) CommandContractFlag {
	short := ""
	if flag.Short != 0 {
		short = string(flag.Short)
	}
	return CommandContractFlag{
		Name: flag.Name, Short: short, Required: flag.Required,
		Default: flag.Default, Help: flag.Help, Aliases: flag.Aliases,
	}
}
