package site

import (
	"bytes"
	"cmp"
	"embed"
	"fmt"
	"html"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/bookmaker"
	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
	"github.com/yuin/goldmark/ast"
	"go.yaml.in/yaml/v4"
)

//go:embed assets/*
var assetFS embed.FS

// Topic is one top-level section of the site: a menu entry and a sidebar.
type Topic struct {
	Name string
	// Folder is the topic's content folder, relative to the project root.
	Folder string
	// Title is the menu text; "" uses the title of the folder's index page.
	Title string
}

// Options configure Build.
type Options struct {
	Root   string // project root, absolute
	OutDir string // output directory, absolute
	Title  string // site title; "" = title of the root index page, else name of the root folder
	Lang   string // html lang attribute
	// CalloutTitles are the user's own callout titles by language and
	// type (see markdown.CalloutTitles).
	CalloutTitles map[string]map[string]string
	Topics        []Topic
	Keep          markdown.Keep
	// Skip reports the folder and file names (without extension) that the
	// site leaves out, with everything below them; may be nil.
	Skip func(name string) bool
	// Ignore leaves out what the project's ignore patterns name.
	Ignore  config.Ignore
	Formats []string
	// CSS are stylesheets, absolute paths, that every page links after the
	// site's own. They are copied verbatim into OutDir/_site/css.
	CSS []string
	// BrandCSS is the stylesheet of the project's brand, which every page
	// links between the site's own and CSS; "" for none. BrandFonts are
	// the font files it refers to: name below _site/fonts to source file.
	BrandCSS   string
	BrandFonts map[string]string
	// Logo is shown in the navbar before the title and is the site's icon.
	// SidebarLogo, if set, stands above the sidebar instead, while the
	// sidebar is beside the page.
	Logo, SidebarLogo Logo
	// Footer returns the Markdown footer of a page of a topic, by the
	// topic's name, "" for the home page, and the page's front matter,
	// nil for a page without source; "" for none. May be nil.
	Footer func(topic string, meta map[string]any) string
	// Expand expands shortcodes in a page's Markdown; may be nil.
	Expand func(string) (string, error)
	// Draft shows todos, comments, reviews and new pages, and keeps the
	// pages marked `draft: true`, which are otherwise left out.
	Draft bool
	// NumberedHeadings numbers the headings of each topic as its book
	// numbers them (see markdown.NumberHeadings); FigureLabel, if set,
	// numbers its figures, "Figure 3", through the topic.
	NumberedHeadings bool
	FigureLabel      string
	// SectionPrefix and ChapterPrefix, if set, go before the titles of
	// each topic's sections and chapters with their number, "Part 2: ",
	// as the topic's book puts them (issue #179).
	SectionPrefix, ChapterPrefix string
	// AllTargets returns the headings and figures of the whole project,
	// for references to pages the site leaves out; may be nil.
	AllTargets func() markdown.Index
	// Log receives warnings and progress; may be nil.
	Log func(format string, args ...any)
}

// Logo is the site's logo: image files, absolute, for the light and the
// dark color scheme, and their alternative text. Without Light there is
// no logo; without Dark the light one serves both schemes.
type Logo struct{ Light, Dark, Alt string }

// out is where the logo's files go, OutDir-relative, named by stem.
func (l Logo) out(stem string) Logo {
	if l.Light == "" {
		return Logo{}
	}
	o := Logo{Light: "_site/" + stem + strings.ToLower(filepath.Ext(l.Light)), Alt: l.Alt}
	if l.Dark != "" {
		o.Dark = "_site/" + stem + "-dark" + strings.ToLower(filepath.Ext(l.Dark))
	}
	return o
}

// in returns the logo with its paths below base.
func (l Logo) in(base string) Logo {
	if l.Light != "" {
		l.Light = base + l.Light
	}
	if l.Dark != "" {
		l.Dark = base + l.Dark
	}
	return l
}

// pageInfo is one page of the site.
type pageInfo struct {
	node     *bookmaker.Node
	topic    *topicInfo // nil for the home page
	out      string     // output file, slash-separated, relative to OutDir
	title    string
	audience string
	srcDir   string // project-relative directory links resolve against
	home     bool
	// landing, when set, is generated content for a page without source.
	landing template.HTML
	// doc is the page's prepared document, targets its headings and
	// figures.
	doc     *markdown.Document
	targets []markdown.Target
}

type topicInfo struct {
	Topic
	title string
	root  *bookmaker.Node
	pages []*pageInfo // in sidebar order
	home  *pageInfo   // the topic's entry page
	byNod map[*bookmaker.Node]*pageInfo
}

type builder struct {
	o        Options
	root     string
	title    string
	topics   []*topicInfo
	pages    map[string]*pageInfo // by source key: node Rel, "" for home
	homePg   *pageInfo
	copied   map[string]bool
	tmpl     *template.Template
	labels   labels
	logo     Logo     // OutDir-relative paths of the logo's files
	sideLogo Logo     // likewise, of the sidebar's logo
	css      []string // OutDir-relative paths of the extra stylesheets
	outAbs   string
	rootAbs  string
	search   []searchDoc
	local    markdown.Index // the targets of the site's pages
}

func (b *builder) logf(format string, args ...any) {
	if b.o.Log != nil {
		b.o.Log(format, args...)
	}
}

// Build renders the website into o.OutDir.
func Build(o Options) error {
	if o.Root == "" || o.OutDir == "" {
		return fmt.Errorf("site: Root and OutDir are required")
	}
	root, err := filepath.Abs(o.Root)
	if err != nil {
		return err
	}
	out, err := filepath.Abs(o.OutDir)
	if err != nil {
		return err
	}
	b := &builder{o: o, root: root, rootAbs: root, outAbs: out,
		pages: map[string]*pageInfo{}, copied: map[string]bool{}}
	b.title = o.Title
	if b.o.Lang == "" {
		b.o.Lang = "en"
	}
	b.labels = labelsFor(b.o.Lang)
	b.logo, b.sideLogo = o.Logo.out("logo"), o.SidebarLogo.out("logo-medium")
	for i, c := range o.CSS {
		name := strings.TrimSuffix(filepath.Base(c), filepath.Ext(c))
		b.css = append(b.css, fmt.Sprintf("_site/css/%d-%s.css", i+1, name))
	}

	b.tmpl, err = template.ParseFS(assetFS, "assets/page.html")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := b.load(); err != nil {
		return err
	}
	for _, p := range b.allPages() {
		if err := b.preparePage(p); err != nil {
			return err
		}
	}
	if err := b.number(); err != nil {
		return err
	}
	var local []markdown.Target
	for _, p := range b.allPages() {
		local = append(local, p.targets...)
	}
	b.local = markdown.NewIndex(local)
	for _, p := range b.allPages() {
		if err := b.renderPage(p); err != nil {
			return err
		}
	}
	return b.writeStatic()
}

func (b *builder) allPages() []*pageInfo {
	var all []*pageInfo
	if b.homePg != nil {
		all = append(all, b.homePg)
	}
	for _, t := range b.topics {
		all = append(all, t.pages...)
	}
	return all
}

// load reads the topic trees and the home page.
func (b *builder) load() error {
	for _, name := range []string{"index.qmd", "index.md"} {
		file := filepath.Join(b.root, name)
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		title, body := splitFront(string(data))
		n := &bookmaker.Node{Name: "index", Rel: "index",
			Page: &bookmaker.Page{File: file, Rel: name, Body: body}}
		n.Page.Title = title
		bookmaker.Pages(n, "")
		p := &pageInfo{node: n, out: "index.html", title: n.Title(), srcDir: ".", home: true}
		b.homePg = p
		b.pages[""] = p
		b.title = cmp.Or(b.title, title)
		break
	}
	b.title = cmp.Or(b.title, filepath.Base(b.root))
	for _, t := range b.o.Topics {
		dir := filepath.Join(b.root, filepath.FromSlash(t.Folder))
		tree, err := bookmaker.LoadTree(dir, b.root, b.o.Ignore)
		if err != nil {
			return fmt.Errorf("site: topic %q: %w", t.Name, err)
		}
		if b.o.Skip != nil {
			tree.Drop(b.o.Skip)
		}
		if !b.o.Draft {
			tree.DropDrafts()
		}
		bookmaker.Pages(tree, t.Title)
		ti := &topicInfo{Topic: t, root: tree, title: tree.Title(), byNod: map[*bookmaker.Node]*pageInfo{}}
		var walk func(n *bookmaker.Node, aud string)
		walk = func(n *bookmaker.Node, aud string) {
			if n.Audience != "" {
				aud = n.Audience
			}
			if n.Page != nil {
				p := &pageInfo{node: n, topic: ti, title: n.Title(), audience: aud, srcDir: path.Dir(n.Page.Rel)}
				if n.IsDir() {
					p.out = n.Rel + "/index.html"
				} else {
					p.out = n.Rel + ".html"
				}
				b.pages[n.Rel] = p
				ti.pages = append(ti.pages, p)
				ti.byNod[n] = p
			}
			for _, c := range n.Children {
				walk(c, aud)
			}
		}
		walk(tree, "")
		if len(ti.pages) == 0 {
			return fmt.Errorf("site: topic %q (%s) has no pages", t.Name, t.Folder)
		}
		ti.home = ti.pages[0]
		b.topics = append(b.topics, ti)
	}
	if b.homePg == nil {
		b.homePg = &pageInfo{out: "index.html", title: b.title, srcDir: ".", home: true,
			landing: b.landingHTML()}
	}
	return nil
}

func (b *builder) landingHTML() template.HTML {
	var s strings.Builder
	s.WriteString("<h1 class=\"page-title\">" + html.EscapeString(b.title) + "</h1>\n<ul class=\"topic-list\">\n")
	for _, t := range b.topics {
		s.WriteString(`<li><a href="` + html.EscapeString(relURL("index.html", t.home.out)) + `">` +
			html.EscapeString(t.title) + "</a></li>\n")
	}
	s.WriteString("</ul>\n")
	return template.HTML(s.String())
}

// splitFront separates the title from the body of a page without a tree.
func splitFront(src string) (title, body string) {
	src = strings.ReplaceAll(strings.TrimPrefix(src, "\xEF\xBB\xBF"), "\r\n", "\n")
	if !strings.HasPrefix(src, "---\n") {
		return "", src
	}
	rest := src[4:]
	for _, end := range []string{"\n---\n", "\n...\n"} {
		if i := strings.Index(rest, end); i >= 0 {
			var fm struct {
				Title string `yaml:"title"`
			}
			_ = yaml.Unmarshal([]byte(rest[:i]), &fm)
			return strings.TrimSpace(fm.Title), rest[i+len(end):]
		}
	}
	if strings.HasSuffix(rest, "\n---") {
		return "", ""
	}
	return "", src
}

type tocItem struct {
	Text, ID string
	Level    int // 1 or 2
}

// key is the page's key in b.pages, which its targets name it by.
func (p *pageInfo) key() string {
	if p.home {
		return ""
	}
	return p.node.Rel
}

// preparePage reads, parses and prepares a page's document and lists its
// headings and figures.
func (b *builder) preparePage(p *pageInfo) (err error) {
	if p.landing != "" {
		return nil
	}
	md, warns := bookmaker.PageMarkdown(p.node, p.audience,
		bookmaker.Options{ProjectRoot: b.root, WrapAudience: true, Draft: b.o.Draft})
	for _, w := range warns {
		b.logf("warning: %s", w)
	}
	if b.o.Expand != nil {
		if md, err = b.o.Expand(md); err != nil {
			return fmt.Errorf("site: %s: %w", p.node.Page.Rel, err)
		}
	}
	warn := func(format string, args ...any) {
		b.logf("warning: %s: "+format, append([]any{p.node.Page.Rel}, args...)...)
	}
	p.doc = markdown.Parse([]byte(md))
	p.doc.Prepare(markdown.Options{Keep: b.o.Keep, Formats: b.o.Formats, Lang: b.o.Lang, CalloutTitles: b.o.CalloutTitles, Draft: b.o.Draft, Warn: warn})
	p.targets = markdown.Targets(p.doc, p.key(), p.node.Page.PageID(), warn)
	return nil
}

func (b *builder) renderPage(p *pageInfo) (err error) {
	var content template.HTML
	var toc []tocItem
	if p.landing != "" {
		content = p.landing
	} else {
		doc := p.doc
		b.rewriteLinks(doc, p)
		markdown.Resolver{Page: p.key(), Local: b.local, All: b.o.AllTargets,
			Href: func(t markdown.Target) string {
				if t.Page == p.key() {
					return "#" + t.ID
				}
				return relURL(p.out, b.pages[t.Page].out) + "#" + t.ID
			},
			Warn: func(format string, args ...any) { b.logf("warning: "+format, args...) },
		}.Resolve(doc)
		b.expandSubpages(doc, p)
		toc = tocOf(doc)
		var s string
		if s, err = markdown.HTML(doc); err != nil {
			return fmt.Errorf("site: %s: %w", p.node.Page.Rel, err)
		}
		content = template.HTML(s)
		sd := searchDoc{Title: p.title, URL: p.out, Body: htmlText(s)}
		if p.topic != nil {
			sd.Section = p.topic.title
		}
		b.search = append(b.search, sd)
	}
	data := b.pageData(p, content, toc)
	if data.Footer, err = b.footer(p); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := b.tmpl.ExecuteTemplate(&buf, "page.html", data); err != nil {
		return err
	}
	dst := filepath.Join(b.outAbs, filepath.FromSlash(p.out))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	b.logf("site: %s", p.out)
	return os.WriteFile(dst, buf.Bytes(), 0o644)
}

// tocOf lists the headings one and two levels below the page's title
// heading (the shallowest heading). The first heading of that level is
// marked as the page title.
func tocOf(doc *markdown.Document) []tocItem {
	var hs []*ast.Heading
	_ = ast.Walk(doc.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering {
			hs = append(hs, h)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if len(hs) == 0 {
		return nil
	}
	base := 7
	for _, h := range hs {
		base = min(base, h.Level)
	}
	if hs[0].Level == base {
		hs[0].SetAttributeString("class", "page-title")
	}
	var items []tocItem
	for _, h := range hs {
		d := h.Level - base
		if d < 1 || d > 2 {
			continue
		}
		items = append(items, tocItem{Text: markdown.PlainText(h, doc.Source), ID: markdown.HeadingID(h), Level: d})
	}
	if len(items) < 2 {
		return nil
	}
	return items
}

// footer renders the footer of p's pages, a single paragraph without its
// <p>.
func (b *builder) footer(p *pageInfo) (template.HTML, error) {
	if b.o.Footer == nil {
		return "", nil
	}
	topic := ""
	if p.topic != nil {
		topic = p.topic.Name
	}
	var meta map[string]any
	if p.node != nil && p.node.Page != nil {
		meta = p.node.Page.Meta
	}
	md := b.o.Footer(topic, meta)
	if md == "" {
		return "", nil
	}
	doc := markdown.Parse([]byte(md))
	doc.Prepare(markdown.Options{Lang: b.o.Lang})
	s, err := markdown.HTML(doc)
	if err != nil {
		return "", fmt.Errorf("site: footer: %w", err)
	}
	s = strings.TrimSpace(s)
	if inner, ok := strings.CutPrefix(s, "<p>"); ok && strings.Count(s, "<p>") == 1 {
		s = strings.TrimSuffix(inner, "</p>")
	}
	return template.HTML(s), nil
}

// number numbers the headings and figures of every topic's pages as the
// topic's book numbers them, when the options ask for it: the topic is
// flattened, prefixed and numbered as a document is, and each page takes
// the prefixes and numbers of its headings, by key, and the numbers of its
// figures, in order.
func (b *builder) number() error {
	if !b.o.NumberedHeadings && b.o.FigureLabel == "" && b.o.SectionPrefix == "" && b.o.ChapterPrefix == "" {
		return nil
	}
	for _, t := range b.topics {
		res, err := bookmaker.Flatten(t.root, bookmaker.Options{ProjectRoot: b.root, Title: t.Title,
			WrapAudience: true, PageMarkers: true, Draft: b.o.Draft,
			SectionPrefix: b.o.SectionPrefix, ChapterPrefix: b.o.ChapterPrefix})
		if err != nil {
			return fmt.Errorf("site: topic %q: %w", t.Name, err)
		}
		md := res.Content
		if b.o.Expand != nil {
			if md, err = b.o.Expand(md); err != nil {
				return fmt.Errorf("site: topic %q: %w", t.Name, err)
			}
		}
		book := markdown.Parse([]byte(md))
		book.Prepare(markdown.Options{Keep: b.o.Keep, Formats: b.o.Formats, Lang: b.o.Lang, NoDiagrams: true})
		if b.o.NumberedHeadings {
			markdown.NumberHeadings(book, b.o.SectionPrefix != "", b.o.ChapterPrefix != "")
		}
		if b.o.FigureLabel != "" {
			markdown.NumberFigures(book, b.o.FigureLabel)
		}
		headings := map[[2]string]*ast.Heading{}
		figures := map[string][]string{}
		for _, x := range markdown.Targets(book, "", "", nil) {
			switch {
			case x.Number == "":
			case x.Figure:
				figures[x.Page] = append(figures[x.Page], x.Number)
			default:
				headings[[2]string{x.Page, x.Key}] = x.Node.(*ast.Heading)
			}
		}
		for _, p := range t.pages {
			if p.doc == nil {
				continue
			}
			for i, x := range p.targets {
				h, ok := x.Node.(*ast.Heading)
				bh := headings[[2]string{p.key(), x.Key}]
				if !ok || bh == nil {
					continue
				}
				if pre := markdown.HeadingPrefix(bh); pre != "" {
					markdown.SetHeadingPrefix(h, pre)
				} else {
					markdown.SetHeadingNumber(h, markdown.HeadingNumber(bh))
				}
				p.targets[i].Number = markdown.HeadingNumber(h)
			}
			labels := figures[p.key()]
			for i, img := range markdown.Figures(p.doc) {
				if i >= len(labels) {
					break
				}
				markdown.SetFigureLabel(img, labels[i])
				for j := range p.targets {
					if p.targets[j].Node == img {
						p.targets[j].Number = labels[i]
					}
				}
			}
		}
	}
	return nil
}
