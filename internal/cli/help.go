package cli

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/christophberger/start"
	flag "github.com/spf13/pflag"
)

// Description is folio's one-line summary, the first line of its help.
var Description string

var (
	// commands holds every command registered through Add, in order.
	// start keeps a command's subcommands to itself, so Help cannot ask
	// start for them.
	commands []*start.Command

	flags                    = flag.CommandLine
	stdout, stderr io.Writer = os.Stdout, os.Stderr
)

// builtins are the commands start adds by itself in Up().
var builtins = []*start.Command{
	{Name: "help", Short: "Show help for folio or for a command"},
	{Name: "version", Short: "Show folio's version"},
}

// Add registers c with start and records it for Help. Every folio command
// is registered through Add.
func Add(c *start.Command) error {
	if err := start.Add(c); err != nil {
		return err
	}
	commands = append(commands, c)
	return nil
}

// Help answers the command lines that start's own help gets wrong or
// leaves inconsistent, and reports whether it did:
//
//   - `folio`, `folio --help` and `folio help` print the same overview;
//   - `folio help <command>` and `folio <command> --help` print the same
//     help for that command;
//   - an unknown command, or an object without its subcommand, prints an
//     error and the help to stderr and fails the exit status.
//
// args are the arguments left after flag parsing; asked is whether --help
// was given. When Help reports false, start runs the command.
func Help(args []string, asked bool) bool {
	if len(args) > 0 && args[0] == "help" {
		args, asked = args[1:], true
	}
	if len(args) == 0 {
		overview(stdout)
		return true
	}
	path, c := lookup(args)
	switch {
	case c == nil:
		return fail(fmt.Sprintf("unknown command %q", strings.Join(path, " ")), nil)
	case !asked && hasChildren(c) && len(path) == 1:
		return fail(fmt.Sprintf("%q needs a subcommand", c.Name), c)
	case asked:
		usage(stdout, c)
		return true
	}
	if f := foreignFlag(c); f != "" {
		return fail(fmt.Sprintf("%q does not take --%s", fullName(c), f), c)
	}
	return false
}

// Usage prints the help for the command on the command line, or the
// overview, to stderr. It is pflag's Usage, shown after a flag error.
func Usage() {
	if args := flags.Args(); len(args) > 0 {
		if _, c := lookup(args); c != nil {
			usage(stderr, c)
			return
		}
	}
	overview(stderr)
}

// foreignFlag returns a flag given on the command line that belongs to
// another command than c, or "".
func foreignFlag(c *start.Command) string {
	g := globals()
	var foreign string
	flags.Visit(func(f *flag.Flag) {
		if foreign == "" && !slices.Contains(c.Flags, f.Name) && !slices.Contains(g, f.Name) {
			foreign = f.Name
		}
	})
	return foreign
}

// fail reports an error with the matching help on stderr.
func fail(msg string, c *start.Command) bool {
	failed.Store(true)
	fmt.Fprintf(stderr, "error: %s\n\n", msg)
	if c == nil {
		overview(stderr)
	} else {
		usage(stderr, c)
	}
	return true
}

// lookup finds the command that args name: a top-level command and, for an
// object, its subcommand. It returns the words it looked at and the command,
// or nil if there is none.
func lookup(args []string) ([]string, *start.Command) {
	c := find("", args[0])
	if c == nil || !hasChildren(c) || len(args) == 1 {
		return args[:1], c
	}
	return args[:2], find(c.Name, args[1])
}

func find(parent, name string) *start.Command {
	for _, c := range slices.Concat(commands, builtins) {
		if c.Parent == parent && c.Name == name {
			return c
		}
	}
	return nil
}

func hasChildren(c *start.Command) bool {
	return slices.ContainsFunc(commands, func(s *start.Command) bool { return s.Parent == c.Name })
}

func fullName(c *start.Command) string {
	return strings.TrimSpace(c.Parent + " " + c.Name)
}

func overview(w io.Writer) {
	all := slices.SortedFunc(slices.Values(slices.Concat(commands, builtins)),
		func(a, b *start.Command) int { return strings.Compare(fullName(a), fullName(b)) })
	fmt.Fprintf(w, "%s\n\nUsage:\n  folio <command> [<subcommand>] [flags] [args]\n\nCommands:\n", Description)
	list(w, all, fullName)
	fmt.Fprintf(w, "\nGlobal flags:\n%s\n", flagUsages(globals()))
	fmt.Fprintln(w, `Run "folio help <command>" or "folio <command> --help" for a command's own flags.`)
}

func usage(w io.Writer, c *start.Command) {
	fmt.Fprintf(w, "folio %s: %s\n", fullName(c), c.Short)
	if c.Long != "" {
		fmt.Fprintf(w, "\n%s\n", wrap(c.Long, 80))
	}
	var subs []*start.Command
	for _, s := range commands {
		if s.Parent == c.Name && c.Parent == "" {
			subs = append(subs, s)
		}
	}
	if len(subs) > 0 {
		fmt.Fprintln(w, "\nSubcommands:")
		list(w, subs, func(s *start.Command) string { return s.Name })
		fmt.Fprintf(w, "\nRun \"folio help %s <subcommand>\" for a subcommand's flags.\n", c.Name)
		return
	}
	g := globals()
	var own []string
	for _, f := range c.Flags {
		if !slices.Contains(g, f) {
			own = append(own, f)
		}
	}
	if len(own) > 0 {
		fmt.Fprintf(w, "\nFlags:\n%s", flagUsages(own))
	}
	fmt.Fprintf(w, "\nGlobal flags:\n%s", flagUsages(g))
}

// list prints commands as a two-column table of name and summary.
func list(w io.Writer, cs []*start.Command, name func(*start.Command) string) {
	width := 0
	for _, c := range cs {
		width = max(width, len(name(c)))
	}
	for _, c := range cs {
		fmt.Fprintf(w, "  %-*s  %s\n", width, name(c), c.Short)
	}
}

// globals returns the flags no command claims in its Flags. start treats
// these as global and accepts them with every command.
func globals() []string {
	var names []string
	flags.VisitAll(func(f *flag.Flag) {
		if !slices.ContainsFunc(commands, func(c *start.Command) bool { return slices.Contains(c.Flags, f.Name) }) {
			names = append(names, f.Name)
		}
	})
	return names
}

func flagUsages(names []string) string {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	for _, n := range names {
		if f := flags.Lookup(n); f != nil {
			fs.AddFlag(f)
		}
	}
	return fs.FlagUsagesWrapped(80)
}

// wrap breaks text into lines of at most width columns, at spaces.
func wrap(text string, width int) string {
	var b strings.Builder
	n := 0
	for _, word := range strings.Fields(text) {
		switch {
		case n == 0:
		case n+1+len(word) > width:
			b.WriteByte('\n')
			n = 0
		default:
			b.WriteByte(' ')
			n++
		}
		b.WriteString(word)
		n += len(word)
	}
	return b.String()
}
