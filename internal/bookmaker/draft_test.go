package bookmaker

import (
	"path/filepath"
	"strings"
	"testing"
)

func draftBook(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "book", "index.md"), "---\ntitle: Book\n---\nIntro\n")
	writeFile(t, filepath.Join(dir, "book", "a.md"), "---\ntitle: Alpha\norder: 1\nreview: Check   the\n  numbers\nnew: true\n---\nAlpha text\n")
	writeFile(t, filepath.Join(dir, "book", "b.md"), "---\norder: 2\nfolio:\n  review: true\n  status: new\n---\n# Beta\n\nBeta text\n")
	writeFile(t, filepath.Join(dir, "book", "c.md"), "---\ntitle: Gamma\norder: 3\ndraft: true\n---\nGamma text\n")
	writeFile(t, filepath.Join(dir, "book", "d", "index.md"), "---\ntitle: Delta\norder: 4\ndraft: true\n---\nDelta text\n")
	writeFile(t, filepath.Join(dir, "book", "d", "e.md"), "---\ntitle: Epsilon\n---\nEpsilon text\n")
	return dir
}

// A draft shows a page's review and its being new below its title.
func TestDraftBanner(t *testing.T) {
	dir := draftBook(t)
	tree, err := LoadTree(filepath.Join(dir, "book"), dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Flatten(tree, Options{ProjectRoot: dir, Draft: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Alpha {#",
		"}\n\n🆕\n\n::: {.callout-warning .folio-review title=\"\"}\n**Review:** Check the numbers\n:::\n\nAlpha text",
		"# Beta {#",
		"}\n\n🆕\n\n::: {.callout-warning .folio-review title=\"\"}\n**Needs review**\n:::\n\nBeta text",
	} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("missing %q in\n%s", want, res.Content)
		}
	}
	res, _ = Flatten(tree, Options{ProjectRoot: dir})
	if strings.Contains(res.Content, "🆕") || strings.Contains(res.Content, "Review") {
		t.Errorf("banner without draft:\n%s", res.Content)
	}
}

// Pages marked as drafts are left out, a folder whose index page is one
// with all below it.
func TestDropDrafts(t *testing.T) {
	dir := draftBook(t)
	tree, err := LoadTree(filepath.Join(dir, "book"), dir)
	if err != nil {
		t.Fatal(err)
	}
	tree.DropDrafts()
	res, err := Flatten(tree, Options{ProjectRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"Gamma", "Delta", "Epsilon"} {
		if strings.Contains(res.Content, gone) {
			t.Errorf("draft %s kept:\n%s", gone, res.Content)
		}
	}
	if !strings.Contains(res.Content, "Alpha text") {
		t.Errorf("page lost:\n%s", res.Content)
	}
}
