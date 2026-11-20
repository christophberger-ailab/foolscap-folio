package project

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func load(t *testing.T, root string) *Tree {
	t.Helper()
	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func mustMove(t *testing.T, root, src, parent string, pos int) *Tree {
	t.Helper()
	if _, err := load(t, root).Move(src, parent, pos); err != nil {
		t.Fatal(err)
	}
	return load(t, root)
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func order(t *testing.T, p *Page) int {
	t.Helper()
	if p.Order == nil {
		t.Fatalf("page %s is unordered", p.Path)
	}
	return *p.Order
}

// Reordering within a group renumbers the whole group sequentially,
// including previously unordered pages.
func TestMoveReorderWithinGroup(t *testing.T) {
	root := writeFixture(t)
	tree := mustMove(t, root, "chapter2/third.qmd", "chapter2/index.qmd", 0)

	ch2 := tree.Find("chapter2/index.qmd")
	want := []string{"chapter2/third.qmd", "chapter2/second.qmd", "chapter2/fourth.qmd"}
	if got := paths(ch2.Children); !reflect.DeepEqual(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	for i, c := range ch2.Children {
		if order(t, c) != i+1 {
			t.Errorf("%s order = %d, want %d", c.Path, *c.Order, i+1)
		}
	}
}

// Moving a page up to the root renumbers both groups and leaves its
// headings as they are (issue #96); unordered pages in the source group
// stay unordered.
func TestMoveToRoot(t *testing.T) {
	root := writeFixture(t)
	tree := mustMove(t, root, "chapter2/second.qmd", "", 1)

	want := []string{"index.qmd", "second.qmd", "chapter1/index.qmd", "chapter2/index.qmd", "chapter3.qmd"}
	if got := paths(tree.Pages); !reflect.DeepEqual(got, want) {
		t.Fatalf("root = %v, want %v", got, want)
	}
	for i, p := range tree.Pages {
		if order(t, p) != i+1 {
			t.Errorf("%s order = %d, want %d", p.Path, *p.Order, i+1)
		}
	}

	content := readFile(t, root, "second.qmd")
	if !strings.Contains(content, "\n## Detail") {
		t.Errorf("headings changed:\n%s", content)
	}

	ch2 := tree.Find("chapter2/index.qmd")
	if got := paths(ch2.Children); !reflect.DeepEqual(got, []string{"chapter2/third.qmd", "chapter2/fourth.qmd"}) {
		t.Fatalf("chapter2 children = %v", got)
	}
	if order(t, ch2.Children[0]) != 1 {
		t.Errorf("third.qmd order = %d, want 1 (gap closed)", *ch2.Children[0].Order)
	}
	if ch2.Children[1].Order != nil {
		t.Errorf("fourth.qmd should stay unordered")
	}
}

// Dropping a page into a leaf turns the leaf into a section.
func TestMoveIntoLeaf(t *testing.T) {
	root := writeFixture(t)
	tree := mustMove(t, root, "chapter3/another.qmd", "chapter2/second.qmd", 0)

	second := tree.Find("chapter2/second.qmd")
	if got := paths(second.Children); !reflect.DeepEqual(got, []string{"chapter2/second/another.qmd"}) {
		t.Fatalf("second children = %v", got)
	}
	if !strings.Contains(readFile(t, root, "chapter2/second/another.qmd"), "\n# Another") {
		t.Error("headings changed")
	}
}

// Moving an index.qmd-style section moves its whole directory.
func TestMoveSectionIndexStyle(t *testing.T) {
	root := writeFixture(t)
	tree := mustMove(t, root, "chapter2/index.qmd", "chapter3.qmd", 0)

	want := []string{"index.qmd", "chapter1/index.qmd", "chapter3.qmd"}
	if got := paths(tree.Pages); !reflect.DeepEqual(got, want) {
		t.Fatalf("root = %v, want %v", got, want)
	}
	ch3 := tree.Find("chapter3.qmd")
	wantCh3 := []string{"chapter3/chapter2/index.qmd", "chapter3/another.qmd", "chapter3/yet-another.qmd", "chapter3/deep/index.qmd"}
	if got := paths(ch3.Children); !reflect.DeepEqual(got, wantCh3) {
		t.Fatalf("chapter3 children = %v, want %v", got, wantCh3)
	}
	if !strings.Contains(readFile(t, root, "chapter3/chapter2/index.qmd"), "\n# Two") {
		t.Error("section headings changed")
	}
	if !strings.Contains(readFile(t, root, "chapter3/chapter2/second.qmd"), "\n## Detail") {
		t.Error("descendant headings changed")
	}
}

// Moving a name.qmd-style section moves both the file and its directory.
func TestMoveSectionNameStyle(t *testing.T) {
	root := writeFixture(t)
	tree := mustMove(t, root, "chapter3.qmd", "chapter1/index.qmd", 0)

	ch1 := tree.Find("chapter1/index.qmd")
	if got := paths(ch1.Children); !reflect.DeepEqual(got, []string{"chapter1/chapter3.qmd"}) {
		t.Fatalf("chapter1 children = %v", got)
	}
	moved := tree.Find("chapter1/chapter3.qmd")
	wantKids := []string{"chapter1/chapter3/another.qmd", "chapter1/chapter3/yet-another.qmd", "chapter1/chapter3/deep/index.qmd"}
	if got := paths(moved.Children); !reflect.DeepEqual(got, wantKids) {
		t.Fatalf("moved children = %v, want %v", got, wantKids)
	}
}

func TestMoveIntoOwnSubtree(t *testing.T) {
	root := writeFixture(t)
	if _, err := load(t, root).Move("chapter3.qmd", "chapter3/deep/index.qmd", 0); err == nil {
		t.Error("want error moving a section into its own subtree")
	}
}

func TestMoveIntoRootIndex(t *testing.T) {
	root := writeFixture(t)
	if _, err := load(t, root).Move("chapter2/second.qmd", "index.qmd", 0); err == nil {
		t.Error("want error moving under the root index page")
	}
}

// CreatePage creates the new page as name/index.md, ordered after the
// last ordered sibling. The folder name is the typed name in lower case
// with dashes for spaces; the heading is the name as typed (issue #118),
// on one line, so that a pasted line break cannot reach the frontmatter.
func TestCreatePage(t *testing.T) {
	root := writeFixture(t)
	created, err := load(t, root).CreatePage("chapter2/index.qmd", " Fifth \n Page")
	if err != nil {
		t.Fatal(err)
	}
	if created != "chapter2/fifth-page/index.md" {
		t.Fatalf("created = %q", created)
	}
	tree := load(t, root)
	p := tree.Find("chapter2/fifth-page/index.md")
	if p == nil {
		t.Fatal("created page not in tree")
	}
	// The page carries no title of its own (issue #58): a `title:` would
	// override the heading the page starts with, and the tree names the
	// page by that heading.
	// It starts from the new-page template (issue #94): marked new, and an
	// explanation div headed by the page's name at its depth in the tree.
	if got, want := readFile(t, root, created), "---\norder: 3\nstatus: new\n---\n\n::: explanation\n# Fifth Page\n\n:::\n"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
	if p.Title != "Fifth Page" {
		t.Errorf("title = %q", p.Title)
	}
	if order(t, p) != 3 { // after third (order 2), before unordered fourth
		t.Errorf("order = %d, want 3", *p.Order)
	}
}

func TestCreatePageAtRoot(t *testing.T) {
	root := writeFixture(t)
	created, err := load(t, root).CreatePage("", "about")
	if err != nil {
		t.Fatal(err)
	}
	if created != "about/index.md" {
		t.Fatalf("created = %q", created)
	}
	if p := load(t, root).Find("about/index.md"); p == nil || order(t, p) != 5 {
		t.Fatalf("about/index.md = %+v", p)
	}
	if got, want := readFile(t, root, created), "---\norder: 5\nstatus: new\n---\n\n::: explanation\n# about\n\n:::\n"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// CreatePageAfter inserts the new page right after a sibling, renumbering
// the whole group so previously unordered pages become ordered.
func TestCreatePageAfter(t *testing.T) {
	root := writeFixture(t)
	created, err := load(t, root).CreatePageAfter("chapter2/second.qmd", "inserted")
	if err != nil {
		t.Fatal(err)
	}
	if created != "chapter2/inserted/index.md" {
		t.Fatalf("created = %q", created)
	}
	tree := load(t, root)
	ch2 := tree.Find("chapter2/index.qmd")
	want := []string{
		"chapter2/second.qmd", "chapter2/inserted/index.md",
		"chapter2/third.qmd", "chapter2/fourth.qmd",
	}
	if got := paths(ch2.Children); !reflect.DeepEqual(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	for i, c := range ch2.Children {
		if order(t, c) != i+1 {
			t.Errorf("%s order = %d, want %d", c.Path, *c.Order, i+1)
		}
	}
	if got, want := readFile(t, root, created), "---\norder: 2\nstatus: new\n---\n\n::: explanation\n# inserted\n\n:::\n"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// CreatePageAfter a root page inserts into the root group.
func TestCreatePageAfterAtRoot(t *testing.T) {
	root := writeFixture(t)
	created, err := load(t, root).CreatePageAfter("index.qmd", "intro")
	if err != nil {
		t.Fatal(err)
	}
	if created != "intro/index.md" {
		t.Fatalf("created = %q", created)
	}
	tree := load(t, root)
	want := []string{
		"index.qmd", "intro/index.md", "chapter1/index.qmd",
		"chapter2/index.qmd", "chapter3.qmd",
	}
	if got := paths(tree.Pages); !reflect.DeepEqual(got, want) {
		t.Fatalf("root pages = %v, want %v", got, want)
	}
	if got, want := readFile(t, root, created), "---\norder: 2\nstatus: new\n---\n\n::: explanation\n# intro\n\n:::\n"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestCreatePageExists(t *testing.T) {
	root := writeFixture(t)
	// chapter1/ already exists as a directory, so it is taken.
	if _, err := load(t, root).CreatePage("", "chapter1"); err == nil {
		t.Error("want error creating page whose folder exists")
	}
	// The name is checked after it is normalized: "Chapter3" is chapter3,
	// and chapter3.qmd is taken.
	if _, err := load(t, root).CreatePage("", "Chapter3"); err == nil {
		t.Error("want error creating page whose normalized name is taken")
	}
}

// noSystemTrash keeps test files out of the real system trash by forcing
// the in-project _trash fallback.
func noSystemTrash(t *testing.T) {
	t.Helper()
	saved := TrashCommands
	TrashCommands = nil
	t.Cleanup(func() { TrashCommands = saved })
}

func TestDeletePage(t *testing.T) {
	noSystemTrash(t)
	root := writeFixture(t)
	if err := load(t, root).DeletePage("chapter2/third.qmd"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "chapter2", "third.qmd")); !os.IsNotExist(err) {
		t.Error("third.qmd still exists")
	}
}

func TestDeleteSection(t *testing.T) {
	noSystemTrash(t)
	root := writeFixture(t)
	if err := load(t, root).DeletePage("chapter3.qmd"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "chapter3.qmd")); !os.IsNotExist(err) {
		t.Error("chapter3.qmd still exists")
	}
	if _, err := os.Stat(filepath.Join(root, "chapter3")); !os.IsNotExist(err) {
		t.Error("chapter3/ still exists")
	}
	if tree := load(t, root); tree.Find("chapter3/another.qmd") != nil {
		t.Error("children still in tree")
	}
}

// Move reports what it renamed, so a caller holding a page path from
// before the move can follow it to the file's new place.
func TestMoveReportsRenames(t *testing.T) {
	root := writeFixture(t)
	renames, err := load(t, root).Move("chapter2/second.qmd", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := Remap(renames, "chapter2/second.qmd"); got != "second.qmd" {
		t.Errorf("Remap of the moved page = %q, want %q", got, "second.qmd")
	}
	if got := Remap(renames, "chapter2/third.qmd"); got != "chapter2/third.qmd" {
		t.Errorf("Remap of an untouched page = %q, want it unchanged", got)
	}
}

// A section takes its whole directory along, so every page below it moves
// with it.
func TestMoveSectionRemapsPagesBelow(t *testing.T) {
	root := writeFixture(t)
	renames, err := load(t, root).Move("chapter2/index.qmd", "chapter3.qmd", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := Remap(renames, "chapter2/second.qmd"); got != "chapter3/chapter2/second.qmd" {
		t.Errorf("Remap of a page inside the section = %q", got)
	}
}

// A reorder within one sibling group renames nothing.
func TestMoveWithinGroupRenamesNothing(t *testing.T) {
	root := writeFixture(t)
	renames, err := load(t, root).Move("chapter2/third.qmd", "chapter2/index.qmd", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(renames) != 0 {
		t.Errorf("renames = %v, want none", renames)
	}
}

// Renaming a page changes its file name only: it stays in its directory,
// keeps its text, and keeps its place among its siblings.
func TestRenamePage(t *testing.T) {
	root := writeFixture(t)
	before := readFile(t, root, "chapter2/second.qmd")
	renames, err := load(t, root).RenamePage("chapter2/second.qmd", "zweite")
	if err != nil {
		t.Fatal(err)
	}
	if got := Remap(renames, "chapter2/second.qmd"); got != "chapter2/zweite.qmd" {
		t.Errorf("Remap of the renamed page = %q, want %q", got, "chapter2/zweite.qmd")
	}
	if _, err := os.Stat(filepath.Join(root, "chapter2", "second.qmd")); !os.IsNotExist(err) {
		t.Error("the old file is still there")
	}
	if got := readFile(t, root, "chapter2/zweite.qmd"); got != before {
		t.Errorf("text changed by the rename:\n%s", got)
	}
	tree := load(t, root)
	if p := tree.Find("chapter2/zweite.qmd"); p == nil || order(t, p) != 1 {
		t.Errorf("renamed page lost its place: %+v", p)
	}
}

// A page with children takes its children's directory along.
func TestRenamePageTakesChildrenAlong(t *testing.T) {
	root := writeFixture(t)
	renames, err := load(t, root).RenamePage("chapter3.qmd", "kapitel3.qmd")
	if err != nil {
		t.Fatal(err)
	}
	if got := Remap(renames, "chapter3/deep/leaf.qmd"); got != "kapitel3/deep/leaf.qmd" {
		t.Errorf("Remap of a page below = %q", got)
	}
	p := load(t, root).Find("kapitel3.qmd")
	if p == nil || len(p.Children) != 3 {
		t.Fatalf("renamed page lost its children: %+v", p)
	}
}

func TestRenamePageRefuses(t *testing.T) {
	for _, tc := range []struct{ page, name string }{
		{"chapter2/second.qmd", "third.qmd"},   // taken
		{"chapter2/second.qmd", "third"},       // taken, extension added
		{"chapter3/another.qmd", "deep"},       // its directory is taken
		{"chapter2/second.qmd", "sub/x.qmd"},   // not a file name
		{"chapter2/second.qmd", "_hidden.qmd"}, // the tree skips it
		{"chapter2/second.qmd", ".qmd"},        // no name
		{"chapter2/second.qmd", "index.qmd"},   // a section's own page
		{"index.qmd", "home.qmd"},              // the project's own page
		{"chapter2/index.qmd", "chapter3"},     // taken by a section and a file
		{"chapter3/deep/index.qmd", "another"}, // taken by a file
		{"chapter2/nope.qmd", "zwei.qmd"},      // no such page
	} {
		root := writeFixture(t)
		if _, err := load(t, root).RenamePage(tc.page, tc.name); err == nil {
			t.Errorf("RenamePage(%q, %q) succeeded, want an error", tc.page, tc.name)
		}
	}
}

// The name the page has already is no rename at all.
func TestRenamePageToItsOwnName(t *testing.T) {
	root := writeFixture(t)
	renames, err := load(t, root).RenamePage("chapter2/second.qmd", "second.qmd")
	if err != nil || len(renames) != 0 {
		t.Errorf("renames = %v, err = %v; want none and no error", renames, err)
	}
}

// A section page is named by its directory, which is renamed with the
// section's pages in it; index.qmd keeps its name.
func TestRenameSection(t *testing.T) {
	root := writeFixture(t)
	renames, err := load(t, root).RenamePage("chapter2/index.qmd", "kapitel2.qmd")
	if err != nil {
		t.Fatal(err)
	}
	for from, want := range map[string]string{
		"chapter2/index.qmd":  "kapitel2/index.qmd",
		"chapter2/second.qmd": "kapitel2/second.qmd",
	} {
		if got := Remap(renames, from); got != want {
			t.Errorf("Remap(%q) = %q, want %q", from, got, want)
		}
	}
	p := load(t, root).Find("kapitel2/index.qmd")
	if p == nil || len(p.Children) != 3 || order(t, p) != 3 {
		t.Fatalf("renamed section lost its children or place: %+v", p)
	}
}

func TestRenamePageMarkdown(t *testing.T) {
	root := t.TempDir()
	for rel, body := range map[string]string{
		"index.md":     "---\norder: 1\n---\n",
		"one.md":       "---\norder: 2\n---\n",
		"one/kid.md":   "---\norder: 1\n---\n",
		"sec/index.md": "---\norder: 3\n---\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	if _, err := load(t, root).RenamePage("index.md", "x"); err == nil {
		t.Error("root index.md renamed")
	}
	if _, err := load(t, root).RenamePage("one.md", "index"); err == nil {
		t.Error("index accepted")
	}
	if _, err := load(t, root).RenamePage("one.md", "index.md"); err == nil {
		t.Error("index.md accepted")
	}
	if _, err := load(t, root).RenamePage("one.md", "sec"); err == nil {
		t.Error("collision with sec/ accepted")
	}
	if _, err := load(t, root).RenamePage("one.md", "two"); err != nil {
		t.Fatal(err)
	}
	tree := load(t, root)
	two := tree.Find("two.md")
	if two == nil || len(two.Children) != 1 || two.Children[0].Path != "two/kid.md" {
		t.Fatalf("two.md = %+v", two)
	}
	if _, err := tree.RenamePage("two.md", "three.md"); err != nil {
		t.Fatal(err)
	}
	if load(t, root).Find("three.md") == nil {
		t.Error("three.md missing")
	}
	// A name taken by a .md file blocks creating a page.
	if _, err := load(t, root).CreatePage("", "three"); err == nil {
		t.Error("CreatePage over three.md accepted")
	}
}

func TestMoveMarkdownSection(t *testing.T) {
	root := t.TempDir()
	for rel, body := range map[string]string{
		"index.md":    "---\norder: 1\n---\n",
		"a/index.md":  "---\norder: 2\n---\n# A\n",
		"a/leaf.md":   "---\norder: 1\n---\n# Leaf\n",
		"b/index.qmd": "---\norder: 3\n---\n# B\n",
		"loose.md":    "---\norder: 4\n---\n# Loose\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	tree := mustMove(t, root, "a/index.md", "b/index.qmd", 0)
	if tree.Find("b/a/index.md") == nil || tree.Find("b/a/leaf.md") == nil {
		t.Fatal("section not moved")
	}
	if got := readFile(t, root, "b/a/leaf.md"); !strings.Contains(got, "\n# Leaf") {
		t.Errorf("headings changed: %q", got)
	}
	mustMove(t, root, "loose.md", "b/index.qmd", 0)
	if load(t, root).Find("b/loose.md") == nil {
		t.Error("loose.md not moved")
	}
}
