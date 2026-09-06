// Command todo is the single deployable this repository builds: one binary
// with three modes. See docs/adrs/0001-ship-todo-as-one-go-binary.md.
package main

import (
	"os"

	"github.com/possiblyneal/todo/apps/todo/src/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
