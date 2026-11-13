package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
)

// A document titles a callout without a title of its own from the user's
// translations (Config.CalloutTitles) for the project's language.
func TestDocumentTranslatesCalloutTitles(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("folio.yml", "lang: fr\nformats:\n  handout: {type: docx}\n")
	write("book/index.qmd", "---\ntitle: Book\n---\n\n::: {.callout-warning}\nBody\n:::\n")
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg.CalloutTitles = map[string]map[string]string{"fr": {"warning": "Attention"}}
	sels, err := cfg.Expand(config.Request{Formats: []string{"handout"}})
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err := Document(sels[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	html, err := markdown.HTML(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<div class="callout-title">Attention</div>`) {
		t.Errorf("callout not titled in French:\n%s", html)
	}
}
