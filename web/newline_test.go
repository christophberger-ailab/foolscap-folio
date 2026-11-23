package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// crlfProject rewrites the fixture's pages and folio.yml with CRLF line
// endings, as a checkout on Windows has them.
func crlfProject(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(p, []byte(strings.ReplaceAll(string(b), "\n", "\r\n")), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The editor hands back LF whatever the file had. Saving a CRLF page keeps
// it CRLF, so Git sees the one line that changed rather than every line
// (issue #129) -- also when the tree renumbered the page in between.
func TestSaveKeepsCRLF(t *testing.T) {
	srv, root := testServer(t)
	crlfProject(t, root)
	page := filepath.Join(root, "chapter2", "second.qmd")
	if rec := get(t, srv, "/content?path=chapter2/second.qmd"); rec.Code != http.StatusOK {
		t.Fatalf("content: status %d: %s", rec.Code, rec.Body)
	}
	edited := "---\ntitle: Second\norder: 1\n---\n# Second\n\nnew line\n"
	if rec := post(t, srv, "/save", url.Values{"path": {"chapter2/second.qmd"}, "body": {edited}}); rec.Code != http.StatusOK {
		t.Fatalf("save: status %d: %s", rec.Code, rec.Body)
	}
	if got, want := readFile(t, page), strings.ReplaceAll(edited, "\n", "\r\n"); got != want {
		t.Errorf("saved page = %q, want %q", got, want)
	}

	if rec := post(t, srv, "/move", url.Values{
		"src": {"chapter2/third.qmd"}, "parent": {"chapter2/index.qmd"}, "pos": {"0"},
	}); rec.Code != http.StatusOK {
		t.Fatalf("move: status %d: %s", rec.Code, rec.Body)
	}
	moved := readFile(t, page)
	if strings.Count(moved, "---") != 2 || strings.Contains(strings.ReplaceAll(moved, "\r\n", ""), "\n") {
		t.Fatalf("renumbered page not CRLF with one frontmatter block: %q", moved)
	}
	again := edited + "more\n"
	if rec := post(t, srv, "/save", url.Values{"path": {"chapter2/second.qmd"}, "body": {again}}); rec.Code != http.StatusOK {
		t.Fatalf("save after move: status %d: %s", rec.Code, rec.Body)
	}
	got := readFile(t, page)
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") || !strings.HasSuffix(got, "more\r\n") || strings.Contains(got, "order: 1\r\n") {
		t.Errorf("page saved after the move = %q, want CRLF with the tree's order and the edit", got)
	}
}

// The configuration files are edited in the browser too.
func TestConfigFilesKeepCRLF(t *testing.T) {
	srv, root := configTestServer(t)
	css := filepath.Join(cssDir(root), defaultCSSName)
	os.MkdirAll(filepath.Dir(css), 0o755)
	for _, tc := range []struct{ name, file, action, text string }{
		{"folio.yml", filepath.Join(root, "folio.yml"), "/config/files/project", "title: Renamed\nformats:\n  pdf: {type: pdf}\n"},
		{"_brand.yml", filepath.Join(root, "_brand.yml"), "/config/files/brand", "color:\n  primary: '#112233'\n"},
		{"css", css, "/config/files/css", "h1 {\n  color: red;\n}\n"},
	} {
		if err := os.WriteFile(tc.file, []byte("# old\r\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		form := url.Values{"text": {tc.text}, "file": {defaultCSSName}}
		if rec := post(t, srv, tc.action, form); !strings.Contains(rec.Body.String(), "Saved.") {
			t.Fatalf("%s: save not confirmed:\n%s", tc.name, rec.Body)
		}
		if got, want := readFile(t, tc.file), strings.ReplaceAll(tc.text, "\n", "\r\n"); got != want {
			t.Errorf("%s = %q, want %q", tc.name, got, want)
		}
	}
}
