package web

import (
	"net/url"
	"regexp"
	"testing"
)

// modifiedRow finds the tree row of rel and reports whether it carries
// the modified mark.
func modifiedRow(t *testing.T, body, rel string) bool {
	t.Helper()
	row := regexp.MustCompile(`(?s)<li class="page[^"]*" data-path="` + regexp.QuoteMeta(rel) + `".*?</div>`).FindString(body)
	if row == "" {
		t.Fatalf("no row for %s in:\n%s", rel, body)
	}
	return regexp.MustCompile(`class="modified"`).MatchString(row)
}

// UI.1-8: a page changed since the last commit is marked with an asterisk
// after its title, and the mark goes once the change is committed.
func TestTreeMarksModified(t *testing.T) {
	srv, root := gitProject(t)
	if body := get(t, srv, "/tree").Body.String(); modifiedRow(t, body, "chapter2/second.qmd") {
		t.Errorf("clean page marked modified")
	}
	writeFile(t, root, "chapter2/second.qmd", "---\ntitle: Second\norder: 1\n---\n# Edited\n")
	body := get(t, srv, "/tree").Body.String()
	if !modifiedRow(t, body, "chapter2/second.qmd") {
		t.Errorf("edited page not marked modified")
	}
	if modifiedRow(t, body, "chapter2/third.qmd") {
		t.Errorf("untouched page marked modified")
	}
	post(t, srv, "/git/stage", url.Values{"all": {"1"}})
	if !modifiedRow(t, get(t, srv, "/tree").Body.String(), "chapter2/second.qmd") {
		t.Errorf("staged page not marked modified")
	}
	post(t, srv, "/git/commit", url.Values{"message": {"edit"}})
	if modifiedRow(t, get(t, srv, "/tree").Body.String(), "chapter2/second.qmd") {
		t.Errorf("committed page still marked modified")
	}
}

// UI.1-8-1: the watch refreshes the tree when the set of modified pages
// changes, even when the pages themselves have not: a commit changes no
// page, and a save of the app's own does not count as an outside change.
func TestWatchFollowsModified(t *testing.T) {
	srv, root := gitProject(t)
	get(t, srv, "/tree")
	if rec := get(t, srv, "/watch"); rec.Code != 204 {
		t.Fatalf("watch answered %d with nothing changed", rec.Code)
	}
	writeFile(t, root, "chapter2/second.qmd", "---\ntitle: Second\norder: 1\n---\n# Edited\n")
	if rec := get(t, srv, "/watch"); rec.Code != 200 || !modifiedRow(t, rec.Body.String(), "chapter2/second.qmd") {
		t.Fatalf("watch did not bring the mark: %d", rec.Code)
	}
	if rec := get(t, srv, "/watch"); rec.Code != 204 {
		t.Fatalf("watch answered %d twice for one change", rec.Code)
	}
	post(t, srv, "/git/stage", url.Values{"all": {"1"}})
	post(t, srv, "/git/commit", url.Values{"message": {"edit"}})
	if rec := get(t, srv, "/watch"); rec.Code != 200 || modifiedRow(t, rec.Body.String(), "chapter2/second.qmd") {
		t.Fatalf("watch did not take the mark away after the commit: %d", rec.Code)
	}
}

// Outside a Git working tree nothing is marked, and the tree still loads.
func TestTreeWithoutGit(t *testing.T) {
	srv, _ := testServer(t)
	if modifiedRow(t, get(t, srv, "/tree").Body.String(), "chapter2/second.qmd") {
		t.Errorf("page marked modified outside a repository")
	}
}
