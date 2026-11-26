package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
)

// draftProject has a page with a todo, a comment, a review, the new
// flag and a bookmark, and a page marked as a draft. It returns the
// selections of the format named.
func draftProject(t *testing.T, format string, draft bool) []config.Selection {
	t.Helper()
	root := t.TempDir()
	for name, body := range map[string]string{
		"folio.yml":     "formats:\n  handout: {type: docx}\n  site: {type: html}\n",
		"book/index.md": "# Book\n",
		"book/a.md":     "---\ntitle: Alpha\norder: 1\nreview: Check the figures\nnew: true\nbookmarked: true\n---\nDo ==TODO== and <!-- << Why? -->this<!-- >> -->.\n",
		"book/c.md":     "---\ntitle: Gamma\norder: 2\ndraft: true\n---\nGamma text\n",
	} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	sels, err := cfg.Expand(config.Request{Formats: []string{format}, Draft: draft})
	if err != nil {
		t.Fatal(err)
	}
	return sels
}

var draftMarks = []string{"Gamma text", `<mark class="folio-todo">TODO</mark>`, `<mark class="folio-comment">this</mark>`, `<span class="folio-note" role="note">`, `class="callout callout-warning"`, "<strong>Review:</strong> Check the figures", "🆕", `<div class="folio-bookmark">`}

func TestDraftDocument(t *testing.T) {
	for _, draft := range []bool{false, true} {
		doc, _, err := Document(draftProject(t, "handout", draft)[0], nil)
		if err != nil {
			t.Fatal(err)
		}
		html, err := markdown.HTML(doc)
		if err != nil {
			t.Fatal(err)
		}
		for _, mark := range draftMarks {
			if strings.Contains(html, mark) != draft {
				t.Errorf("draft %v: %q shown %v:\n%s", draft, mark, !draft, html)
			}
		}
	}
}

func TestDraftSite(t *testing.T) {
	for _, draft := range []bool{false, true} {
		dir := t.TempDir()
		if err := Site(draftProject(t, "site", draft)[0], dir, nil); err != nil {
			t.Fatal(err)
		}
		a, err := os.ReadFile(filepath.Join(dir, "book", "a.html"))
		if err != nil {
			t.Fatal(err)
		}
		c, _ := os.ReadFile(filepath.Join(dir, "book", "c.html"))
		html := string(a) + string(c)
		for _, mark := range draftMarks {
			if strings.Contains(html, mark) != draft {
				t.Errorf("draft %v: %q shown %v:\n%s", draft, mark, !draft, html)
			}
		}
		if strings.Contains(html, `<body class="draft">`) != draft {
			t.Errorf("draft %v: body class wrong", draft)
		}
	}
}
