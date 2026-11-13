//go:build !windows

package selfupdate

import (
	"os"
	"syscall"
)

// Restart replaces the running process with the binary at exe, run with the
// same arguments and environment. The process keeps its ID and its
// terminal; every file and socket it had open is closed. It returns only
// when the exec fails.
func Restart(exe string) error {
	return syscall.Exec(exe, os.Args, os.Environ()) // #nosec G204 -- exe is folio's own binary
}
