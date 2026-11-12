// Command tnxs is the tnxstor CLI and TUI entry point.
package main

import (
	"os"

	"tnxstor/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], cli.DefaultStdio()))
}
