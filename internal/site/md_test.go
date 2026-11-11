package site

import (
	"path/filepath"
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
