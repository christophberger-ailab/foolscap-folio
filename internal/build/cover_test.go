package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
)

const templateSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="210" height="297">` +
	`<rect width="210" height="297" fill="#003366"/>` +
	`<path id="TITLE" fill="#ffffff" d="M21 50 h60 v15 h-60 z"/>%s</svg>`

// project writes the files to a new project and returns the selection of
// its one document.
func project(t *testing.T, files map[string]string) config.Selection {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
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
	if len(sels) != 1 {
		t.Fatalf("%d selections", len(sels))
	}
	return sels[0]
}

const coverConfig = "formats:\n  handout: {type: [docx, pdf]}\n" +
	"audiences:\n  dev: {vars: {audience-template: _DEV, audience-label: developers}}\n"

func TestBookTitleAndSubtitle(t *testing.T) {
	sel := project(t, map[string]string{
		"folio.yml":      coverConfig,
		"book/index.qmd": "---\ntitle: Book\nsubtitle: For {audience-label}, {nobody}\n---\n\nIntro\n\n# Chapter\n",
	})
	var log []string
	doc, book, err := Document(sel, func(f string, a ...any) { log = append(log, fmt.Sprintf(f, a...)) })
	if err != nil {
		t.Fatal(err)
	}
	// The undefined variable is warned about, and the text kept as written.
	if book.Title != "Book" || book.Subtitle != "For {audience-label}, {nobody}" || book.Anchor == "" {
		t.Errorf("book %+v", book)
	}
	if !strings.Contains(strings.Join(log, "\n"), "subtitle") {
		t.Errorf("no warning: %q", log)
	}
	html, _ := markdown.HTML(doc)
	if strings.Contains(html, ">Book<") || !strings.Contains(html, ">Chapter<") || !strings.Contains(html, "Intro") {
		t.Errorf("book title not taken out of the body:\n%s", html)
	}

	sel.Audience.Vars["nobody"] = "all"
	if _, book, _ = Document(sel, nil); book.Subtitle != "For developers, all" {
		t.Errorf("subtitle %q", book.Subtitle)
	}
}

func TestDefaultCover(t *testing.T) {
	sel := project(t, map[string]string{
		"folio.yml":      coverConfig,
		"book/index.qmd": "---\ntitle: Book\n---\n",
		BrandFile:        "meta:\n  fonts:\n    title: {font: Aptos Display, size: 40pt}\n    subtitle: {size: 24px}\n    h2: {font: Aptos, size: 16}\n",
	})
	b, err := loadBrand(sel.Config.Root)
	if err != nil {
		t.Fatal(err)
	}
	c, done, err := coverPage(sel, b, Book{Title: "Book", Subtitle: "Sub", Anchor: "book"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if c.Image != "" || c.Anchor != "book" {
		t.Errorf("cover %+v", c)
	}
	ti, sub := c.Title, c.Subtitle
	if ti.Text != "Book" || ti.Font != "Aptos Display" || ti.Size != 40 || !ti.Center || ti.Y <= 0 {
		t.Errorf("title %+v", ti)
	}
	if sub.Text != "Sub" || sub.Size != 18 || !sub.Below {
		t.Errorf("subtitle %+v", sub)
	}
	headings, sizes := b.headings()
	if headings[1].Name != "Aptos" || headings[1].Size != 16 || sizes["h2"] != 16 || sizes["title"] != 40 {
		t.Errorf("headings %v, sizes %v", headings, sizes)
	}
}

// The most specific template the topic and audience have is filled in: its
// title placeholder takes the title, in its color; one without a subtitle
// placeholder shows no subtitle, with a warning.
func TestTemplateCover(t *testing.T) {
	both := strings.Replace(templateSVG, "%s", `<path id="subtitle" d="M21 80 h40 v8 h-40 z"/>`, 1)
	sel := project(t, map[string]string{
		"folio.yml":                           coverConfig,
		"book/index.qmd":                      "---\ntitle: Book\n---\n",
		BrandFile:                             "meta:\n  coverpage: /assets/templates/cover.svg\n",
		"assets/templates/cover.svg":          both,
		"assets/templates/cover_DEV.svg":      both,
		"assets/templates/cover_book_DEV.svg": strings.Replace(templateSVG, "%s", "", 1),
	})
	b, err := loadBrand(sel.Config.Root)
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	c, done, err := coverPage(sel, b, Book{Title: "Book", Subtitle: "Sub"}, "", func(f string, a ...any) { log = append(log, fmt.Sprintf(f, a...)) })
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if _, err := os.Stat(c.Image); err != nil {
		t.Fatalf("no cover image: %v", err)
	}
	ti := c.Title
	if ti.Text != "Book" || ti.Center || ti.Color != [3]uint8{255, 255, 255} || ti.X < 0.09 || ti.X > 0.11 || ti.Y < 0.16 || ti.Y > 0.18 {
		t.Errorf("title %+v", ti)
	}
	if c.Subtitle.Text != "" || len(log) != 1 || !strings.Contains(log[0], "subtitle") {
		t.Errorf("subtitle %+v, log %q", c.Subtitle, log)
	}
	done()
	if _, err := os.Stat(c.Image); err == nil {
		t.Error("cover image left behind")
	}
}
