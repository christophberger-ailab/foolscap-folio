package site

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// A project written in plain .md files builds like one in .qmd files.
func TestBuildMarkdownPages(t *testing.T) {
	root, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	write(t, root, "index.md", "---\ntitle: Home\n---\n\nWelcome home.\n")
	write(t, root, "guide/index.md", "---\ntitle: Guide\n---\n\n# Guide\n\nIntro.\n")
	write(t, root, "guide/page.md", "---\ntitle: A Page\n---\n\n# A Page\n\n[back](index.md)\n")
	o := Options{Root: root, OutDir: out, Title: "T", Topics: []Topic{{Name: "g", Folder: "guide"}}}
	if err := Build(o); err != nil {
		t.Fatal(err)
	}
	has(t, "home", read(t, out, "index.html"), "Welcome home.")
	has(t, "page", read(t, out, "guide/page.html"), "A Page", `href="index.html"`)
}

// A topic folder without any page is an error, not a crash.
func TestBuildEmptyTopic(t *testing.T) {
	root, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	write(t, root, "guide/notes.txt", "not a page")
	o := Options{Root: root, OutDir: out, Title: "T", Topics: []Topic{{Name: "g", Folder: "guide"}}}
	if err := Build(o); err == nil {
		t.Fatal("want an error for a topic without pages")
	}
}

// A relative link that climbs out of the project root and back in through
// the root's own name -- written where the tree sits inside a larger one --
// resolves to the page it names, as it does on disk.
func TestBuildLinkThroughRootName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sources_md")
	out := filepath.Join(t.TempDir(), "out")
	write(t, root, "guide/page.md", "# Page\n\n[file](../../sources_md/some/path/file.md#sec) [dir](../../sources_md/some/)\n")
	write(t, root, "some/index.md", "# Some\n")
	write(t, root, "some/path/file.md", "# File\n\n## Sec\n")
	var logs []string
	o := Options{Root: root, OutDir: out, Title: "T",
		Topics: []Topic{{Name: "g", Folder: "guide"}, {Name: "s", Folder: "some"}},
		Log:    func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }}
	if err := Build(o); err != nil {
		t.Fatal(err)
	}
	has(t, "page", read(t, out, "guide/page.html"), `href="../some/path/file.html#sec"`, `href="../some/index.html"`)
	for _, l := range logs {
		if strings.HasPrefix(l, "warning") {
			t.Errorf("unexpected %s", l)
		}
	}
}

// A link to a page that exists but is not part of the site -- a folder that
// is no topic, or a topic not selected -- says so instead of "not found".
func TestBuildLinkOutsideSite(t *testing.T) {
	root, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	write(t, root, "guide/page.md", "# Page\n\n[other](../other/file.md)\n")
	write(t, root, "other/file.md", "# File\n")
	var logs []string
	o := Options{Root: root, OutDir: out, Title: "T", Topics: []Topic{{Name: "g", Folder: "guide"}},
		Log: func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }}
	if err := Build(o); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(logs, "\n"); !strings.Contains(got, `"../other/file.md" is not a page of the site`) {
		t.Errorf("want a not-in-site warning, got:\n%s", got)
	}
}
