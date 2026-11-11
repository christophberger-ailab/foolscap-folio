package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestMain lets the test binary stand in for folio: run with FOLIO_RUN_MAIN
// set, it is folio.
func TestMain(m *testing.M) {
	if os.Getenv("FOLIO_RUN_MAIN") != "" {
		os.Args = append(os.Args[:1], strings.Fields(os.Getenv("FOLIO_ARGS"))...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// folio runs main with args and returns its stdout and exit code.
func folio(t *testing.T, args string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "FOLIO_RUN_MAIN=1", "FOLIO_ARGS="+args)
	cmd.Dir = t.TempDir()
	out, err := cmd.Output()
	if ee, ok := err.(*exec.ExitError); ok {
		return string(out), ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), 0
}

func TestHelpIsTheSameWithAndWithoutFlag(t *testing.T) {
	plain, code := folio(t, "")
	if code != 0 {
		t.Fatalf("folio exits %d", code)
	}
	for _, want := range []string{"chapters insert", "render", "web", "Global flags:", "--project"} {
		if !strings.Contains(plain, want) {
			t.Errorf("folio's help lacks %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "--addr") {
		t.Errorf("folio's help lists web's own --addr:\n%s", plain)
	}
	for _, args := range []string{"--help", "-h", "help"} {
		if got, code := folio(t, args); got != plain || code != 0 {
			t.Errorf("folio %s (exit %d) differs from folio:\n%s", args, code, got)
		}
	}
}

func TestCommandHelpListsCommandFlags(t *testing.T) {
	viaHelp, _ := folio(t, "help web")
	if !strings.Contains(viaHelp, "--addr") {
		t.Errorf("folio help web lacks --addr:\n%s", viaHelp)
	}
	for _, args := range []string{"web --help", "-p . web -h"} {
		if got, code := folio(t, args); got != viaHelp || code != 0 {
			t.Errorf("folio %s (exit %d) differs from folio help web:\n%s", args, code, got)
		}
	}
}
