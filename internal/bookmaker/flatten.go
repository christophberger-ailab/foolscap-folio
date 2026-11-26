package bookmaker

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/foliocore"
	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
	"github.com/christophberger-ailab/foolscap-folio/internal/shortcode"
)

// Options controls how a book folder is flattened.
type Options struct {
	// ProjectRoot is the Quarto project root. Page paths are expressed
	// relative to it, which is what makes website-absolute media paths such
	// as `/assets/images/x.png` keep working in the generated file.
	ProjectRoot string

	// Title overrides the book title, which the flattened document carries
	// as its first level-1 heading. When empty the title is taken from the
	// book root's index.qmd.
	Title string

	// RewriteLinks turns links that point at another page of this book into
	// anchor references. Links leaving the book are never touched.
	RewriteLinks bool

	// WrapAudience wraps content coming from a folder with an `_FW` or
	// `_POL` suffix in a matching `::: fw` / `::: pol` div, so that the
	// project's targeted-content filter can select the right variant at
	// render time.
	WrapAudience bool

	// ExtractSlides moves the content of every `::: slide` div out of the
	// book and into Result.Slides, a document of its own. Without it the
	// slide divs stay in the book like any other div.
	ExtractSlides bool

	// SectionPrefix and ChapterPrefix, if set, go before the title of
	// every section and chapter, with its number: "Part 2: Title".
	// Sections and chapters are each numbered through the book.
	SectionPrefix, ChapterPrefix string

	// Draft shows below each page's title that the page is new and what
	// to review on it, as its front matter says (spec-render.yaml DRAFT).
	Draft bool

	// site renders a page for a website, which keeps its headings as
	// written. PageMarkdown sets it.
	site bool
}

// SectionClass is the class of the div a section's page is wrapped in, so
// that the renderers can set it apart from the chapters.
const SectionClass = markdown.SectionClass

// TOCClass is the class of the empty div that marks where the contents
// go: after the book's preliminaries, before its first section.
const TOCClass = markdown.TOCClass

// Result is the outcome of flattening a book folder.
type Result struct {
	// Content is the complete generated .qmd document.
	Content string
	// Slides is the generated slide document: the content of every
	// `::: slide` div of the book, its fences removed, in book order. It
	// is empty when the book has no slides or ExtractSlides is off.
	Slides string
	// Pages is the number of source pages that contributed content.
	Pages int
	// Links is the number of in-book links rewritten into anchors.
	Links int
	// SlideBlocks is the number of slide divs moved into Slides.
	SlideBlocks int
	// Warnings collects non-fatal problems found while flattening.
	Warnings []string

	// sections and chapters count the book's sections and chapters so
	// far, for their prefixes.
	sections, chapters int
}

// bookTitleClasses are the classes put on the book's own title heading --
// the document's first level-1 heading. It names the book rather than a
// chapter of it, so it must stay out of the chapter numbering.
var bookTitleClasses = []string{"unnumbered"}

// Flatten renders a book tree into a single Quarto document.
//
// With Options.ExtractSlides the book's slide content is separated out into
// a second document, Result.Slides: a deck's structure is its own headings,
// which the chapter and section headings generated around the divs would
// only get in the way of.
//
// The documents deliberately carry no YAML front matter. These files are
// meant to be pulled into a book chapter with `{{< include >}}`, and Quarto
// reads a chapter's opening heading with a line scanner that does not know
// it is looking at an included file: the `---` closing an inlined
// front-matter block reads as a Setext underline, and the book ends up
// titled `title: <whatever the first include declared>`. The book title
// belongs in the project's `book:` configuration; here it is the first
// level-1 heading.
func Flatten(root *Node, opts Options) (*Result, error) {
	res := &Result{}
	assignAnchors(root)
	root.bookRoot = true
	root.titleOverride = opts.Title

	var targets linkTargets
	if opts.RewriteLinks {
		targets = buildLinkTargets(root)
	}

	content, slides := renderNode(root, "", targets, opts, res)
	res.Content = content + "\n"
	if slides != "" {
		res.Slides = slides + "\n"
	}

	sort.Strings(res.Warnings)
	return res, nil
}

// renderNode renders a node and its descendants, deepest structure last,
// returning the book text and the slide text side by side. parentAudience
// carries the agency marker already applied by an ancestor so that a `_FW`
// folder nested inside another `_FW` folder is wrapped once.
func renderNode(n *Node, parentAudience string, targets linkTargets, opts Options, res *Result) (book, slides string) {
	bookBlocks := make([]string, 0, len(n.Children)+1)
	slideBlocks := make([]string, 0, len(n.Children)+1)

	page, pageSlides := renderPage(n, targets, opts, res)
	if page != "" && n.Depth == 1 {
		page = wrapInDiv(page, SectionClass)
	}
	if page != "" {
		bookBlocks = append(bookBlocks, page)
	}
	if n.bookRoot {
		bookBlocks = append(bookBlocks, "::: "+TOCClass+"\n:::")
	}
	if pageSlides != "" {
		slideBlocks = append(slideBlocks, pageSlides)
	}

	audience := parentAudience
	if opts.WrapAudience && n.Audience != "" {
		audience = n.Audience
	}

	for _, child := range n.Children {
		sub, subSlides := renderNode(child, audience, targets, opts, res)
		if sub != "" {
			bookBlocks = append(bookBlocks, sub)
		}
		if subSlides != "" {
			slideBlocks = append(slideBlocks, subSlides)
		}
	}

	book = strings.Join(bookBlocks, "\n\n")
	slides = strings.Join(slideBlocks, "\n\n")

	if opts.WrapAudience && n.Audience != "" && n.Audience != parentAudience {
		// The deck is built per agency just as the book is, so the
		// slides lifted out of an `_FW` / `_POL` folder keep the marker
		// that tells the content filter whom they are for.
		if book != "" {
			book = wrapInDiv(book, n.Audience)
		}
		if slides != "" {
			slides = wrapInDiv(slides, n.Audience)
		}
	}
	return book, slides
}

// renderPage produces the flattened Markdown for a single node's own page:
// markup balanced, in-book links resolved and headings normalised to the
// node's level with an anchor attached. Its second return value is the
// page's slide content, which none of that applies to.
func renderPage(n *Node, targets linkTargets, opts Options, res *Result) (page, slides string) {
	lines, slideLines := preparePage(n, targets, opts, res)
	slides = strings.Join(slideLines, "\n")

	headings := findHeadings(lines)
	base := minHeadingLevel(headings)

	// Decide whether the page already carries its own title heading.
	//
	// Quarto's rule for a book chapter is that the `title:` front-matter
	// field is the chapter heading, unless the body supplies one itself.
	// Pages come in both shapes: some repeat the title as a heading inside
	// every content div, others give only sub-headings and rely on the
	// front matter. Treating the body's shallowest heading as the page
	// title in the second case would wrongly promote a section to chapter
	// rank, so a declared title counts as present in the body only when a
	// heading at that shallowest level actually spells it out.
	declared, banner := "", ""
	if n.Page != nil {
		declared = n.Page.Title
		if opts.Draft {
			banner = draftBanner(n.Page.FrontMatter)
		}
	}

	edit := titleEdit{ID: n.Anchor, Text: n.titleOverride}
	if n.bookRoot {
		edit.Classes = bookTitleClasses
	}

	// A website keeps a page's headings as written. In print, each tree
	// level has its meaning (issue #96): the book root's preliminaries,
	// sections and chapters keep theirs too, and anything below a chapter
	// goes down a level for each level of the tree.
	delta := 0
	if !opts.site {
		delta = max(n.Depth-2, 0)
		edit.Prefix = prefix(n, opts, res)
	}
	level := 1 + delta

	// Includes are expanded before Quarto reads the page, so a page without
	// a title whose included files bring the heading needs none generated.
	if base == 0 && declared == "" && !n.bookRoot && n.Page != nil && opts.ProjectRoot != "" &&
		includesHeading(lines, opts.ProjectRoot, path.Dir(n.Page.Rel), 0) {
		return below(banner, strings.Join(lines, "\n")), slides
	}

	if base == 0 || !echoesTitle(headings, base, declared) {
		head := strings.Repeat("#", level) + " " + edit.Prefix + n.Title() +
			" " + mergeAttrs("", n.Anchor, edit.Classes)
		head = strings.TrimRight(below(head, banner), "\n")
		if base == 0 {
			if len(lines) == 0 {
				return head, slides
			}
			return head + "\n\n" + strings.Join(lines, "\n"), slides
		}
		// In print, body headings are subordinate to the generated title
		// heading. They stay below it even where the page's own levels
		// would put them beside it.
		target := base + delta
		if !opts.site {
			target = max(target, level+1)
		}
		lines = shiftHeadings(lines, headings, n, target, base, titleEdit{}, res)
		return head + "\n\n" + strings.Join(lines, "\n"), slides
	}

	lines = shiftHeadings(lines, headings, n, base+delta, base, edit, res)
	if banner != "" {
		lines = bannerBelowTitles(lines, headings, base, banner)
	}
	return strings.Join(lines, "\n"), slides
}

// draftBanner is what a draft shows below a page's title: 🔖 for a
// bookmarked page, which the renderers set in the corner of the page
// (markdown.BookmarkClass), 🆕 for a new page, and its review in a box.
func draftBanner(fm FrontMatter) string {
	var parts []string
	if fm.IsBookmarked() {
		parts = append(parts, "::: {."+markdown.BookmarkClass+"}\n🔖\n:::")
	}
	if fm.IsNew() {
		parts = append(parts, "🆕")
	}
	if text, ok := fm.Review(); ok {
		if text == "" {
			text = "**Needs review**"
		} else {
			text = "**Review:** " + text
		}
		parts = append(parts, "::: {.callout-warning .folio-review title=\"\"}\n"+text+"\n:::")
	}
	return strings.Join(parts, "\n\n")
}

// below joins b below a with a blank line between them; either may be
// empty.
func below(a, b string) string {
	if a == "" || b == "" {
		return a + b
	}
	return a + "\n\n" + b
}

// bannerBelowTitles puts the banner below every heading of lines that is
// the page's title: the first at level base, and those that repeat it in
// sibling divs, of which the render may keep any one.
func bannerBelowTitles(lines []string, headings []heading, base int, banner string) []string {
	title := ""
	var at []int
	for _, h := range headings {
		if h.Level != base {
			continue
		}
		if title == "" {
			title = normaliseTitle(h.Text())
		}
		if normaliseTitle(h.Text()) == title {
			at = append(at, h.Index)
		}
	}
	for i := len(at) - 1; i >= 0; i-- {
		lines = slices.Insert(lines, at[i]+1, "", banner)
	}
	return lines
}

// prefix returns what goes before the title of n in print: the section or
// chapter prefix and its number, "" for the other nodes or with no prefix.
// It counts n as the next section or chapter.
func prefix(n *Node, opts Options, res *Result) string {
	switch {
	case n.Depth == 1 && opts.SectionPrefix != "":
		res.sections++
		return fmt.Sprintf("%s %d: ", opts.SectionPrefix, res.sections)
	case n.Depth == 2 && opts.ChapterPrefix != "":
		res.chapters++
		return fmt.Sprintf("%s %d: ", opts.ChapterPrefix, res.chapters)
	}
	return ""
}

// preparePage turns a node's source body into the lines that will appear in
// the flattened document, before any heading adjustment, and hands back the
// slide content taken out of it.
func preparePage(n *Node, targets linkTargets, opts Options, res *Result) (lines, slides []string) {
	if n.Page == nil {
		if n.IsDir() {
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"%s: directory has no index.qmd; a heading was generated from its name", n.Rel))
		}
		return nil, nil
	}

	lines = trimBlankEdges(strings.Split(n.Page.Body, "\n"))

	lines, repair := balanceFences(lines)
	if !repair.clean() {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%s: unbalanced markup repaired (%s); the page renders on its own but would leak into the rest of the book",
			n.Page.Rel, describeRepair(repair)))
	}

	if opts.ExtractSlides {
		// Slides are taken out before the page is touched any further:
		// heading levels are what cuts a deck into slides, and the
		// anchors an in-book link needs do not exist in the deck.
		var count int
		lines, slides, count = splitSlides(lines)
		res.SlideBlocks += count
	}

	if opts.RewriteLinks {
		var changed int
		lines, changed = rewriteLinks(lines, targets, path.Dir(n.Page.Rel))
		res.Links += changed
	}

	lines = absoluteIncludes(lines, path.Dir(n.Page.Rel))

	res.Pages++
	return lines, slides
}

// echoesTitle reports whether one of the page's shallowest headings restates
// the declared front-matter title, meaning the body already provides the page
// heading.
func echoesTitle(headings []heading, base int, declared string) bool {
	want := normaliseTitle(declared)
	if want == "" {
		// Without a declared title the body's shallowest heading is the
		// page heading by definition, which is Quarto's own rule.
		return true
	}
	for _, h := range headings {
		if h.Level == base && normaliseTitle(h.Text()) == want {
			return true
		}
	}
	return false
}

func normaliseTitle(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '*', '_', '`', '"', '\'', '“', '”', '„':
			return -1
		}
		return r
	}, s)
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// shiftHeadings rewrites every heading so that the shallowest one lands on
// target, preserving the relative depth of the rest, and applies edit to the
// page's own title heading.
//
// The anchor id is attached to the page's own title heading. Sibling content
// divs (`::: explanation`, `::: tutorial`, ...) typically repeat that heading
// verbatim and only some of them survive the project's content filter, so
// every repetition claims the id -- once per div. Pandoc renames the
// duplicates that do survive, leaving the anchor on the first one. Headings
// at the same level that say something else are ordinary sections and are
// left alone.
func shiftHeadings(lines []string, headings []heading, n *Node, target, base int, edit titleEdit, res *Result) []string {
	delta := target - base
	out := append([]string(nil), lines...)

	titleText := ""
	for _, h := range headings {
		if h.Level == base {
			titleText = normaliseTitle(h.Text())
			break
		}
	}
	claimed := map[int]bool{}

	for _, h := range headings {
		level := h.Level + delta
		if level > 6 {
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"%s: heading %q would be at level %d, deeper than Markdown's 6; it is set as emphasized text",
				n.Rel, h.Text(), level))
			out[h.Index] = mockHeading(h, level)
			continue
		}

		applied := titleEdit{}
		if !edit.empty() && level == target && !claimed[h.Block] && normaliseTitle(h.Text()) == titleText {
			applied = edit
			claimed[h.Block] = true
		}
		out[h.Index] = renderHeading(h, level, applied)
	}
	return out
}

// mockMarks are the emphasis that stands in for the headings Markdown does
// not have: bold for h7, bold italics for h8, italics for h9 and deeper.
var mockMarks = map[int]string{7: "**", 8: "***"}

// mockHeading sets a heading of a level beyond 6 as a paragraph of
// emphasized text.
func mockHeading(h heading, level int) string {
	mark, ok := mockMarks[level]
	if !ok {
		mark = "*"
	}
	return h.Indent + mark + h.Text() + mark
}

// describeRepair renders a fenceRepair for a warning message.
func describeRepair(r fenceRepair) string {
	var parts []string
	if r.Code {
		parts = append(parts, "closed an unterminated code fence")
	}
	if r.Divs > 0 {
		parts = append(parts, fmt.Sprintf("closed %d unterminated div(s)", r.Divs))
	}
	if r.Stray > 0 {
		parts = append(parts, fmt.Sprintf("dropped %d stray closing fence(s)", r.Stray))
	}
	if r.Glued > 0 {
		parts = append(parts, fmt.Sprintf(
			"separated %d div fence(s) from the text above, which Pandoc would have read as part of it", r.Glued))
	}
	return strings.Join(parts, ", ")
}

// wrapInDiv encloses content in a fenced div with the given class, using a
// fence wider than anything the content itself contains.
func wrapInDiv(content, class string) string {
	lines := strings.Split(content, "\n")
	width := maxFenceWidth(lines) + 1
	if width < 3 {
		width = 3
	}
	fence := strings.Repeat(":", width)
	return fence + " " + class + "\n" + content + "\n" + fence
}

// trimBlankEdges removes leading and trailing blank lines.
func trimBlankEdges(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

// includeLine matches a line that is one `{{< include path >}}` shortcode,
// the form the shortcode expander resolves, and captures the text before
// the path, the path (quoted or not) and the text after it.
var includeLine = regexp.MustCompile(`^(\s*\{\{<\s*include\s+)("[^"]*"|'[^']*'|[^\s>"']+)(\s*>\}\}\s*)$`)

// absoluteIncludes rewrites the relative include paths in a page's lines,
// which are relative to the page's folder dir, into project-absolute ones.
// The page's text ends up in a book or a site page that no longer knows
// where it came from; an absolute path still finds the file. Includes in
// fenced code blocks are left as written.
func absoluteIncludes(lines []string, dir string) []string {
	var code codeTracker
	for i, line := range lines {
		if code.step(line) {
			continue
		}
		m := includeLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		p := strings.Trim(m[2], `"'`)
		if p == "" || strings.HasPrefix(p, "/") {
			continue
		}
		abs := "/" + path.Join(dir, p)
		if m[2] != p {
			abs = m[2][:1] + abs + m[2][:1]
		}
		lines[i] = m[1] + abs + m[3]
	}
	return lines
}

// includesHeading reports whether a file included by lines, directly or
// through further includes, contributes a heading. dir is the project-relative
// folder of the file holding lines. Includes in code blocks are ignored, and
// so are files that cannot be read: the shortcode expander reports those.
func includesHeading(lines []string, root, dir string, depth int) bool {
	if depth >= shortcode.MaxIncludeDepth {
		return false
	}
	var code codeTracker
	for _, line := range lines {
		m := includeLine.FindStringSubmatch(line)
		if code.step(line) || m == nil {
			continue
		}
		rel := shortcode.ResolveInclude(dir, strings.Trim(m[2], `"'`))
		abs, err := foliocore.ProjectPath(root, rel)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		_, body, _ := foliocore.SplitFrontMatter(normaliseNewlines(string(data)))
		sub := strings.Split(body, "\n")
		if len(findHeadings(sub)) > 0 || includesHeading(sub, root, path.Dir(rel), depth+1) {
			return true
		}
	}
	return false
}

// Pages prepares a tree for rendering page by page, as a website does:
// every node gets its anchor and the root is marked as the book root, as
// Flatten would do. title overrides the root's title.
func Pages(root *Node, title string) {
	assignAnchors(root)
	root.bookRoot = true
	root.titleOverride = title
}

// PageMarkdown renders a single node's own page — not its children — for a
// website: markup balanced and a title heading generated where the page
// has none, the page's own headings as written. Links are left alone. With opts.WrapAudience the page is
// wrapped in a div of the given audience class, which the caller works
// out from the node and its ancestors. Call Pages on the tree first.
func PageMarkdown(n *Node, audience string, opts Options) (string, []string) {
	opts.RewriteLinks, opts.site = false, true
	res := &Result{}
	page, _ := renderPage(n, nil, opts, res)
	if opts.WrapAudience && audience != "" && page != "" {
		page = wrapInDiv(page, audience)
	}
	return page, res.Warnings
}
