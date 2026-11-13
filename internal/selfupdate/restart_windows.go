package selfupdate

import (
	"os"
	"os/exec"
)

// Restart starts the binary at exe with the same arguments, in the same
// console, and ends the running process. Windows has no exec that keeps
// the process, so the new one gets a new process ID. It returns only when
// the new process cannot be started.
func Restart(exe string) error {
	cmd := exec.Command(exe, os.Args[1:]...) // #nosec G204 -- exe is folio's own binary
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
