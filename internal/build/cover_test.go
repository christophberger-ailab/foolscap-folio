package build

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
)

// The brand's meta field gives the cover's title and subtitle and each
// heading level a font and a size; a size that is no length in points is
// reported and left out, as is a font the PDF has no TrueType file of.
func TestBrandMetaFonts(t *testing.T) {
	root := brandProject(t, brandYML+`meta:
  fonts:
    title: {font: Corporate Mono, size: 40pt}
    subtitle: {font: Aptos Display, size: "18"}
    h1: {size: 24px}
    h2: {font: Corporate Sans, size: huge}
    h7: {size: 9pt}
`)
	b, err := loadBrand(root, "")
	if err != nil {
		t.Fatal(err)
	}
	look, notes := b.style()
	if look.Sizes["title"] != 40 || look.Sizes["subtitle"] != 18 || look.Sizes["h1"] != 18 || look.Sizes["h2"] != 0 {
		t.Errorf("sizes = %v", look.Sizes)
	}
	if look.Fonts["title"] != "Corporate Mono" || look.Fonts["subtitle"] != "Aptos Display" || look.Fonts["h2"] != "Corporate Sans" {
		t.Errorf("fonts = %v", look.Fonts)
	}
	if all := strings.Join(notes, "\n"); !strings.Contains(all, `"huge"`) || !strings.Contains(all, "h7") {
		t.Errorf("notes = %q", notes)
	}
	paths, notes, err := b.fonts()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(paths["title"]) != "Mono.ttf" || filepath.Base(paths["h2"]) != "Sans-Bold.ttf" || paths["subtitle"] != "" || paths["h1"] != "" {
		t.Errorf("paths = %v", paths)
	}
	if all := strings.Join(notes, "\n"); !strings.Contains(all, "Aptos Display") {
		t.Errorf("no note on the subtitle's font: %q", notes)
	}

	// No brand, no meta: nothing, and no failure.
	var none *brand
	if look, notes := none.style(); len(look.Sizes)+len(look.Fonts)+len(notes) != 0 {
		t.Errorf("look of no brand = %+v, %v", look, notes)
	}
}

// The cover of a topic and audience is the most specific file beside the
// one the brand names.
func TestCoverFile(t *testing.T) {
	root := t.TempDir()
	touch := func(names ...string) {
		for _, n := range names {
			p := filepath.Join(root, "assets", n)
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.WriteFile(p, []byte("<svg/>"), 0o644)
		}
	}
	vars := map[string]string{"topic": "databases", "audience-template": "_DEV"}
	find := func(vars map[string]string) string {
		t.Helper()
		p, err := coverFile(root, "/assets/cover.svg", vars)
		if err != nil {
			t.Fatal(err)
		}
		return filepath.Base(p)
	}
	if got := find(vars); got != "." && got != "" {
		t.Errorf("found %q in an empty folder", got)
	}
	touch("cover.svg")
	if got := find(vars); got != "cover.svg" {
		t.Errorf("got %q", got)
	}
	touch("cover_DEV.svg")
	if got := find(vars); got != "cover_DEV.svg" {
		t.Errorf("got %q", got)
	}
	if got := find(map[string]string{"topic": "databases"}); got != "cover.svg" {
		t.Errorf("without an audience-template: %q", got)
	}
	touch("cover_databases.svg")
	if got := find(vars); got != "cover_databases.svg" {
		t.Errorf("got %q", got)
	}
	touch("cover_databases_DEV.svg")
	if got := find(vars); got != "cover_databases_DEV.svg" {
		t.Errorf("got %q", got)
	}
	for _, bad := range []string{"../cover.svg", "/assets/cover.png"} {
		if _, err := coverFile(root, bad, vars); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

const coverSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 210 297">` +
	`<rect width="210" height="297" fill="#003366"/>` +
	`<path id="title" d="M21 100h168v30h-168z" fill="#ffffff"/>%s</svg>`

// coverProject is a project with a book for two audiences and a brand
// that names a cover; files are added to it.
func coverProject(t *testing.T, files map[string]string) (string, []config.Selection) {
	t.Helper()
	root := t.TempDir()
	all := map[string]string{
		"folio.yml": "vars: {product: Folio}\nformats:\n  handout: {type: [docx, pdf]}\n" +
			"audiences:\n  std:\n  dev: {vars: {audience-label: developers, audience-template: _DEV}}\n",
		"book/index.md":   "---\ntitle: \"{product} Guide\"\nsubtitle: For {audience-label}\n---\n\nPreface.\n",
		"book/chapter.md": "# Chapter\n\nText.\n",
	}
	for name, body := range files {
		all[name] = body
	}
	for name, body := range all {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
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
	return root, sels
}

func docxPart(t *testing.T, path, name string) string {
	t.Helper()
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	r, err := z.Open(name)
	if err != nil {
		return ""
	}
	b, _ := io.ReadAll(r)
	return string(b)
}

func renderAll(t *testing.T, sels []config.Selection) string {
	t.Helper()
	var log strings.Builder
	err := Run(Options{Selections: sels}, func(f string, a ...any) { log.WriteString(strings.TrimSpace(fmt.Sprintf(f, a...)) + "\n") })
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	return log.String()
}

// Without a brand a document opens with a title page: the book's title
// and subtitle, their variables resolved, and the title is no heading of
// the body any more.
func TestRenderTitlePage(t *testing.T) {
	root, sels := coverProject(t, nil)
	log := renderAll(t, sels)
	doc := docxPart(t, filepath.Join(root, "_output/handout/book-dev.docx"), "word/document.xml")
	for _, want := range []string{">Folio Guide</w:t>", ">For developers</w:t>", `<w:sz w:val="72"/>`, `<w:sz w:val="40"/>`, "Preface.", "Chapter"} {
		if !strings.Contains(doc, want) {
			t.Errorf("dev: missing %s", want)
		}
	}
	if n := strings.Count(doc, "Folio Guide"); n != 1 {
		t.Errorf("the title is in the document %d times, want once, on the cover", n)
	}
	// The audience std has no audience-label: the subtitle stays as written.
	doc = docxPart(t, filepath.Join(root, "_output/handout/book-std.docx"), "word/document.xml")
	if !strings.Contains(doc, "For {audience-label}") || !strings.Contains(log, "audience-label") {
		t.Errorf("std: subtitle or warning missing\n%s", log)
	}
	if _, err := os.Stat(filepath.Join(root, "_output/handout/book-dev.pdf")); err != nil {
		t.Error(err)
	}
}

// The brand's cover image is the background of the cover page, in the
// variant of the audience; the title goes where the image's title element
// is, in its color, and a subtitle without an element is left out with a
// warning.
func TestRenderCoverImage(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	root, sels := coverProject(t, map[string]string{
		"_brand.yml":           "meta:\n  coverpage: /assets/cover.svg\n  fonts:\n    title: {font: Aptos Display, size: 30pt}\n",
		"assets/cover.svg":     strings.Replace(coverSVG, "%s", `<rect id="Subtitle" x="21" y="140" width="100" height="10"/>`, 1),
		"assets/cover_DEV.svg": strings.Replace(coverSVG, "%s", "", 1),
	})
	log := renderAll(t, sels)
	dev := filepath.Join(root, "_output/handout/book-dev.docx")
	doc := docxPart(t, dev, "word/document.xml")
	for _, want := range []string{"<w:framePr ", ">Folio Guide</w:t>", `w:ascii="Aptos Display"`, `<w:sz w:val="60"/>`, `<w:color w:val="FFFFFF"/>`, "w:drawing"} {
		if !strings.Contains(doc, want) {
			t.Errorf("dev: missing %s", want)
		}
	}
	if strings.Contains(doc, "For developers") {
		t.Error("dev: the subtitle is on a cover that has no place for it")
	}
	if !strings.Contains(log, `no element with the id "subtitle"`) {
		t.Errorf("no warning about the subtitle:\n%s", log)
	}
	if docxPart(t, dev, "word/media/folio-cover.png") == "" {
		t.Error("dev: cover image not embedded")
	}
	doc = docxPart(t, filepath.Join(root, "_output/handout/book-std.docx"), "word/document.xml")
	if strings.Count(doc, "<w:framePr ") != 2 || !strings.Contains(doc, "For {audience-label}") {
		t.Errorf("std: want title and subtitle in frames")
	}
	for _, f := range []string{"book-dev.pdf", "book-std.pdf"} {
		b, err := os.ReadFile(filepath.Join(root, "_output/handout", f))
		if err != nil || !strings.Contains(string(b), "/Subtype /Image") {
			t.Errorf("%s: no cover image (%v)", f, err)
		}
	}
	if m, _ := filepath.Glob(filepath.Join(tmp, "*")); len(m) != 0 {
		t.Errorf("temporary covers left behind: %v", m)
	}
}

// A cover the brand names but the project lacks does not stop the render:
// the document gets the plain title page, with a warning.
func TestRenderMissingCoverImage(t *testing.T) {
	root, sels := coverProject(t, map[string]string{"_brand.yml": "meta:\n  coverpage: assets/none.svg\n"})
	log := renderAll(t, sels[:1])
	doc := docxPart(t, filepath.Join(root, "_output/handout/book-std.docx"), "word/document.xml")
	if !strings.Contains(doc, ">Folio Guide</w:t>") || strings.Contains(doc, "w:drawing") || !strings.Contains(log, "assets/none.svg") {
		t.Errorf("document or warning wrong:\n%s", log)
	}
}

// A topic's own PNG cover is a finished page: nothing is written on it,
// and the title stays the document's first heading.
func TestRenderTopicCoverKeepsTitleHeading(t *testing.T) {
	root, sels := coverProject(t, map[string]string{
		"folio.yml": "vars: {product: Folio}\nformats:\n  handout: {type: docx}\ntopics:\n  book: {cover: assets/none.png}\n",
	})
	renderAll(t, sels)
	doc := docxPart(t, filepath.Join(root, "_output/handout/book.docx"), "word/document.xml")
	if !strings.Contains(doc, "Folio Guide") || strings.Contains(doc, "<w:framePr ") || !strings.Contains(doc, "w:drawing") {
		t.Error("the topic's cover is not the cover, or the title heading is gone")
	}
}
