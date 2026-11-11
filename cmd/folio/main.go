// Command folio manages and renders a Markdown documentation tree.
//
// Usage: folio <object> <subcommand> [flags] [args]
//
// The object "chapters" carries the subcommands `insert`, `move`, and
// `remove`; "lint", "flatten", "render", "migrate", and "web" have no
// subcommands. The "chapters" command itself is a dummy wrapper
// required because github.com/christophberger/start has no native notion of
// an "object" parameter (see spec.yaml, constraint SUBCOMMANDS.3).
package main

import (
	"fmt"
	"os"

	"github.com/christophberger-ailab/foolscap-folio/flatten"
	"github.com/christophberger-ailab/foolscap-folio/insert"
	"github.com/christophberger-ailab/foolscap-folio/internal/cli"
	"github.com/christophberger-ailab/foolscap-folio/lint"
	"github.com/christophberger-ailab/foolscap-folio/migrate"
	"github.com/christophberger-ailab/foolscap-folio/move"
	"github.com/christophberger-ailab/foolscap-folio/remove"
	"github.com/christophberger-ailab/foolscap-folio/render"
	"github.com/christophberger-ailab/foolscap-folio/web"

	"github.com/christophberger/start"
	flag "github.com/spf13/pflag"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	// Global flag(s) — shared by all subcommands. Per spec
	// constraint SUBCOMMANDS.2-2 these have matching environment
	// variables (QM_PROJECT) and config file entries (project=).
	projectFlag := flag.StringP("project", "p", ".", "Path to the document tree")
	helpFlag := flag.BoolP("help", "h", false, "Show help for folio or for a command")

	cli.Description = "folio manages and renders a Markdown documentation tree."
	start.SetVersion(version)
	start.SetInitFunc(func() error { return nil })

	// Register the dummy "chapters" object command. start has no notion
	// of an <object> parameter, so we model it as a parent command that
	// only dispatches to its subcommands (SUBCOMMANDS.3). Cmd stays
	// nil: start v0.6.0 panics when Cmd is set but no subcommand was
	// supplied, and cli.Help answers a missing subcommand anyway.
	if err := cli.Add(&start.Command{
		Name:  "chapters",
		Short: "Manage book chapters",
		Long:  "Manage the chapters of a content folder with one of the subcommands.",
	}); err != nil {
		fatal(err)
	}

	// Register each subcommand under "chapters" as its own package.
	insert.Register("chapters", projectFlag)
	move.Register("chapters", projectFlag)
	remove.Register("chapters", projectFlag)

	// The `lint` object has no sub-commands, so it is registered as a
	// leaf command directly (SUBCOMMANDS.3 does not apply when there is
	// nothing to dispatch to).
	lint.Register(projectFlag)

	// The `render` object also has no sub-commands.
	render.Register(projectFlag)

	// The `flatten` object saves render's generated inputs without rendering.
	flatten.Register(projectFlag)

	// `migrate` writes folio.yml from a project's old Quarto configuration.
	migrate.Register(projectFlag)

	// The `web` object serves the sorter UI; no sub-commands either.
	web.Register(projectFlag)

	// start's own help prints different texts for `folio` and `folio
	// --help` and lists every command's flags as global; cli.Help answers
	// all requests for help instead. Parsing twice is harmless: start
	// reads the environment and config file only once.
	flag.Usage = cli.Usage
	if err := start.Parse(); err != nil {
		fatal(err)
	}
	if !cli.Help(flag.Args(), *helpFlag) {
		start.Up()
	}

	// start.Up() prints a command's error and returns; it does not set an
	// exit status. Scripts and CI read it to tell a failed render from a
	// good one. See internal/cli.
	cli.Exit()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
