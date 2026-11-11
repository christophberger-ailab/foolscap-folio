package web

import (
	"os"
	"path/filepath"
	"testing"
)

// Plain .md pages are pages just like .qmd ones: the chat may read them,
// search indexes them, the tree refreshes on them, and completion offers
// them.
func TestMarkdownPagesAreProjectPages(t *testing.T) {
	srv, root := testServer(t)
	if !isProjectPage("chapter2/notes.md") {
		t.Error("isProjectPage(chapter2/notes.md) = false")
	}

	before := fingerprint(root)
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("# Zebrafish\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fingerprint(root) == before {
		t.Error("fingerprint ignores a new .md page")
	}
	if pages := indexPages(root).words["Zebrafish"]; len(pages["notes.md"]) == 0 {
		t.Errorf("search index misses notes.md: %v", pages)
	}
	if pages := completeNames(completeEntries(t, get(t, srv, "/complete?dir=.&kind=page"))); !contains(pages, "notes.md") {
		t.Errorf("page completion = %v, want notes.md", pages)
	}
}
