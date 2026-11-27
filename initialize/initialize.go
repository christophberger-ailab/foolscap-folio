// Package initialize implements the `folio init` subcommand.
//
// It writes a starter folio.yml into a Markdown tree that has none yet:
// the project title, every content folder as a topic, a website format, and
// the listen addresses of `folio web` and `folio preview`, with commented
// examples of what else the file can say. Beside it goes a sample
// _brand.yml, unless the project has one, and an empty folder first gets
// a sample project whose pages explain its structure.
package initialize

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/christophberger-ailab/foolscap-folio/internal/build"
	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/project"
	"github.com/christophberger/start"
	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// Register wires the `init` command into start. It shares --force and
// --dry-run with `migrate`, whichever of the two is registered first.
// userBrand returns the path of the user's global brand file.
func Register(projectFlag *string, userBrand func() string) {
	force := boolFlag("force", "Overwrite an existing folio.yml")
	dryRun := boolFlag("dry-run", "Print the folio.yml instead of writing it")
	demo := boolFlag("demo", "In an empty folder, write a demo project of folio's features")
	start.Add(&start.Command{
		Name:  "init",
		Short: "Write a starter folio.yml and _brand.yml, or start a new project",
		Long: "Write a folio.yml for a tree of .md or .qmd pages that has none yet: " +
			"the title (from the root index page, README.md or the folder name), " +
			"every content folder as a topic, a website format, and the web and " +
			"preview addresses, and a sample _brand.yml unless there is one. An " +
			"empty folder gets a home page beside them, the start of a new project; " +
			"with --demo it gets a demo project instead, with two topics that show " +
			"folio's Markdown extensions. Use --dry-run to print the folio.yml " +
			"instead, --force to overwrite an existing folio.yml. A project with " +
			"old _quarto*.yml files needs `folio migrate` instead.",
		Flags: []string{"force", "dry-run", "demo"},
		Cmd: func(*start.Command) error {
			root, err := filepath.Abs(*projectFlag)
			if err != nil {
				return fmt.Errorf("cannot resolve project path: %w", err)
			}
			res, err := Run(root, Options{Force: force(), DryRun: dryRun(), Demo: demo(), UserBrand: userBrand()})
			if err != nil {
				return err
			}
			if res.Written {
				for _, f := range append(res.Files, config.FileName) {
					fmt.Printf("wrote %s\n", filepath.Join(root, filepath.FromSlash(f)))
				}
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
	Demo   bool // start an empty folder from the demo project
	// UserBrand is the user's global brand file, which a project's own
	// overrides; "" for none.
	UserBrand string
}

// Result is what Run produced.
type Result struct {
	YAML    string
	Written bool
	// Files are the other files written, project-relative and
	// slash-separated: the sample project's and the brand file.
	Files []string
}

//go:embed all:sample
var sample embed.FS

//go:embed brand.yml
var brandText string

var brandTmpl = template.Must(template.New("brand").Parse(brandText))

// Run writes the starter folio.yml for the tree in root, and checks that it
// loads before it reports it as written. A folder that is empty, dot files
// aside, first gets the sample project. A project without a brand file
// gets a sample one.
func Run(root string, opts Options) (*Result, error) {
	if existing := config.Find(root); existing != "" && !opts.DryRun && !opts.Force {
		return nil, fmt.Errorf("%s exists; use --force to overwrite it", existing)
	}
	if old, _ := filepath.Glob(filepath.Join(root, "_quarto*.y*ml")); len(old) > 0 {
		return nil, fmt.Errorf("%s still has the old Quarto configuration (%s); run `folio migrate` to convert it",
			root, filepath.Base(old[0]))
	}
	empty, err := isEmpty(root)
	if err != nil {
		return nil, err
	}
	if opts.Demo && !empty {
		return nil, fmt.Errorf("%s is not empty: --demo writes its project into an empty folder only (dot files aside)", root)
	}
	dir := root
	if opts.DryRun {
		if dir, err = os.MkdirTemp("", "folio-init-"); err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
	}
	res, title := &Result{}, Title(root) // before the sample has a home page
	src := root
	if empty {
		if opts.Demo {
			res.Files, err = writeSample(dir)
		} else {
			res.Files, err = writeHome(dir, title)
		}
		if err != nil {
			return nil, err
		}
		src = dir
	}
	ig, err := config.NewIgnore(config.DefaultIgnore, "_output")
	if err != nil {
		return nil, err
	}
	topics, err := config.ContentFolders(src, ig)
	if err != nil {
		return nil, err
	}
	if len(topics) == 0 && !empty {
		return nil, fmt.Errorf("%s has no content folders (folders below it holding .md or .qmd pages)", root)
	}
	res.YAML = render(title, topics, opts.Demo)
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(res.YAML), 0o644); err != nil {
		return nil, err
	}
	if _, err := config.Load(dir); err != nil {
		return nil, fmt.Errorf("generated folio.yml does not load: %w", err)
	}
	if opts.DryRun {
		return &Result{YAML: res.YAML}, nil
	}
	wrote, err := writeBrand(root, opts.Demo, opts.UserBrand)
	if err != nil {
		return nil, err
	}
	if wrote {
		res.Files = append(res.Files, build.BrandFile)
	}
	res.Written = true
	return res, nil
}

// isEmpty reports whether dir holds nothing but dot files, such as .git.
func isEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			return false, nil
		}
	}
	return true, nil
}

// writeHome writes the home page of a new project into dir and returns it.
func writeHome(dir, title string) ([]string, error) {
	page := fmt.Sprintf("---\ntitle: %s\n---\n\n# %s\n\n"+
		"This is the home page of the project. Every folder of pages next to it becomes a topic: "+
		"a book of its own and an entry in the website's menu.\n", quote(title), title)
	return []string{"index.md"}, os.WriteFile(filepath.Join(dir, "index.md"), []byte(page), 0o644)
}

// writeSample copies the sample project into dir and returns its files.
func writeSample(dir string) ([]string, error) {
	sub, err := fs.Sub(sample, "sample")
	if err != nil {
		return nil, err
	}
	var files []string
	err = fs.WalkDir(sub, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(sub, name)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		files = append(files, name)
		return os.WriteFile(dst, data, 0o644)
	})
	return files, err
}

// writeBrand writes the sample brand file into root unless the project has
// one, and reports whether it did. The sample project's brand names its
// logo. If the user's global brand file exists, the sample warns that it
// overrides it.
func writeBrand(root string, logo bool, user string) (bool, error) {
	for _, name := range []string{build.BrandFile, "_brand.yaml"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return false, nil
		}
	}
	var b strings.Builder
	data := struct {
		Logo   bool
		Global string
	}{Logo: logo}
	if user != "" {
		if st, err := os.Stat(user); err == nil && !st.IsDir() {
			data.Global = user
		}
	}
	if err := brandTmpl.Execute(&b, data); err != nil {
		return false, err
	}
	if err := build.CheckBrand([]byte(b.String())); err != nil {
		return false, fmt.Errorf("sample %s does not load: %w", build.BrandFile, err)
	}
	f, err := os.OpenFile(filepath.Join(root, build.BrandFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return false, err
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
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

// render writes the folio.yml. The demo's shows a document format, two
// audiences and a variable in use, where any other's has them as comments.
// Without topics, the list is a comment, so that every content folder
// added later is a topic.
func render(title string, topics []string, demo bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, `# folio.yml, written by `+"`folio init`"+`. Every key is optional. Apart from the
# title, what follows is what folio uses when the file does not say: delete
# what you do not want to pin down. The README's "folio.yml" section has the rest.

title: %s
# lang: en
# output: _output
# footer: "{title} · {date}"

# The paths renders, previews and folio web leave out, as in a .gitignore.
# Without this list, folio leaves out nothing but its output and .git.
ignore: [%s]
`, quote(title), quoteList(config.DefaultIgnore))
	if demo {
		b.WriteString(`
# The website shows the "explanation" blocks, the handout does not.
formats:
  web: {type: html, show: [explanation], numbered_figures: true}
  handout: {type: [docx, pdf], toc: true, numbered_headings: true, numbered_figures: true}

# A block or span with the class "experts" is for the experts alone.
audiences:
  everyone: {}
  experts: {}

# {{product}} in a page shows this value.
vars:
  product: Foolscap Folio
`)
	} else {
		b.WriteString(`
formats:
  web: {type: html}
  # handout: {type: [docx, pdf], toc: true, template: assets/Template.docx}

# audiences:
#   std: {}
#   pol: {show: [pol], vars: {suffix: _POL}}
`)
	}
	b.WriteString(`
# One topic per content folder; each renders into its own document. While
# this list is here, a folder added later is a topic only once listed; without
# it, folio takes every content folder.
`)
	if len(topics) == 0 {
		b.WriteString("# topics:\n#   guide: {}\n")
	} else {
		b.WriteString("topics:\n")
	}
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

// quoteList renders l as the items of a YAML flow sequence, each quoted.
func quoteList(l []string) string {
	q := make([]string, len(l))
	for i, s := range l {
		q[i] = strconv.Quote(s)
	}
	return strings.Join(q, ", ")
}

// quote renders s as a YAML scalar, quoted only where it needs to be.
func quote(s string) string {
	out, _ := yaml.Marshal(s)
	return strings.TrimSuffix(string(out), "\n")
}
