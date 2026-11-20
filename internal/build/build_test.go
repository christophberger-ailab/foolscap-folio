package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/docx"
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

// A format's section_prefix and chapter_prefix number the document's
// sections and chapters in the project's language (issue #96).
func TestFlattenPrefixesSectionsAndChapters(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"folio.yml":                "lang: de\nformats:\n  handout: {type: docx, section_prefix: true, chapter_prefix: true}\n",
		"book/index.md":            "# Book\n",
		"book/part/index.md":       "---\norder: 1\n---\n# Grundlagen\n",
		"book/part/chapter.md":     "---\norder: 1\n---\n# Einstieg\n",
		"book/part/chapter/sub.md": "# Detail\n",
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
	sels, err := cfg.Expand(config.Request{})
	if err != nil {
		t.Fatal(err)
	}
	text, _, err := Flatten(sels[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Teil 1 Grundlagen", "# Kapitel 1 Einstieg", "\n## Detail"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q:\n%s", want, text)
		}
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

// The contents of a DOCX get their page numbers from LibreOffice only when
// the render asks for it.
func TestUpdateTOCIsOptIn(t *testing.T) {
	root := t.TempDir()
	for file, body := range map[string]string{
		"folio.yml":      "formats:\n  handout: {type: docx, toc: true}\n",
		"book/index.qmd": "---\ntitle: Book\n---\n\n# Book\n\nText.\n",
	} {
		os.MkdirAll(filepath.Join(root, filepath.Dir(file)), 0o755)
		os.WriteFile(filepath.Join(root, file), []byte(body), 0o644)
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	sels, err := cfg.Expand(config.Request{})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	defer func(f func(string) (bool, error)) { paginate = f }(paginate)
	paginate = func(string) (bool, error) { calls++; return true, nil }

	if err := Run(Options{Selections: sels}, nil); err != nil || calls != 0 {
		t.Errorf("without the option: %v, %d calls", err, calls)
	}
	if err := Run(Options{Selections: sels, UpdateTOC: true}, nil); err != nil || calls != 1 {
		t.Errorf("with the option: %v, %d calls", err, calls)
	}
	// A dry run touches nothing.
	if err := Run(Options{Selections: sels, UpdateTOC: true, DryRun: true}, nil); err != nil || calls != 1 {
		t.Errorf("dry run: %v, %d calls", err, calls)
	}
	// A contents list that cannot be updated fails the render, with the file written.
	paginate = func(string) (bool, error) { return false, docx.ErrNoOffice }
	var log []string
	err = Run(Options{Selections: sels, UpdateTOC: true}, func(f string, a ...any) { log = append(log, fmt.Sprintf(f, a...)) })
	if err == nil || !strings.Contains(strings.Join(log, "\n"), "LibreOffice") {
		t.Errorf("no LibreOffice: %v\n%s", err, strings.Join(log, "\n"))
	}
	if _, err := os.Stat(filepath.Join(root, "_output", "handout", "book.docx")); err != nil {
		t.Error(err)
	}
}

// A render leaves out the folders and files that carry another audience's
// audience-template suffix, and everything below them (#92).
func TestRenderSkipsOtherAudiencesPaths(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"folio.yml":                  "formats:\n  web: {type: html}\n  handout: {type: docx}\naudiences:\n  std:\n  abc: {vars: {audience-template: _ABC}}\n",
		"book/index.md":              "# Book\n",
		"book/plain.md":              "# Plain page\n",
		"book/chapter1_ABC/index.md": "# ABC chapter\n",
		"book/chapter1_ABC/sub.md":   "# ABC sub page\n",
		"book/extra_ABC.md":          "# ABC page\n",
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
	for aud, want := range map[string]bool{"std": false, "abc": true} {
		sels, err := cfg.Expand(config.Request{Formats: []string{"handout"}, Audiences: []string{aud}})
		if err != nil {
			t.Fatal(err)
		}
		text, _, err := Flatten(sels[0], nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{"ABC chapter", "ABC sub page", "ABC page"} {
			if got := strings.Contains(text, s); got != want {
				t.Errorf("%s document: has %q = %v, want %v", aud, s, got, want)
			}
		}
		if !strings.Contains(text, "Plain page") {
			t.Errorf("%s document lost the plain page", aud)
		}

		sels, err = cfg.Expand(config.Request{Formats: []string{"web"}, Audiences: []string{aud}})
		if err != nil {
			t.Fatal(err)
		}
		out := t.TempDir()
		if err := Site(sels[0], out, nil); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{"book/chapter1_ABC/index.html", "book/chapter1_ABC/sub.html", "book/extra_ABC.html"} {
			_, err := os.Stat(filepath.Join(out, p))
			if got := err == nil; got != want {
				t.Errorf("%s site: has %s = %v, want %v", aud, p, got, want)
			}
		}
	}
}

// A website's pages end in the project's footer, its placeholders
// resolved from the page's front matter, else _variables.yml, else
// folio.yml (issue #115), and the date, with the page's topic; a
// placeholder without a variable is shown as written, with a
// warning.
func TestSiteFooter(t *testing.T) {
	root := t.TempDir()
	for file, body := range map[string]string{
		"folio.yml":       "lang: de\nvars: {owner: Acme, version: 0.9}\nfooter: \"{owner} · {topic} · {{version}} · {{ date }} · {nope}\"\nformats:\n  web: {type: html}\ntopics:\n  book: {vars: {owner: Book Co}}\n",
		"book/third.qmd":  "---\ntitle: Third\norder: 2\nversion: 2.0-beta\nowner: Page Co\n---\n\n# Third\n",
		"_variables.yml":  "version: 1.2\ndate: 2026-03-05\ndate_format: D. MON YYYY\n",
		"book/index.qmd":  "---\ntitle: Book\n---\n\n# Book\n\nText.\n",
		"book/second.qmd": "---\ntitle: Second\norder: 1\n---\n\n# Second\n\nIt is {{< var date >}}.\n",
	} {
		os.MkdirAll(filepath.Join(root, filepath.Dir(file)), 0o755)
		os.WriteFile(filepath.Join(root, file), []byte(body), 0o644)
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	sels, err := cfg.Expand(config.Request{})
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	if err := Run(Options{Selections: sels}, func(f string, a ...any) { log = append(log, fmt.Sprintf(f, a...)) }); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, _ := os.ReadFile(filepath.Join(root, "_output", "web", name))
		return string(b)
	}
	if want := `<footer class="page-footer">Book Co · book · 1.2 · 5. März 2026 · {nope}</footer>`; !strings.Contains(read("book/second.html"), want) {
		t.Errorf("topic page lacks %s", want)
	}
	if want := `<footer class="page-footer">Page Co · book · 2.0-beta · 5. März 2026 · {nope}</footer>`; !strings.Contains(read("book/third.html"), want) {
		t.Errorf("page with front matter lacks %s", want)
	}
	if want := `<footer class="page-footer">Acme ·  · 1.2 · 5. März 2026 · {nope}</footer>`; !strings.Contains(read("index.html"), want) {
		t.Errorf("home page lacks %s", want)
	}
	if !strings.Contains(read("book/second.html"), "It is 5. März 2026.") {
		t.Error("page: the var shortcode does not give the date")
	}
	warned := 0
	for _, l := range log {
		if strings.Contains(l, "footer") && strings.Contains(l, "nope") {
			warned++
		}
	}
	if warned != 2 {
		t.Errorf("want a warning per topic and the home page, got %d:\n%s", warned, strings.Join(log, "\n"))
	}
}
