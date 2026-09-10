// squiz — a Socratic understanding checker: Claude Code skill + CLI state
// machine + TUI. See squiz-design.md in the repository for the design.
package main

import (
	"os"

	"github.com/agentwiki/squiz/internal/cli"
)

// set by GoReleaser, for example: -ldflags "-X main.version=v0.2.0"
var version = "dev"

func main() {
	cli.SetVersion(version)
	os.Exit(cli.Main(os.Args[1:]))
}
