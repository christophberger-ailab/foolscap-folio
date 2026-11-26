package web

import (
	"strings"
	"testing"
)

// UI.3-10: the bookmark button reads the page's `bookmarked:` from the
// editor's text, and toggling writes it back there: the line is set to
// true or false where it stands, added at the end of the frontmatter, or
// brought with a frontmatter of its own for a page without one.
func TestBookmarkInText(t *testing.T) {
	script := jsFunc(t, "assets/static/editor.js", "frontmatterOf") +
		jsFunc(t, "assets/static/editor.js", "bookmarkedIn") +
		jsFunc(t, "assets/static/editor.js", "withBookmark") + `
const cases = [
  ["---\ntitle: A\n---\n# A\n", true],
  ["---\ntitle: A\nbookmarked: true\n---\n# A\n", false],
  ["---\ntitle: A\nbookmarked: false # later\n---\n# A\n", true],
  ["# A\n", true],
  ["# A\n", false],
  ["---\n---\nbookmarked: true\n", true],
  ["---\ntitle: A\n---\n# A\n", false],
];
for (const [text, on] of cases) {
  const out = withBookmark(text, on);
  console.log(JSON.stringify([bookmarkedIn(text), out, bookmarkedIn(out)]));
}`
	want := strings.Join([]string{
		`[false,"---\ntitle: A\nbookmarked: true\n---\n# A\n",true]`,
		`[true,"---\ntitle: A\nbookmarked: false\n---\n# A\n",false]`,
		`[false,"---\ntitle: A\nbookmarked: true\n---\n# A\n",true]`,
		`[false,"---\nbookmarked: true\n---\n# A\n",true]`,
		`[false,"# A\n",false]`,
		`[false,"---\nbookmarked: true\n---\nbookmarked: true\n",true]`,
		`[false,"---\ntitle: A\n---\n# A\n",false]`,
	}, "\n") + "\n"
	if out := runNode(t, script); out != want {
		t.Errorf("bookmark text:\n%s\nwant:\n%s", out, want)
	}
}

// UI.3-10: the bookmark button stands above the editor, between the path
// and the save status.
func TestBookmarkButtonAboveTheEditor(t *testing.T) {
	srv, _ := testServer(t)
	body := get(t, srv, "/content?path=chapter2/second.qmd").Body.String()
	title := strings.Index(body, `id="content-title"`)
	button := strings.Index(body, `id="bookmark-toggle"`)
	status := strings.Index(body, `id="save-status"`)
	if title < 0 || button < title || status < button {
		t.Errorf("title at %d, bookmark button at %d, save status at %d", title, button, status)
	}
}

// UI.14-11: the Bookmarks pane lists the bookmarked pages, in the order of
// the tree, each by its title and path; a project without one says so.
func TestBookmarksPane(t *testing.T) {
	srv, root := testServer(t)
	if body := get(t, srv, "/").Body.String(); !strings.Contains(body, `id="bookmarks-pane" data-pane="bookmarks"`) {
		t.Fatalf("page carries no Bookmarks pane")
	}
	if body := get(t, srv, "/bookmarks").Body.String(); !strings.Contains(body, "No page is bookmarked") {
		t.Errorf("empty list not said:\n%s", body)
	}
	writeFile(t, root, "chapter2/third.qmd", "---\ntitle: Third\norder: 2\nbookmarked: true\n---\n# Third\n")
	writeFile(t, root, "chapter2/second.qmd", "---\ntitle: Second\norder: 1\nbookmarked: true\n---\n# Second\n")
	body := get(t, srv, "/bookmarks").Body.String()
	second := strings.Index(body, `data-path="chapter2/second.qmd"`)
	third := strings.Index(body, `data-path="chapter2/third.qmd"`)
	if second < 0 || third < second || !strings.Contains(body, ">Second<") {
		t.Errorf("bookmarks not listed in tree order:\n%s", body)
	}
	if strings.Contains(body, `data-path="chapter2/loose.qmd"`) {
		t.Errorf("page without a bookmark listed:\n%s", body)
	}
}
