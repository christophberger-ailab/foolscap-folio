package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/christophberger/start"
	flag "github.com/spf13/pflag"
)

// setup gives a test its own flags, commands and output, shaped like
// folio's: a global --project and --help, a leaf command with a flag of its
// own, and an object with a subcommand.
func setup(t *testing.T) (out, errOut *bytes.Buffer) {
	t.Helper()
	oldFlags, oldCmds, oldOut, oldErr := flags, commands, stdout, stderr
	t.Cleanup(func() {
		flags, commands, stdout, stderr = oldFlags, oldCmds, oldOut, oldErr
		failed.Store(false)
	})
	flags = flag.NewFlagSet("folio", flag.ContinueOnError)
	flags.StringP("project", "p", ".", "Path to the document tree")
	flags.BoolP("help", "h", false, "Show help")
	flags.String("addr", "", "Address the web UI listens on")
	commands = nil
	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}
	stdout, stderr = out, errOut

	commands = append(commands,
		&start.Command{Name: "web", Short: "Serve the web UI", Long: "Serve a local web app.",
			Flags: []string{"addr"}},
		&start.Command{Name: "chapters", Short: "Manage book chapters", Long: "Manage the chapters."},
		&start.Command{Name: "insert", Parent: "chapters", Short: "Insert a chapter",
			Long: "Insert a new chapter."},
	)
	return out, errOut
}

func TestHelpOverviewListsCommandsAndGlobalFlags(t *testing.T) {
	out, _ := setup(t)
	if !Help(nil, false) {
		t.Fatal("no command was not answered with help")
	}
	got := out.String()
	for _, want := range []string{
		"Usage:", "Commands:", "web ", "Serve the web UI", "chapters ", "chapters insert ",
		"Insert a chapter", "help ", "version ", "Global flags:", "--project", "--help",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "--addr") {
		t.Errorf("overview lists the web command's own --addr:\n%s", got)
	}
	if failed.Load() {
		t.Error("asking for help was recorded as a failure")
	}
}

func TestHelpIsTheSameHoweverAsked(t *testing.T) {
	out, _ := setup(t)
	Help(nil, false)
	plain := out.String()

	for _, tc := range []struct {
		args   []string
		asked  bool
		reason string
	}{
		{nil, true, "folio --help"},
		{[]string{"help"}, false, "folio help"},
	} {
		out.Reset()
		if !Help(tc.args, tc.asked) {
			t.Fatalf("%s was not answered with help", tc.reason)
		}
		if out.String() != plain {
			t.Errorf("%s differs from folio:\n%s\n---\n%s", tc.reason, out.String(), plain)
		}
	}
}

func TestHelpForCommandListsItsFlags(t *testing.T) {
	out, _ := setup(t)
	Help([]string{"help", "web"}, false)
	viaHelp := out.String()
	for _, want := range []string{"folio web", "Serve a local web app.", "Flags:", "--addr", "Global flags:", "--project"} {
		if !strings.Contains(viaHelp, want) {
			t.Errorf("web help lacks %q:\n%s", want, viaHelp)
		}
	}
	// --project is global, so it is listed once, under the global flags.
	if strings.Count(viaHelp, "--project") != 1 {
		t.Errorf("--project is listed more than once:\n%s", viaHelp)
	}

	out.Reset()
	Help([]string{"web"}, true)
	if out.String() != viaHelp {
		t.Errorf("folio web --help differs from folio help web:\n%s\n---\n%s", out.String(), viaHelp)
	}
}

func TestHelpForObjectListsSubcommands(t *testing.T) {
	out, _ := setup(t)
	Help([]string{"help", "chapters"}, false)
	got := out.String()
	for _, want := range []string{"Subcommands:", "insert", "Insert a chapter"} {
		if !strings.Contains(got, want) {
			t.Errorf("chapters help lacks %q:\n%s", want, got)
		}
	}

	out.Reset()
	Help([]string{"chapters", "insert"}, true)
	if !strings.Contains(out.String(), "folio chapters insert") {
		t.Errorf("subcommand help does not name it:\n%s", out.String())
	}
}

func TestHelpLeavesCommandsToStart(t *testing.T) {
	setup(t)
	for _, args := range [][]string{{"web"}, {"web", "../book"}, {"chapters", "insert", "3"}, {"version"}} {
		if Help(args, false) {
			t.Errorf("%v was answered with help instead of run", args)
		}
	}
}

func TestHelpRejectsUnknownAndIncompleteCommands(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"bogus"}, `unknown command "bogus"`},
		{[]string{"help", "bogus"}, `unknown command "bogus"`},
		{[]string{"chapters"}, `"chapters" needs a subcommand`},
		{[]string{"chapters", "bogus"}, `unknown command "chapters bogus"`},
	} {
		out, errOut := setup(t)
		if !Help(tc.args, false) {
			t.Errorf("%v was handed to start", tc.args)
		}
		if !strings.Contains(errOut.String(), tc.want) {
			t.Errorf("%v: stderr lacks %q:\n%s", tc.args, tc.want, errOut.String())
		}
		if out.Len() > 0 {
			t.Errorf("%v: help went to stdout although it answers an error", tc.args)
		}
		if !failed.Load() {
			t.Errorf("%v: the error was not recorded for the exit status", tc.args)
		}
		failed.Store(false)
	}
}

func TestAddRecordsCommands(t *testing.T) {
	setup(t)
	c := &start.Command{Name: "cli-test-add", Short: "x"}
	if err := Add(c); err != nil {
		t.Fatal(err)
	}
	if commands[len(commands)-1] != c {
		t.Fatal("Add did not record the command for Help")
	}
	delete(start.Commands, c.Name)
}

func TestHelpRejectsAnotherCommandsFlag(t *testing.T) {
	_, errOut := setup(t)
	if err := flags.Parse([]string{"chapters", "insert", "--addr", ":80", "3"}); err != nil {
		t.Fatal(err)
	}
	if !Help(flags.Args(), false) {
		t.Fatal("a command with another command's flag was handed to start")
	}
	want := `"chapters insert" does not take --addr`
	if !strings.Contains(errOut.String(), want) || !strings.Contains(errOut.String(), "folio chapters insert:") {
		t.Errorf("stderr lacks %q and the command's help:\n%s", want, errOut.String())
	}
	if !failed.Load() {
		t.Error("the error was not recorded for the exit status")
	}
}

func TestUsageShowsHelpForTheCommandOnStderr(t *testing.T) {
	out, errOut := setup(t)
	_ = flags.Parse([]string{"web"})
	Usage()
	if out.Len() > 0 || !strings.Contains(errOut.String(), "folio web:") {
		t.Errorf("Usage did not print web's help to stderr:\n%s", errOut.String())
	}
}
