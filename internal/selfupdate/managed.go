package selfupdate

import (
	"fmt"
	"path"
	"strings"
)

// Manager tells how to update a binary that a package manager installed:
// the command to run, or "" when folio was put in place by hand (or by
// folio upgrade) and may replace itself. A package manager keeps track of
// the files it installed, so overwriting one would leave it out of step.
//
// It goes by where exe lies, with symlinks resolved (see Executable):
// Homebrew's Cellar and Caskroom, Scoop's apps folder, WinGet's packages
// folder (and its Links folder, should a link come back unresolved), and
// /usr/bin, which only packages write to. folio installed by its Windows
// setup program is not managed: it lives in the user's own Programs folder,
// so folio upgrade and the Update button keep it up to date.
func Manager(exe string) string {
	p := strings.ToLower(strings.ReplaceAll(exe, `\`, "/"))
	switch {
	case strings.Contains(p, "/cellar/"), strings.Contains(p, "/caskroom/"):
		return "brew upgrade folio"
	case strings.Contains(p, "/scoop/apps/"):
		return "scoop update folio"
	case strings.Contains(p, "/winget/packages/"), strings.Contains(p, "/winget/links/"):
		return "winget upgrade --id ChristophBerger.FoolscapFolio"
	case path.Dir(p) == "/usr/bin":
		return "the system's package manager (apt, dnf, apk or pacman)"
	}
	return ""
}

// managedError is the refusal to replace a binary a package manager owns.
func managedError(exe string) error {
	return fmt.Errorf("%s was installed by a package manager; update it with %s", exe, Manager(exe))
}
