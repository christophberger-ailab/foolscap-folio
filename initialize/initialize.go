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
	start.Add(&start.Command{
		Name:  "init",
		Short: "Write a starter folio.yml and _brand.yml, or a sample project",
		Long: "Write a folio.yml for a tree of .md or .qmd pages that has none yet: " +
			"the title (from the root index page, README.md or the folder name), " +
			"every content folder as a topic, a website format, and the web and " +
			"preview addresses, and a sample _brand.yml unless there is one. In an " +
			"empty folder it first writes a sample project to start from. Use " +
			"--dry-run to print the folio.yml instead, --force to overwrite an " +
			"existing folio.yml. A project with old _quarto*.yml files needs " +
			"`folio migrate` instead.",
		Flags: []string{"force", "dry-run"},
		Cmd: func(*start.Command) error {
			root, err := filepath.Abs(*projectFlag)
			if err != nil {
				return fmt.Errorf("cannot resolve project path: %w", err)
			}
			res, err := Run(root, Options{Force: force(), DryRun: dryRun(), UserBrand: userBrand()})
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
		if res.Files, err = writeSample(dir); err != nil {
			return nil, err
		}
		src = dir
	}
	topics, err := config.ContentFolders(src)
	if err != nil {
		return nil, err
	}
	if len(topics) == 0 {
		return nil, fmt.Errorf("%s has no content folders (folders below it holding .md or .qmd pages)", root)
	}
	res.YAML = render(title, topics)
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(res.YAML), 0o644); err != nil {
		return nil, err
	}
	if _, err := config.Load(dir); err != nil {
		return nil, fmt.Errorf("generated folio.yml does not load: %w", err)
	}
	if opts.DryRun {
		return &Result{YAML: res.YAML}, nil
	}
	wrote, err := writeBrand(root, empty, opts.UserBrand)
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

func render(title string, topics []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `# folio.yml, written by `+"`folio init`"+`. Every key is optional. Apart from the
# title, what follows is what folio uses when the file does not say: delete
# what you do not want to pin down. The README's "folio.yml" section has the rest.

title: %s
# lang: en
# output: _output
# footer: "{title} · {date}"

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
