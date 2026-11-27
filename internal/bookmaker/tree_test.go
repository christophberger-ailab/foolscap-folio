package bookmaker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
)

// quarto leaves out what Quarto does, as the test projects expect.
var quarto, _ = config.NewIgnore(config.DefaultIgnore, "")

const (
	testProject = "../../testdata/project"
	testBook    = testProject + "/book"
)

func loadTestTree(t *testing.T) *Node {
	t.Helper()
	root, err := LoadTree(testBook, testProject, quarto)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoadTreeOrderAndDepths(t *testing.T) {
	root := loadTestTree(t)

	type entry struct {
		rel   string
		depth int
	}
	var got []entry
	root.Walk(func(n *Node) { got = append(got, entry{n.Rel, n.Depth}) })

	want := []entry{
		{"book", 0},
		// Siblings sort by `order:`; a directory takes the order of its
		// index.qmd, which is Quarto's "index sorts at the parent level".
		{"book/erster", 1},
		{"book/erster/alpha", 2},
		{"book/erster/alpha/tief", 3},
		{"book/erster/beta", 2},
		{"book/zweiter", 1},
		{"book/dritter_FW", 1},
		{"book/dritter_FW/seite", 2},
	}

	if len(got) != len(want) {
		t.Fatalf("visited %d nodes, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("node %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLoadTreeSkipsHiddenAndUnderscore(t *testing.T) {
	root := loadTestTree(t)
	root.Walk(func(n *Node) {
		if strings.Contains(n.Rel, "_entwurf") || strings.Contains(n.Rel, ".hidden") {
			t.Errorf("%s should have been skipped", n.Rel)
		}
	})
}

func TestLoadTreePrunesContentFreeDirectories(t *testing.T) {
	root := loadTestTree(t)
	root.Walk(func(n *Node) {
		if filepath.Base(n.Rel) == "images" {
			t.Errorf("media directory %s should have been pruned", n.Rel)
		}
	})
}

func TestLoadTreeAudience(t *testing.T) {
	root := loadTestTree(t)
	want := map[string]string{
		"book":                  "",
		"book/erster":           "",
		"book/dritter_FW":       "fw",
		"book/dritter_FW/seite": "",
	}
	root.Walk(func(n *Node) {
		if w, ok := want[n.Rel]; ok && n.Audience != w {
			t.Errorf("%s: audience = %q, want %q", n.Rel, n.Audience, w)
		}
	})
}

func TestLoadTreeRejectsFiles(t *testing.T) {
	if _, err := LoadTree(testBook+"/index.qmd", testProject, quarto); err == nil {
		t.Fatal("expected an error for a file argument")
	}
}

func TestHasProjectConfig(t *testing.T) {
	if !HasProjectConfig(testProject) {
		t.Error("HasProjectConfig should be true for the project root")
	}
	if HasProjectConfig(testBook) {
		t.Error("HasProjectConfig should be false for a content folder")
	}
}

// TestBookFolders checks the folders --all picks up: content folders yes,
// media folders and nested projects no.
func TestBookFolders(t *testing.T) {
	got, err := BookFolders(testProject)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture project holds `book` (content) and `assets` (media only).
	want := []string{filepath.Join(testProject, "book")}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("BookFolders = %v, want %v", got, want)
	}
}

func TestBookFoldersSkipsHiddenAndNestedProjects(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "buch", "index.qmd"), "# Buch\n")
	writeFile(t, filepath.Join(dir, "_entwurf", "index.qmd"), "# Entwurf\n")
	writeFile(t, filepath.Join(dir, ".hidden", "index.qmd"), "# Versteckt\n")
	writeFile(t, filepath.Join(dir, "eigenes", "folio.yml"), "title: Eigenes\n")
	writeFile(t, filepath.Join(dir, "eigenes", "index.qmd"), "# Eigenes\n")
	writeFile(t, filepath.Join(dir, "bilder", "x.png"), "")

	got, err := BookFolders(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "buch")}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("BookFolders = %v, want %v", got, want)
	}
}

func TestHumanise(t *testing.T) {
	tests := []struct{ in, want string }{
		{"einsatz-starten", "Einsatz starten"},
		{"betriebszustaende_FW", "Betriebszustaende"},
		{"bma-uea", "Bma uea"},
		{"wartung_patches_updates", "Wartung patches updates"},
	}
	for _, tc := range tests {
		if got := humanise(tc.in); got != tc.want {
			t.Errorf("humanise(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// .md pages make up a book just like .qmd pages, and a folder with neither
// is an error rather than a nil tree.
func TestLoadTreeMarkdown(t *testing.T) {
	dir := t.TempDir()
	for rel, content := range map[string]string{
		"book/index.md":     "---\ntitle: Book\n---\n",
		"book/a.qmd":        "---\ntitle: A\norder: 1\n---\n",
		"book/sub/index.md": "---\ntitle: Sub\norder: 2\n---\n",
		"empty/notes.txt":   "x",
	} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := LoadTree(filepath.Join(dir, "book"), dir, quarto)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	root.Walk(func(n *Node) { titles = append(titles, n.Title()) })
	if got := strings.Join(titles, ","); got != "Book,A,Sub" {
		t.Errorf("titles = %s, want Book,A,Sub", got)
	}
	if _, err := LoadTree(filepath.Join(dir, "empty"), dir, quarto); err == nil {
		t.Error("LoadTree of a folder without pages: want an error")
	}
	if got, _ := BookFolders(dir); len(got) != 1 {
		t.Errorf("BookFolders = %v, want only book", got)
	}
}

// Issue #181: the tree leaves out what the project's ignore patterns name,
// and without patterns nothing: "_" and "." are names like any other.
func TestLoadTreeIgnore(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{"book/index.md", "book/_a.md", "book/.b/index.md", "book/c/OLD/x.md", "book/d.md"} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ig, err := config.NewIgnore([]string{"**/OLD/", "/book/d.md"}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		ig   config.Ignore
		want string
	}{
		{config.Ignore{}, "book book/.b book/_a book/c book/c/OLD book/c/OLD/x book/d"},
		{quarto, "book book/c book/c/OLD book/c/OLD/x book/d"},
		{ig, "book book/.b book/_a"},
	} {
		root, err := LoadTree(filepath.Join(dir, "book"), dir, c.ig)
		if err != nil {
			t.Fatal(err)
		}
		var rels []string
		root.Walk(func(n *Node) { rels = append(rels, n.Rel) })
		if got := strings.Join(rels, " "); got != c.want {
			t.Errorf("tree %s, want %s", got, c.want)
		}
	}
}
