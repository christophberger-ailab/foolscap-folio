package selfupdate

import (
	"context"
	"strings"
	"testing"
)

func TestManager(t *testing.T) {
	for exe, want := range map[string]string{
		"/opt/homebrew/Caskroom/folio/0.9.0/folio":              "brew upgrade folio",
		"/home/linuxbrew/.linuxbrew/Caskroom/folio/0.9.0/folio": "brew upgrade folio",
		"/usr/local/Cellar/folio/0.9.0/bin/folio":               "brew upgrade folio",
		`C:\Users\ana\scoop\apps\folio\0.9.0\folio.exe`:         "scoop update folio",
		`C:\ProgramData\scoop\apps\folio\current\folio.exe`:     "scoop update folio",
		"/usr/bin/folio":             "the system's package manager (apt, dnf, apk or pacman)",
		"/usr/local/bin/folio":       "",
		"/home/ana/bin/folio":        "",
		`C:\Users\ana\bin\folio.exe`: "",
		"/home/ana/src/scoop/folio":  "",
	} {
		if got := Manager(exe); got != want {
			t.Errorf("Manager(%q) = %q, want %q", exe, got, want)
		}
	}
}

// A binary a package manager owns is left to it: Install refuses before
// anything is downloaded.
func TestInstallLeavesAManagedBinaryAlone(t *testing.T) {
	err := Install(context.Background(), nil, Release{Version: "v0.9.0"}, "/opt/homebrew/Caskroom/folio/0.8.0/folio")
	if err == nil || !strings.Contains(err.Error(), "brew upgrade folio") {
		t.Errorf("Install: %v, want a pointer to brew upgrade", err)
	}
}
