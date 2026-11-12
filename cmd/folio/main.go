// Command folio manages and renders a Markdown documentation tree.
//
// Usage: folio <command> [flags] [args]
//
// The commands "lint", "render", "init", "web" and "preview" have no
// subcommands.
package main

import (
	"github.com/christophberger-ailab/foolscap-folio/initialize"
	"github.com/christophberger-ailab/foolscap-folio/lint"
	"github.com/christophberger-ailab/foolscap-folio/preview"
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

	start.SetDescription("folio manages and renders a Markdown documentation tree. " +
		"Usage: folio <command> [flags] [args].")
	start.SetVersion(version)
	start.SetInitFunc(func() error { return nil })

	lint.Register(projectFlag)

	// `render` builds the project's topic × format × audience matrix.
	render.Register(projectFlag)

	// `init` writes a starter folio.yml into a tree that has none.
	initialize.Register(projectFlag)

	// The `web` object serves the sorter UI; no sub-commands either.
	web.Register(projectFlag)

	// `preview` serves the website live; it shares --addr with `web` and
	// the selection flags with `render`, so it comes after both.
	preview.Register(projectFlag)

	// start.Up() answers `folio`, `folio help` and `--help` with the same
	// help, and exits with status 1 when the command line is wrong or the
	// command fails. Scripts and CI read the status to tell a failed render
	// from a good one.
	start.Up()
}
