package initialize

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/build"
	"github.com/christophberger-ailab/foolscap-folio/internal/config"
)

func write(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func tree(t *testing.T) string {
	root := t.TempDir()
	write(t, root, "guide/index.md", "# Guide\n")
	write(t, root, "reference/api/calls.qmd", "# Calls\n")
	write(t, root, "assets/logo.png", "png")
	write(t, root, "_drafts/x.md", "# Draft\n")
	return root
}

func TestWritesLoadableConfig(t *testing.T) {
	root := tree(t)
	write(t, root, "index.md", "---\ntitle: \"Ops: Handbook\"\n---\n\n# Ignored\n")
	res, err := Run(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Written {
		t.Fatal("not written")
	}
	c, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.Path != filepath.Join(root, config.FileName) {
		t.Errorf("loaded from %q", c.Path)
	}
	if c.Title != "Ops: Handbook" {
		t.Errorf("title %q", c.Title)
	}
	var topics []string
	for _, tp := range c.Topics {
		topics = append(topics, tp.Name)
	}
	if strings.Join(topics, ",") != "guide,reference" {
		t.Errorf("topics %v", topics)
	}
	if f, ok := c.Formats.Get("web"); !ok || !f.IsWeb() || len(c.Formats) != 1 {
		t.Errorf("formats %+v", c.Formats)
	}
	if c.Web != config.DefaultWeb || c.Preview != config.DefaultPreview {
		t.Errorf("web %+v preview %+v", c.Web, c.Preview)
	}
	for _, want := range []string{"port: 8199", "port: 8198", "baseurl: localhost", "# handout:", `ignore: ["_*", ".*"]`} {
		if !strings.Contains(res.YAML, want) {
			t.Errorf("folio.yml lacks %q:\n%s", want, res.YAML)
		}
	}
}

func TestTitle(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"heading of index.qmd": {"index.qmd": "Intro\n\n# From Heading\n"},
		"heading of README.md": {"README.md": "# From Heading\n"},
		"directory name":       {},
		"unicode directory":    {},
	} {
		t.Run(name, func(t *testing.T) {
			dir := "my-docs_tree"
			if name == "unicode directory" {
				dir = "über-uns"
			}
			root := filepath.Join(tree(t), dir)
			write(t, root, "guide/index.md", "# Guide\n")
			for n, b := range files {
				write(t, root, n, b)
			}
			want := "From Heading"
			switch name {
			case "directory name":
				want = "My Docs Tree"
			case "unicode directory":
				want = "Über Uns"
			}
			if got := Title(root); got != want {
				t.Errorf("Title = %q, want %q", got, want)
			}
		})
	}
}

func TestRefusesExistingConfig(t *testing.T) {
	for _, name := range []string{config.FileName, config.LegacyFileName} {
		root := tree(t)
		write(t, root, name, "title: Mine\n")
		if _, err := Run(root, Options{}); err == nil || !strings.Contains(err.Error(), "--force") {
			t.Errorf("%s: err = %v", name, err)
		}
		if b, _ := os.ReadFile(filepath.Join(root, name)); string(b) != "title: Mine\n" {
			t.Errorf("%s was changed: %q", name, b)
		}
		if _, err := Run(root, Options{DryRun: true}); err != nil {
			t.Errorf("%s: dry run: %v", name, err)
		}
	}
	root := tree(t)
	write(t, root, config.FileName, "title: Mine\n")
	if _, err := Run(root, Options{Force: true}); err != nil {
		t.Fatal(err)
	}
	if c, err := config.Load(root); err != nil || c.Title == "Mine" {
		t.Errorf("--force did not overwrite: %v %+v", err, c)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	root := tree(t)
	res, err := Run(root, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Written || !strings.Contains(res.YAML, "guide:") {
		t.Errorf("written %v, yaml:\n%s", res.Written, res.YAML)
	}
	if _, err := os.Stat(filepath.Join(root, config.FileName)); !os.IsNotExist(err) {
		t.Errorf("dry run wrote folio.yml: %v", err)
	}
}

func TestRejectsUnsuitableTrees(t *testing.T) {
	empty := t.TempDir()
	write(t, empty, "notes.txt", "x") // not empty, so no sample project
	if _, err := Run(empty, Options{}); err == nil || !strings.Contains(err.Error(), "no content folders") {
		t.Errorf("empty tree: err = %v", err)
	}
	quarto := tree(t)
	write(t, quarto, "_quarto.yml", "project: {type: website}\n")
	if _, err := Run(quarto, Options{}); err == nil || !strings.Contains(err.Error(), "folio migrate") {
		t.Errorf("Quarto tree: err = %v", err)
	}
}

// Init writes a sample _brand.yml beside folio.yml, and leaves one that is
// there alone.
func TestWritesBrand(t *testing.T) {
	root := tree(t)
	res, err := Run(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, build.BrandFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := build.CheckBrand(b); err != nil {
		t.Errorf("sample brand does not load: %v", err)
	}
	if strings.Contains(string(b), "\nlogo:") {
		t.Errorf("brand names a logo the project does not have:\n%s", b)
	}
	if !slices.Contains(res.Files, build.BrandFile) {
		t.Errorf("files %q", res.Files)
	}

	write(t, root, build.BrandFile, "color: {primary: red}\n")
	res, err = Run(root, Options{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, build.BrandFile)); string(b) != "color: {primary: red}\n" || slices.Contains(res.Files, build.BrandFile) {
		t.Errorf("existing brand overwritten: %q, files %q", b, res.Files)
	}

	root = tree(t)
	if _, err := Run(root, Options{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, build.BrandFile)); !os.IsNotExist(err) {
		t.Errorf("dry run wrote the brand: %v", err)
	}
}

// In an empty folder -- dot files aside -- init starts a new project:
// folio.yml, a home page and the brand, and no topic yet. A content folder
// added later is a topic without a change to folio.yml.
func TestNewProject(t *testing.T) {
	root := filepath.Join(t.TempDir(), "my-handbook")
	write(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	res, err := Run(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Written || !slices.Equal(res.Files, []string{"index.md", build.BrandFile}) {
		t.Errorf("written %v, files %q", res.Written, res.Files)
	}
	c, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "My Handbook" || len(c.Topics) != 0 {
		t.Errorf("title %q, topics %+v", c.Title, c.Topics)
	}
	home, _ := os.ReadFile(filepath.Join(root, "index.md"))
	if !strings.Contains(string(home), "title: My Handbook") || !strings.Contains(string(home), "# My Handbook") {
		t.Errorf("index.md:\n%s", home)
	}
	if b, _ := os.ReadFile(filepath.Join(root, build.BrandFile)); strings.Contains(string(b), "\nlogo:") {
		t.Errorf("brand names a logo the project does not have:\n%s", b)
	}
	write(t, root, "guide/index.md", "---\ntitle: Guide\n---\n\n# Guide\n")
	if c, err := config.Load(root); err != nil || len(c.Topics) != 1 || c.Topics[0].Name != "guide" {
		t.Errorf("added folder is no topic: %v %+v", err, c)
	}

	empty := t.TempDir()
	res, err = Run(empty, Options{DryRun: true})
	if err != nil || !strings.Contains(res.YAML, "title:") || res.Written {
		t.Fatalf("dry run: %v\n%s", err, res.YAML)
	}
	if entries, _ := os.ReadDir(empty); len(entries) != 0 {
		t.Errorf("dry run wrote %d entries", len(entries))
	}
}

// --demo starts from a demo project in an empty folder only.
func TestDemoNeedsAnEmptyFolder(t *testing.T) {
	root := tree(t)
	if _, err := Run(root, Options{Demo: true}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, config.FileName)); !os.IsNotExist(err) {
		t.Errorf("folio.yml written: %v", err)
	}
}

// With --demo, an empty folder gets the demo project: a home page, two
// topics with sections, chapters and subchapters, a logo and its brand,
// and the formats and audiences that the pages' conditional content uses.
// Every part of the matrix renders without a warning.
func TestDemoProject(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	res, err := Run(root, Options{Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"index.md", "handbook/index.md", "handbook/writing/index.md",
		"showcase/index.md", "showcase/text/diagrams/sizes.md", "assets/logo.svg", build.BrandFile} {
		if !slices.Contains(res.Files, f) {
			t.Errorf("%s not reported among %q", f, res.Files)
		}
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Error(err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(root, build.BrandFile)); !strings.Contains(string(b), "assets/logo.svg") {
		t.Errorf("sample brand lacks the logo:\n%s", b)
	}
	c, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != filepath.Base(root) { // TempDir's numbered folder
		t.Errorf("title %q, want the folder's name", c.Title)
	}
	var topics []string
	for _, tp := range c.Topics {
		topics = append(topics, tp.Name)
	}
	if strings.Join(topics, ",") != "handbook,showcase" {
		t.Errorf("topics %v", topics)
	}
	if len(c.Formats) < 2 || len(c.Audiences) < 2 {
		t.Errorf("formats %+v audiences %+v", c.Formats, c.Audiences)
	}
	sels, err := c.Expand(config.Request{})
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	if err := build.Run(build.Options{Selections: sels}, func(f string, a ...any) { log = append(log, fmt.Sprintf(f, a...)) }); err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(log, "\n"))
	}
	for _, l := range log {
		if strings.Contains(strings.ToLower(l), "warning") || strings.Contains(l, "note:") {
			t.Errorf("render: %s", l)
		}
	}
	page, err := os.ReadFile(filepath.Join(root, "_output", "web-everyone", "handbook", "writing", "index.html"))
	if err != nil || !strings.Contains(string(page), "site-logo") {
		t.Errorf("section page without the logo: %v", err)
	}

	// A dry run shows the demo's folio.yml and writes nothing.
	empty := t.TempDir()
	res, err = Run(empty, Options{Demo: true, DryRun: true})
	if err != nil || !strings.Contains(res.YAML, "handbook:") {
		t.Fatalf("dry run: %v\n%s", err, res.YAML)
	}
	if entries, _ := os.ReadDir(empty); len(entries) != 0 {
		t.Errorf("dry run wrote %d entries", len(entries))
	}
}

// With a global brand file, the project's sample brand warns up front
// that it overrides that one; without one, it does not.
func TestBrandWarnsAboutGlobalBrand(t *testing.T) {
	global := filepath.Join(t.TempDir(), build.BrandFile)
	for name, c := range map[string]struct {
		user string
		warn bool
	}{
		"global":    {global, true},
		"no file":   {filepath.Join(t.TempDir(), build.BrandFile), false},
		"no config": {"", false},
	} {
		write(t, filepath.Dir(global), build.BrandFile, "color: {primary: red}\n")
		root := tree(t)
		if _, err := Run(root, Options{UserBrand: c.user}); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(filepath.Join(root, build.BrandFile))
		head := string(b)[:min(len(b), 600)]
		if got := strings.Contains(head, "WARNING") && strings.Contains(head, global); got != c.warn {
			t.Errorf("%s: warning %v, want %v:\n%s", name, got, c.warn, b)
		}
		if err := build.CheckBrand(b); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
