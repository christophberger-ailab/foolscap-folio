// Package upgrade implements `folio upgrade` and its alias `folio update`:
// look for a newer release of folio and, when there is one, offer to put it
// in place of the running binary.
//
// Spec: spec-upgrade.yaml.
package upgrade

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/selfupdate"
	"github.com/christophberger/start"
	flag "github.com/spf13/pflag"
)

// Register wires `upgrade` and `update` into start. version is the running
// release; source returns the URL releases are looked for at, which the
// web settings file (config.cue) holds.
func Register(version string, source func() (string, error)) {
	check := flag.Bool("check", false, "Only report whether a newer release exists")
	yes := flag.Bool("yes", false, "Install a newer release without asking")
	long := "Look for a newer release of folio and offer to install it in place of " +
		"the running binary. The download is checked against the release's " +
		"SHA-256 checksums before it is installed. Releases are looked for at the " +
		"update source in folio's config.cue (default: GitHub). " +
		"Use --check to only report, --yes to install without asking."
	run := func(*start.Command) error {
		src, err := source()
		if err != nil {
			return err
		}
		exe, err := selfupdate.Executable()
		if err != nil {
			return fmt.Errorf("cannot find the running binary: %w", err)
		}
		return Run(context.Background(), Options{
			Current: version, Source: src, Exe: exe, Client: selfupdate.Client(),
			CheckOnly: *check, Yes: *yes, In: os.Stdin, Out: os.Stdout,
		})
	}
	for _, c := range []struct{ name, short string }{
		{"upgrade", "Update folio to its newest release"},
		{"update", "Same as upgrade"},
	} {
		start.Add(&start.Command{Name: c.name, Short: c.short, Long: long, Flags: []string{"check", "yes"}, Cmd: run})
	}
}

// Options control Run.
type Options struct {
	Current string // the running release
	Source  string // where the newest release is described
	Exe     string // the binary to replace
	Client  *http.Client

	CheckOnly bool // report, but do not install
	Yes       bool // install without asking

	In  io.Reader // where the answer to "Update now?" is read from
	Out io.Writer
}

// Run looks for a release newer than o.Current and installs it if asked to.
func Run(ctx context.Context, o Options) error {
	if !selfupdate.IsRelease(o.Current) {
		return fmt.Errorf("this is a development build (%s), which cannot be compared with a release; install a release to use upgrade", o.Current)
	}
	rel, err := selfupdate.Latest(ctx, o.Client, o.Source)
	if err != nil {
		return err
	}
	if !selfupdate.Newer(o.Current, rel.Version) {
		fmt.Fprintf(o.Out, "folio %s is the newest release.\n", o.Current)
		return nil
	}
	fmt.Fprintf(o.Out, "folio %s is available (you have %s).\n", rel.Version, o.Current)
	if rel.URL != "" {
		fmt.Fprintf(o.Out, "Release notes: %s\n", rel.URL)
	}
	if o.CheckOnly {
		return nil
	}
	if !o.Yes && !confirm(o.In, o.Out) {
		fmt.Fprintln(o.Out, "Not updated.")
		return nil
	}
	if err := selfupdate.Install(ctx, o.Client, rel, o.Exe); err != nil {
		return fmt.Errorf("not updated: %w", err)
	}
	fmt.Fprintf(o.Out, "Updated to %s.\n", rel.Version)
	return nil
}

// confirm asks "Update now?" and reports whether the answer was yes. No
// answer -- an empty line, a closed input -- is no.
func confirm(in io.Reader, out io.Writer) bool {
	fmt.Fprint(out, "Update now? [y/N] ")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
