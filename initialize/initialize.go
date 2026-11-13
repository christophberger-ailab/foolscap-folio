// Package initialize implements the `folio init` subcommand.
//
// It writes a starter folio.yml into a Markdown tree that has none yet:
// the project title, every content folder as a topic, a website format, and
// the listen addresses of `folio web` and `folio preview`, with commented
// examples of what else the file can say.
package initialize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/project"
	"github.com/christophberger/start"
	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// Register wires the `init` command into start. It shares --force and
// --dry-run with `migrate`, whichever of the two is registered first.
func Register(projectFlag *string) {
	force := boolFlag("force", "Overwrite an existing folio.yml")
	dryRun := boolFlag("dry-run", "Print the folio.yml instead of writing it")
	start.Add(&start.Command{
		Name:  "init",
		Short: "Write a starter folio.yml into a Markdown tree",
		Long: "Write a folio.yml for a tree of .md or .qmd pages that has none yet: " +
			"the title (from the root index page, README.md or the folder name), " +
			"every content folder as a topic, a website format, and the web and " +
			"preview addresses. Use --dry-run to print it instead, --force to " +
			"overwrite an existing folio.yml. A project with old _quarto*.yml " +
			"files needs `folio migrate` instead.",
		Flags: []string{"force", "dry-run"},
		Cmd: func(*start.Command) error {
			root, err := filepath.Abs(*projectFlag)
			if err != nil {
				return fmt.Errorf("cannot resolve project path: %w", err)
			}
			res, err := Run(root, Options{Force: force(), DryRun: dryRun()})
			if err != nil {
				return err
			}
			if res.Written {
				fmt.Printf("wrote %s\n", filepath.Join(root, config.FileName))
			} else {
				fmt.Print(res.YAML)
			}
			return nil
		},
	})
}

// boolFlag defines the bool flag name, or reuses it when another command
// already did: a flag can only be defined once.
func boolFlag(name, usage string) func() bool {
	f := flag.Lookup(name)
	if f == nil {
		flag.Bool(name, false, usage)
		f = flag.Lookup(name)
	}
	return func() bool { return f.Value.String() == "true" }
}

// Options control Run.
type Options struct {
	Force  bool // overwrite an existing folio.yml
	DryRun bool // do not write the file
}

// Result is what Run produced.
type Result struct {
	YAML    string
	Written bool
}

// Run writes the starter folio.yml for the tree in root, and checks that it
// loads before it reports it as written.
func Run(root string, opts Options) (*Result, error) {
	if existing := config.Find(root); existing != "" && !opts.DryRun && !opts.Force {
		return nil, fmt.Errorf("%s exists; use --force to overwrite it", existing)
	}
	if old, _ := filepath.Glob(filepath.Join(root, "_quarto*.y*ml")); len(old) > 0 {
		return nil, fmt.Errorf("%s still has the old Quarto configuration (%s); run `folio migrate` to convert it",
			root, filepath.Base(old[0]))
	}
	topics, err := config.ContentFolders(root)
	if err != nil {
		return nil, err
	}
	if len(topics) == 0 {
		return nil, fmt.Errorf("%s has no content folders (folders below it holding .md or .qmd pages)", root)
	}
	res := &Result{YAML: render(Title(root), topics)}

	dir := root
	if opts.DryRun {
		if dir, err = os.MkdirTemp("", "folio-init-"); err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
	}
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(res.YAML), 0o644); err != nil {
		return nil, err
	}
	if _, err := config.Load(dir); err != nil {
		return nil, fmt.Errorf("generated folio.yml does not load: %w", err)
	}
	res.Written = !opts.DryRun
	return res, nil
}

// Title is the project's title: the title or first heading of the root
// index page, else the first heading of README.md, else the folder's name
// in words.
func Title(root string) string {
	for _, name := range []string{"index.qmd", "index.md", "README.md"} {
		src, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		if t := project.ParseFrontmatter(src).Title; t != "" {
			return t
		}
		if t := project.FirstHeading(src); t != "" {
			return t
		}
	}
	words := strings.FieldsFunc(filepath.Base(root), func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
	for i, w := range words {
		r := []rune(w)
		words[i] = string(unicode.ToUpper(r[0])) + string(r[1:])
	}
	return strings.Join(words, " ")
}

func render(title string, topics []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `# folio.yml, written by `+"`folio init`"+`. Every key is optional. Apart from the
# title, what follows is what folio uses when the file does not say: delete
# what you do not want to pin down. The README's "folio.yml" section has the rest.

title: %s
# lang: en
# output: _output

formats:
  web: {type: html}
  # handout: {type: [docx, pdf], toc: true, template: assets/Template.docx}

# audiences:
#   std: {}
#   pol: {show: [pol], vars: {suffix: _POL}}

# One topic per content folder; each renders into its own document. While
# this list is here, a folder added later is a topic only once listed; without
# it, folio takes every content folder.
topics:
`, quote(title))
	for _, t := range topics {
		fmt.Fprintf(&b, "  %s: {}\n", quote(t))
	}
	fmt.Fprintf(&b, `
# Where `+"`folio web`"+` and `+"`folio preview`"+` listen. Give each project its own
# ports to serve several side by side.
web:
  baseurl: %s
  port: %d
preview:
  baseurl: %s
  port: %d
`, config.DefaultWeb.BaseURL, config.DefaultWeb.Port, config.DefaultPreview.BaseURL, config.DefaultPreview.Port)
	return b.String()
}

// quote renders s as a YAML scalar, quoted only where it needs to be.
func quote(s string) string {
	out, _ := yaml.Marshal(s)
	return strings.TrimSuffix(string(out), "\n")
}
