// Package build renders a folio project: every selected topic × format ×
// audience combination, without Quarto, Pandoc or LaTeX.
//
// A document (DOCX, PDF) is one topic folder flattened into one Markdown
// text by internal/bookmaker — pages in folder order, the topic's index as
// preliminaries, sections and chapters below it, the headings of deeper
// pages demoted, in-book links turned into anchors — with its shortcodes
// resolved, parsed by internal/markdown, stripped of the conditional
// content the selection does not show, and written by internal/docx or
// laid out by internal/docrender.
//
// A website is rendered page by page by internal/site, from the same tree,
// with every page's headings as written.
package build

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	brand, err := loadBrand(sel.Config.Root, sel.Config.UserBrand)
	if err == nil {
		err = brand.interpolate(sel.Vars())
	}
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(cover); cover != "" && errors.Is(statErr, fs.ErrNotExist) {
		if brand != nil && brand.Meta.Coverpage != "" {
			log("warning: topic %s: cover %s not found; %s's cover page %s stands in for it",
				sel.Topic.Name, rel(sel.Config.Root, cover), BrandFile, brand.Meta.Coverpage)
			cover = ""
		} else {
			log("warning: topic %s: cover %s not found; the placeholder stands in for it",
				sel.Topic.Name, rel(sel.Config.Root, cover))
		}
		if cover != "" && !dryRun {
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
	look := brand.style()
	page, done, err := coverPage(sel, brand, book, cover, log)
	if err != nil {
		return fmt.Errorf("topic %s: cover: %w", sel.Topic.Name, err)
	}
	defer done()
	headings, section, sizes := brand.headings()
	foot := footerDoc(sel, log)
	for _, t := range sel.Format.Type {
		out := filepath.Join(dir, stem+"."+t)
		switch t {
		case config.TypeDOCX:
			err = docx.Write(out, doc, docx.Options{
				Root: sel.Config.Root, Template: template, Title: book.Title, Lang: lang, TOC: sel.Format.TOC,
				Look: docx.Look(look), Headings: headings, SectionHeadings: section, Footer: foot, Warn: warn(log),
				Margins: resolveXML(variables(sel, log, nil), "template "+rel(sel.Config.Root, template), log),
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
					Markdown: doc, Footer: foot, Warn: warn(log),
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

// footerDoc returns the parsed page footer of a document selection, nil
// if it has none.
func footerDoc(sel config.Selection, log Logf) *markdown.Document {
	text := footer(sel, log)(sel.Topic.Name, nil)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	doc := markdown.Parse([]byte(text))
	doc.Prepare(markdown.Options{Keep: sel.Keep, Formats: sel.Types(), Lang: sel.Config.Lang, Warn: warn(log)})
	return doc
}

// ProjectTargets lists the headings and figures of every topic of the
// project, all their content kept, for the references a render leaves
// unlinked. A topic that cannot be read is left out.
func ProjectTargets(cfg *config.Config) []markdown.Target {
	var out []markdown.Target
	for _, e := range cfg.Topics {
		tree, err := loadTopic(cfg.Root, e.Value)
		if err != nil {
			continue
		}
		res, err := bookmaker.Flatten(tree, bookmaker.Options{ProjectRoot: cfg.Root, Title: e.Value.Title, PageMarkers: true})
		if err != nil {
			continue
		}
		text, err := Expander(config.Selection{Config: cfg, Topic: e.Value}, nil).Expand(res.Content)
		if err != nil {
			continue
		}
		doc := markdown.Parse([]byte(text))
		markdown.AssignIDs(doc)
		out = append(out, markdown.Targets(doc, "", "", nil)...)
	}
	return out
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
	doc.Prepare(markdown.Options{Keep: sel.Keep, Formats: sel.Types(), Lang: sel.Config.Lang, CalloutTitles: sel.Config.CalloutTitles, Draft: sel.Draft, Warn: warn(log)})
	dropTitle(doc, book.Anchor)
	if f := sel.Format; f.NumberedHeadings {
		markdown.NumberHeadings(doc, f.SectionPrefix, f.ChapterPrefix)
	}
	if l := sel.FigureLabel(); l != "" {
		markdown.NumberFigures(doc, l)
	}
	markdown.Resolver{
		Local: markdown.NewIndex(markdown.Targets(doc, "", "", warn(log))),
		All:   func() markdown.Index { return markdown.NewIndex(ProjectTargets(sel.Config)) },
		Href:  func(t markdown.Target) string { return "#" + t.ID },
		Warn:  warn(log),
	}.Resolve(doc)
	vars := variables(sel, log, nil)
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
	if err == nil {
		tree.Drop(sel.Excludes)
		if !sel.Draft {
			tree.DropDrafts()
		}
	}
	if err != nil {
		return "", Book{}, err
	}
	section, chapter := sel.Prefixes()
	res, err := bookmaker.Flatten(tree, bookmaker.Options{
		ProjectRoot:   root,
		Title:         sel.Topic.Title,
		RewriteLinks:  true,
		WrapAudience:  true,
		ExtractSlides: true,
		SectionPrefix: section,
		ChapterPrefix: chapter,
		Draft:         sel.Draft,
		PageMarkers:   true,
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

// footer returns the page footer of a selection by topic name,
// "" for the home page, and the page's front matter, its placeholders
// resolved against the page's variables. A placeholder without a variable
// is shown as written, with a warning per topic.
func footer(sel config.Selection, log Logf) func(string, map[string]any) string {
	text := sel.Footer()
	warned := map[string]bool{}
	once := func(f string, a ...any) {
		if m := fmt.Sprintf(f, a...); !warned[m] {
			warned[m] = true
			log("%s", m)
		}
	}
	return func(topic string, meta map[string]any) string {
		if text == "" {
			return ""
		}
		ts := sel
		if topic != sel.Topic.Name {
			ts.Topic = config.Topic{}
			for _, t := range sel.Topics {
				if t.Name == topic {
					ts.Topic = t
				}
			}
		}
		page := map[string]string{}
		shortcode.Flatten(meta, page)
		s, err := config.Interpolate(text, variables(ts, once, page))
		if err != nil {
			where := "the home page"
			if topic != "" {
				where = "topic " + topic
			}
			once("warning: footer of %s: %v; it is shown as written", where, err)
		}
		return s
	}
}

// variables are the selection's variables from folio.yml, overlaid with
// those of _variables.yml and then with page's, together with the date
// and the built-ins topic, format and audience.
func variables(sel config.Selection, log Logf, page map[string]string) map[string]string {
	vars, err := shortcode.Variables(sel.Config.Root, sel.Config.Lang, sel.Vars(), page,
		map[string]string{"topic": sel.Topic.Name, "format": sel.Format.Name, "audience": sel.Audience.Name})
	if err != nil {
		log.or()("warning: %v", err)
	}
	return vars
}

// Expander returns the shortcode expander of a selection.
func Expander(sel config.Selection, log Logf) *shortcode.Expander {
	log = log.or()
	return shortcode.New(sel.Config.Root, variables(sel, log, nil),
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
	logo, sideLogo, logoNotes := brand.logos()
	notes = append(notes, logoNotes...)
	for _, n := range notes {
		log("note: %s", n)
	}
	exp := Expander(sel, log)
	err = site.Build(site.Options{
		Footer:        footer(sel, log),
		BrandCSS:      brandCSS,
		BrandFonts:    brandFonts,
		Logo:          logo,
		SidebarLogo:   sideLogo,
		Root:          sel.Config.Root,
		OutDir:        dir,
		Title:         sel.Config.Title,
		Lang:          sel.Config.Lang,
		CalloutTitles: sel.Config.CalloutTitles,
		Topics:        topics,
		Keep:          sel.Keep,
		Skip:          sel.Excludes,
		Formats:       sel.Types(),
		CSS:           css,
		Expand:        exp.Expand,
		Draft:         sel.Draft,
		Log:           log,

		NumberedHeadings: sel.Format.NumberedHeadings,
		FigureLabel:      sel.FigureLabel(),
		AllTargets:       sync.OnceValue(func() markdown.Index { return markdown.NewIndex(ProjectTargets(sel.Config)) }),
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

// RefProblem is a cross-reference that a render could not link: its
// page's source file, project-relative, and what is wrong.
type RefProblem struct{ File, Message string }

// CheckReferences resolves the cross-references of every page of the
// project's topics against the headings and figures of the whole project,
// all content kept, and reports those that name nothing, are ambiguous,
// or name their page by its path rather than its `id:`.
func CheckReferences(cfg *config.Config) []RefProblem {
	type page struct {
		file string
		doc  *markdown.Document
		key  string
	}
	var pages []page
	var all []markdown.Target
	for _, e := range cfg.Topics {
		tree, err := loadTopic(cfg.Root, e.Value)
		if err != nil {
			continue
		}
		bookmaker.Pages(tree, e.Value.Title)
		exp := Expander(config.Selection{Config: cfg, Topic: e.Value}, nil)
		tree.Walk(func(n *bookmaker.Node) {
			if n.Page == nil {
				return
			}
			md, _ := bookmaker.PageMarkdown(n, "", bookmaker.Options{ProjectRoot: cfg.Root})
			if s, err := exp.Expand(md); err == nil {
				md = s
			}
			doc := markdown.Parse([]byte(md))
			markdown.AssignIDs(doc)
			all = append(all, markdown.Targets(doc, n.Rel, n.Page.PageID(), nil)...)
			pages = append(pages, page{n.Page.Rel, doc, n.Rel})
		})
	}
	idx := markdown.NewIndex(all)
	ids := map[string]bool{}
	for _, t := range all {
		if t.PageID != "" {
			ids[t.PageID] = true
		}
	}
	var out []RefProblem
	for _, p := range pages {
		report := func(format string, args ...any) {
			out = append(out, RefProblem{p.file, fmt.Sprintf(format, args...)})
		}
		_ = ast.Walk(p.doc.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if l, ok := n.(*ast.Link); ok && entering {
				if q, _, ok := strings.Cut(string(l.Destination), ":"); ok && strings.HasPrefix(q, "#") && strings.Contains(q, "/") && !ids[q[1:]] {
					report("reference %q names its page by its path, which changes when the page moves; give the page an `id:` in its front matter and use that", string(l.Destination))
				}
			}
			return ast.WalkContinue, nil
		})
		markdown.Resolver{Page: p.key, Local: idx, Warn: report, Unnumbered: true,
			Href: func(t markdown.Target) string { return "#" + t.ID }}.Resolve(p.doc)
	}
	return out
}
