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
	doc, _, _, err := Document(sels[0], nil)
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

// A render styles the website, the DOCX and the PDF by the brand file: the
// project's, or else the user's (Config.UserBrand).
func TestRenderAppliesTheBrand(t *testing.T) {
	user := brandProject(t, brandStyleYML)
	for name, own := range map[string]bool{"project": true, "user": false} {
		root := t.TempDir()
		if own {
			root = brandProject(t, brandStyleYML)
		}
		for file, body := range map[string]string{
			"folio.yml":      "formats:\n  web: {type: html}\n  handout: {type: [docx, pdf]}\n",
			"book/index.qmd": "---\ntitle: Book\n---\n\n# Book\n\nText.\n",
		} {
			os.MkdirAll(filepath.Join(root, filepath.Dir(file)), 0o755)
			os.WriteFile(filepath.Join(root, file), []byte(body), 0o644)
		}
		cfg, err := config.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		cfg.UserBrand = filepath.Join(user, BrandFile)
		sels, err := cfg.Expand(config.Request{})
		if err != nil {
			t.Fatal(err)
		}
		var log []string
		if err := Run(Options{Selections: sels}, func(f string, a ...any) { log = append(log, f) }); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, strings.Join(log, "\n"))
		}
		out := filepath.Join(root, "_output")
		css, err := os.ReadFile(filepath.Join(out, "web", "_site", "brand.css"))
		if err != nil || !strings.Contains(string(css), "--accent:#003366") {
			t.Errorf("%s: brand.css: %v\n%s", name, err, css)
		}
		if _, err := os.Stat(filepath.Join(out, "web", "_site", "fonts", "1-Sans-Regular.ttf")); err != nil {
			t.Errorf("%s: font not copied: %v", name, err)
		}
		page, _ := os.ReadFile(filepath.Join(out, "web", "index.html"))
		if !strings.Contains(string(page), "_site/brand.css") {
			t.Errorf("%s: page does not link the brand", name)
		}
		for _, f := range []string{"handout/book.docx", "handout/book.pdf"} {
			if _, err := os.Stat(filepath.Join(out, f)); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
}
