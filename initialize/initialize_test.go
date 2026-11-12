package initialize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	for _, want := range []string{"port: 8199", "port: 8198", "baseurl: localhost", "# handout:"} {
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
	write(t, empty, "notes.txt", "x")
	if _, err := Run(empty, Options{}); err == nil || !strings.Contains(err.Error(), "no content folders") {
		t.Errorf("empty tree: err = %v", err)
	}
}

// Quarto's configuration files mean nothing to folio: a tree that still has
// one is initialized like any other.
func TestIgnoresQuartoFiles(t *testing.T) {
	root := tree(t)
	write(t, root, "_quarto.yml", "project: {type: website}\n")
	if _, err := Run(root, Options{}); err != nil {
		t.Errorf("tree with _quarto.yml: %v", err)
	}
}
