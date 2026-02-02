package simple_test

import (
	"io"

	"github.com/urfave/cli/v3"
)

// setWriterOnAllCommands recursively sets the writer on a command and all its subcommands
func setWriterOnAllCommands(cmd *cli.Command, w io.Writer) {
	cmd.Writer = w
	for _, subCmd := range cmd.Commands {
		setWriterOnAllCommands(subCmd, w)
	}
}
