// Package build renders a folio project: every selected topic × format ×
// audience combination, without Quarto, Pandoc or LaTeX.
//
// A document (DOCX, PDF) is one topic folder flattened into one Markdown
// text by internal/bookmaker — pages in folder order, headings demoted by
// folder depth, in-book links turned into anchors — with its shortcodes
// resolved, parsed by internal/markdown, stripped of the conditional
// content the selection does not show, and written by internal/docx or
// laid out by internal/docrender.
//
// A website is rendered page by page by internal/site, from the same tree
// and with the same demotion.
package build

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yuin/goldmark/ast"

	"github.com/christophberger-ailab/foolscap-folio/internal/bookmaker"
	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/docrender"
	"github.com/christophberger-ailab/foolscap-folio/internal/docx"
	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
	"github.com/christophberger-ailab/foolscap-folio/internal/shortcode"
	"github.com/christophberger-ailab/foolscap-folio/internal/site"
	"github.com/christophberger-ailab/foolscap-folio/internal/svgimage"
)

// Logf receives the render's progress, one line per call, in the format of
// fmt.Printf. A nil Logf discards the output.
type Logf func(format string, args ...any)

func (l Logf) or() Logf {
	if l == nil {
		return func(string, ...any) {}
	}
	return l
}

// Options describe a render run.
type Options struct {
	Selections []config.Selection
	// Clean empties each selected format's output directory once, before
	// the first render.
	Clean bool
	// DryRun prints what would be written without writing anything.
	DryRun bool
	// UpdateTOC has LibreOffice lay out every DOCX with a table of
	// contents and fills the page numbers in (see docx.Paginate).
	UpdateTOC bool
}

// paginate fills the page numbers into a DOCX's contents.
var paginate = docx.Paginate

// pdfTimeout bounds one PDF layout, which runs in a library that cannot be
// interrupted.
const pdfTimeout = 5 * time.Minute

// Run renders every selection. A failing selection does not stop the
// others; the error reports how many failed.
func Run(o Options, log Logf) error {
	log = log.or()
	if o.Clean && !o.DryRun {
		if err := clean(o.Selections, log); err != nil {
			return err
		}
	}
	failed := 0
	for i, sel := range o.Selections {
		log("[%d/%d] %s", i+1, len(o.Selections), sel)
		if err := Render(sel, o, log); err != nil {
			log("error: %v", err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d renders failed", failed, len(o.Selections))
	}
	log("done: %d render(s)", len(o.Selections))
	return nil
}

// Render produces one selection's output, as o.DryRun and o.UpdateTOC say.
func Render(sel config.Selection, o Options, log Logf) error {
	log = log.or()
	dryRun := o.DryRun
	dir, err := sel.OutputDir()
	if err != nil {
		return err
	}
	if sel.Format.IsWeb() {
		return renderSite(sel, dir, dryRun, log)
	}
	stem, err := sel.FileStem()
	if err != nil {
		return err
	}
	cover, err := sel.Cover()
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(cover); cover != "" && errors.Is(statErr, fs.ErrNotExist) {
		log("warning: topic %s: cover %s not found; the placeholder stands in for it",
			sel.Topic.Name, rel(sel.Config.Root, cover))
		if !dryRun {
			if cover, err = placeholderCover(sel.Config.Root); err != nil {
				return fmt.Errorf("topic %s: cover: %w", sel.Topic.Name, err)
			}
			defer os.Remove(cover)
		}
	} else if cover != "" {
		if err := docrender.ValidateCover(cover); err != nil {
			return fmt.Errorf("topic %s: cover: %w", sel.Topic.Name, err)
		}
	}
	template, err := sel.Template()
	if err != nil {
		return err
	}
	if template != "" {
		if _, err := os.Stat(template); err != nil {
			return fmt.Errorf("format %s: template: %w", sel.Format.Name, err)
		}
	}
	if dryRun {
		for _, t := range sel.Format.Type {
			log("would write %s", rel(sel.Config.Root, filepath.Join(dir, stem+"."+t)))
		}
		return nil
	}

	doc, book, err := Document(sel, log)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lang := sel.Config.Lang
	brand, err := loadBrand(sel.Config.Root, sel.Config.UserBrand)
	if err != nil {
		return err
	}
	look := brand.style()
	page, done, err := coverPage(sel, brand, book, cover, log)
	if err != nil {
		return fmt.Errorf("topic %s: cover: %w", sel.Topic.Name, err)
	}
	defer done()
	headings, sizes := brand.headings()
	for _, t := range sel.Format.Type {
		out := filepath.Join(dir, stem+"."+t)
		switch t {
		case config.TypeDOCX:
			err = docx.Write(out, doc, docx.Options{
				Root: sel.Config.Root, Template: template, Title: book.Title, Lang: lang, TOC: sel.Format.TOC,
				Look: docx.Look(look), Headings: headings, Warn: warn(log),
			})
			if err == nil {
				err = docrender.CoverDOCX(out, page)
			}
			if err == nil && sel.Format.TOC && o.UpdateTOC {
				// The cover is a page too, so the pages are counted last.
				var filled bool
				if filled, err = paginate(out); err != nil {
					err = fmt.Errorf("written, but its contents have no page numbers: %w", err)
				} else if filled {
					log("%s: contents updated with LibreOffice's page numbers", rel(sel.Config.Root, out))
				}
			}
		case config.TypePDF:
			var fonts map[string]string
			var notes []string
			fonts, notes, err = brand.fonts(map[string]string{"title": page.Title.Font, "subtitle": page.Subtitle.Font})
			for _, n := range notes {
				log("note: %s", n)
			}
			if err == nil {
				err = docrender.RunPDF(context.Background(), pdfTimeout, out, nil, docrender.Options{
					Root: sel.Config.Root, Cover: page, Title: book.Title, TOC: sel.Format.TOC, Lang: lang, Fonts: fonts, Sizes: sizes,
					Colors:   docrender.Colors{Text: look.Color, Heading: look.HeadingColor, Link: look.LinkColor},
					Markdown: doc, Warn: warn(log),
				})
			}
		}
		if err != nil {
			return fmt.Errorf("%s: %w", rel(sel.Config.Root, out), err)
		}
		log("wrote %s", rel(sel.Config.Root, out))
	}
	return nil
}

// placeholderCover writes the project's placeholder as a PNG to a
// temporary file, for a cover that is not there. The caller removes it.
func placeholderCover(root string) (string, error) {
	b, err := svgimage.PNG(svgimage.Placeholder(root), 4)
	if err != nil {
		return "", err
	}
	return tempPNG(b)
}

// warn turns log into a writer's warning hook.
func warn(log Logf) func(string, ...any) {
	return func(format string, args ...any) { log("warning: "+format, args...) }
}

// Document builds the parsed, filtered document of a document selection,
// and what its cover shows. The book's title is the cover's, so the
// document leaves out its heading.
func Document(sel config.Selection, log Logf) (*markdown.Document, Book, error) {
	log = log.or()
	text, book, err := Flatten(sel, log)
	if err != nil {
		return nil, Book{}, err
	}
	doc := markdown.Parse([]byte(text))
	doc.Prepare(markdown.Options{Keep: sel.Keep, Formats: sel.Types(), Lang: sel.Config.Lang, CalloutTitles: sel.Config.CalloutTitles})
	dropTitle(doc, book.Anchor)
	vars := sel.Vars()
	for _, f := range []struct {
		name string
		text *string
	}{{"title", &book.Title}, {"subtitle", &book.Subtitle}} {
		if s, err := config.Interpolate(*f.text, vars); err != nil {
			log("warning: topic %s: %s: %v; it is shown as written", sel.Topic.Name, f.name, err)
		} else {
			*f.text = s
		}
	}
	return doc, book, nil
}

// dropTitle removes the document's first heading if it is the book's
// title, with the ID id.
func dropTitle(doc *markdown.Document, id string) {
	var title ast.Node
	_ = ast.Walk(doc.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering {
			if h.Level == 1 && markdown.HeadingID(h) == id {
				title = h
			}
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if title != nil {
		title.Parent().RemoveChild(title.Parent(), title)
	}
}

// Flatten returns a document selection's Markdown — the topic folder
// flattened, shortcodes resolved — and its book's title, subtitle and
// title ID, the variables in them not resolved.
func Flatten(sel config.Selection, log Logf) (string, Book, error) {
	log = log.or()
	root := sel.Config.Root
	tree, err := loadTopic(root, sel.Topic)
	if err != nil {
		return "", Book{}, err
	}
	res, err := bookmaker.Flatten(tree, bookmaker.Options{
		ProjectRoot:   root,
		Title:         sel.Topic.Title,
		RewriteLinks:  true,
		WrapAudience:  true,
		ExtractSlides: true,
	})
	if err != nil {
		return "", Book{}, err
	}
	for _, w := range res.Warnings {
		log("%s: warning: %s", sel.Topic.Folder, w)
	}
	log("%s: %d pages, %d links rewritten", sel.Topic.Folder, res.Pages, res.Links)
	text, err := Expander(sel, log).Expand(res.Content)
	if err != nil {
		return "", Book{}, err
	}
	book := Book{Title: tree.Title(), Anchor: tree.Anchor}
	if tree.Page != nil {
		book.Subtitle = tree.Page.Subtitle
	}
	return text, book, nil
}

// Expander returns the shortcode expander of a selection.
func Expander(sel config.Selection, log Logf) *shortcode.Expander {
	log = log.or()
	return shortcode.New(sel.Config.Root, shortcode.Variables(sel.Config.Root, sel.Vars()),
		warn(log))
}

func loadTopic(root string, t config.Topic) (*bookmaker.Node, error) {
	dir := filepath.Join(root, filepath.FromSlash(t.Folder))
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("topic %s: folder %s does not exist", t.Name, t.Folder)
	}
	tree, err := bookmaker.LoadTree(dir, root)
	if err != nil {
		return nil, err
	}
	if tree == nil {
		return nil, fmt.Errorf("topic %s: %s holds no pages", t.Name, t.Folder)
	}
	return tree, nil
}

// Site renders a website selection into dir instead of its output
// directory, as `folio preview` does.
func Site(sel config.Selection, dir string, log Logf) error {
	return renderSite(sel, dir, false, log.or())
}

func renderSite(sel config.Selection, dir string, dryRun bool, log Logf) error {
	css, err := sel.Stylesheets()
	if err != nil {
		return err
	}
	for _, p := range css {
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			return fmt.Errorf("format %s: css: %s is not a file", sel.Format.Name, rel(sel.Config.Root, p))
		}
	}
	if dryRun {
		log("would write the site to %s/", rel(sel.Config.Root, dir))
		return nil
	}
	topics := make([]site.Topic, len(sel.Topics))
	for i, t := range sel.Topics {
		topics[i] = site.Topic{Name: t.Name, Folder: t.Folder, Title: t.Title}
	}
	brand, err := loadBrand(sel.Config.Root, sel.Config.UserBrand)
	if err != nil {
		return err
	}
	brandCSS, brandFonts, notes := brand.css()
	logo, logoAlt, logoNotes := brand.logo()
	notes = append(notes, logoNotes...)
	for _, n := range notes {
		log("note: %s", n)
	}
	exp := Expander(sel, log)
	err = site.Build(site.Options{
		BrandCSS:      brandCSS,
		BrandFonts:    brandFonts,
		Logo:          logo,
		LogoAlt:       logoAlt,
		Root:          sel.Config.Root,
		OutDir:        dir,
		Title:         sel.Config.Title,
		Lang:          sel.Config.Lang,
		CalloutTitles: sel.Config.CalloutTitles,
		Topics:        topics,
		Keep:          sel.Keep,
		Formats:       sel.Types(),
		CSS:           css,
		Expand:        exp.Expand,
		Log:           log,
	})
	if err != nil {
		return err
	}
	log("wrote %s/index.html", rel(sel.Config.Root, dir))
	return nil
}

// clean empties the output directory of every selection once.
func clean(sels []config.Selection, log Logf) error {
	seen := map[string]bool{}
	for _, sel := range sels {
		dir, err := sel.OutputDir()
		if err != nil {
			return err
		}
		if seen[dir] {
			continue
		}
		seen[dir] = true
		out := filepath.Join(sel.Config.Root, filepath.FromSlash(sel.Config.Output))
		if r, err := filepath.Rel(out, dir); err != nil || r == "." || strings.HasPrefix(r, "..") {
			return fmt.Errorf("refusing to clean %s: it is not below %s", dir, out)
		}
		log("cleaning %s/", rel(sel.Config.Root, dir))
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(r)
	}
	return p
}
