package project

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
)

// Page is a node in the project tree. A page without an Order field is
// "unordered" and sorts below its ordered siblings. Dir is the directory
// that holds (or would hold) the page's children; it is empty when the
// page cannot have children (the root index page).
type Page struct {
	Path      string // slash-separated path relative to the project root
	Dir       string
	Title     string
	Order     *int
	BadFences bool   // the file has unmatched Quarto block fences
	Draft     bool   // the page's frontmatter says `draft: true`
	Marker    string // emoji marker from a _FW/_POL suffix (own or inherited)
	Flags     []Flag // status flags from the frontmatter and the body, see PageFlags
	Hidden    bool   // the page's frontmatter says `hide: true`
	Home      bool   // the root index page, the website's home page
	// TooDeep, if set, says why the page is marked: its headings, or
	// those of a page below it, would go beyond h6 in print at their
	// level of the tree (see markTooDeep).
	TooDeep  string
	Children []*Page

	// maxHeading is the deepest heading level the page's body has.
	maxHeading int
}

// Emoji markers for the _FW and _POL name suffixes.
const (
	markerFW  = "\U0001F692" // 🚒
	markerPOL = "\U0001F693" // 🚔
)

// Flag is a page's status flag as the tree shows it: an emoji, with a
// tooltip saying what it means.
type Flag struct {
	Emoji string
	Title string
}

// The status flags a page can carry, which the tree shows in this order:
// its `status:`, a `==TODO==` in it, a `review:` and a comment.
var (
	newFlag     = Flag{"\U0001F195", "New page"}          // 🆕
	oldFlag     = Flag{"\U0001F5D1\uFE0F", "Deprecated"}  // 🗑️
	todoFlag    = Flag{"\U0001F6A7", "Contains todos"}    // 🚧
	reviewFlag  = Flag{"\U0001F440", "Needs review"}      // 👀
	commentFlag = Flag{"\U0001F4AC", "Contains comments"} // 💬
)

// statusFlags maps the values of `status:` to their flags; deprecated,
// obsolete and old are one status. Any other value sets no flag.
var statusFlags = map[string]Flag{
	"new":        newFlag,
	"deprecated": oldFlag,
	"obsolete":   oldFlag,
	"old":        oldFlag,
}

// PageFlags returns the status flags src carries, each at most once, in the
// order the tree shows them. A `review:` set to anything but false sets
// 👀, and a text says what to review: the tooltip is then "Review: " and
// the text.
func PageFlags(src []byte) []Flag {
	return pageFlagsOf(src, ParseFrontmatter(src))
}

func pageFlagsOf(src []byte, fm FrontMatter) []Flag {
	var flags []Flag
	status, _ := fm.Value("status")
	text, _ := status.(string)
	if f, ok := statusFlags[strings.ToLower(strings.TrimSpace(text))]; ok {
		flags = append(flags, f)
	}
	if hasTodo(src) {
		flags = append(flags, todoFlag)
	}
	if v, ok := fm.Value("review"); ok && v != false {
		f := reviewFlag
		if text, _ := v.(string); strings.TrimSpace(text) != "" {
			f.Title = "Review: " + strings.TrimSpace(text)
		}
		flags = append(flags, f)
	}
	if hasComment(src) {
		flags = append(flags, commentFlag)
	}
	return flags
}

// hasTodo reports whether src holds a `==TODO==` anywhere in the raw file,
// so one inside an HTML comment counts too.
func hasTodo(src []byte) bool {
	return bytes.Contains(src, []byte("==TODO=="))
}

// commentStart matches the opening marker of a folio comment,
// `<!-- << The comment -->`, or of a reply to one, `<!-- <<+ ... -->`.
var commentStart = regexp.MustCompile(`<!--\s*<<`)

// hasComment reports whether src's body holds a folio comment. A marker in
// a fenced code block or a code span is code, not a comment, as it is in
// the editor and the preview.
func hasComment(src []byte) bool {
	_, body := splitFrontmatter(src)
	var text []byte
	inFence := false
	for _, ln := range bytes.Split(body, []byte("\n")) {
		trimmed := bytes.TrimLeft(ln, " ")
		if bytes.HasPrefix(trimmed, []byte("```")) || bytes.HasPrefix(trimmed, []byte("~~~")) {
			inFence = !inFence
			continue
		}
		if !inFence {
			text = append(append(text, dropCodeSpans(ln)...), '\n')
		}
	}
	return commentStart.Match(text)
}

// backticks matches a run of backticks, which opens or closes a code span.
var backticks = regexp.MustCompile("`+")

// dropCodeSpans returns ln without its code spans: a run of backticks up to
// the next run of the same length. A run without a match is literal text.
func dropCodeSpans(ln []byte) []byte {
	runs := backticks.FindAllIndex(ln, -1)
	var out []byte
	at := 0
	for i := 0; i < len(runs); i++ {
		for j := i + 1; j < len(runs); j++ {
			if runs[j][1]-runs[j][0] == runs[i][1]-runs[i][0] {
				out = append(out, ln[at:runs[i][0]]...)
				at, i = runs[j][1], j
				break
			}
		}
	}
	return append(out, ln[at:]...)
}

// IsHidden reports whether src's frontmatter says `hide: true`, which
// leaves the page out of the tree view.
func IsHidden(src []byte) bool {
	return isHidden(ParseFrontmatter(src))
}

func isHidden(fm FrontMatter) bool {
	return fm.Flag("hide")
}

// HasVisibleChildren reports whether any of the page's children is shown
// in the tree, so that a section whose children are all hidden does not
// offer to expand.
func (p *Page) HasVisibleChildren() bool {
	for _, c := range p.Children {
		if !c.Hidden {
			return true
		}
	}
	return false
}

// Tree is the page tree of a folio project.
type Tree struct {
	Root  string // absolute path of the project
	Pages []*Page
}

// SkipDir reports whether the directory at rel, a slash-separated path
// relative to the project root, is left out of the page tree: the
// top-level assets directory, which holds media, and any directory whose
// name starts with "_" or ".", at any depth.
func SkipDir(rel string) bool {
	name := path.Base(rel)
	return rel == "assets" || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")
}

// SkipWalkedDir reports whether a walk of root skips the directory p, an
// OS path below root, by the rule of SkipDir.
func SkipWalkedDir(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != "." && SkipDir(filepath.ToSlash(rel))
}

// Load scans root for .qmd and .md files and builds the page tree.
//
// index.qmd or index.md inside a directory represents that directory's
// section and is ordered among the pages one level up, as is a name.qmd or
// name.md file next to a name/ directory. Directories that SkipDir names
// are ignored.
func Load(root string) (*Tree, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if SkipWalkedDir(root, p) {
				return filepath.SkipDir
			}
			return nil
		}
		if config.IsContentFile(name) && !strings.HasPrefix(name, "_") {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Directories that (transitively) contain .qmd or .md files.
	hasFiles := map[string]bool{}
	for _, f := range files {
		for d := path.Dir(f); d != "."; d = path.Dir(d) {
			hasFiles[d] = true
		}
	}

	pages := make(map[string]*Page, len(files))
	for _, f := range files {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			return nil, err
		}
		fm := ParseFrontmatter(src)
		p := &Page{
			Path:       f,
			Title:      fm.Title,
			Order:      fm.Order,
			Draft:      fm.Draft,
			Flags:      pageFlagsOf(src, fm),
			Hidden:     isHidden(fm),
			BadFences:  !BalancedFences(src),
			maxHeading: maxHeadingLevel(src),
		}
		if p.Title == "" {
			if h := FirstHeading(src); h != "" {
				p.Title = h
			} else {
				p.Title = fallbackTitle(f)
			}
		}
		pages[f] = p
	}

	// A section page owns a directory: name/index.qmd wins over name.qmd (or .md).
	sections := map[string]*Page{}
	for f, p := range pages {
		if isIndexPage(f) && path.Dir(f) != "." {
			sections[path.Dir(f)] = p
		}
	}
	for f, p := range pages {
		d := strings.TrimSuffix(f, path.Ext(f))
		if !isIndexPage(f) && hasFiles[d] && sections[d] == nil {
			sections[d] = p
		}
	}

	// Group pages by the directory whose section they belong to.
	groups := map[string][]*Page{}
	for f, p := range pages {
		dir := path.Dir(f)
		if isIndexPage(f) {
			if dir == "." {
				p.Dir = "" // root index.qmd takes no children
			} else {
				p.Dir = dir
				dir = path.Dir(dir) // section sorts among its parent's pages
			}
		} else {
			p.Dir = strings.TrimSuffix(f, path.Ext(f))
			if sections[p.Dir] == p {
				dir = path.Dir(p.Dir)
			}
		}
		groups[dir] = append(groups[dir], p)
	}

	// Directories with pages but no section page get a synthetic node.
	for d := range groups {
		for ; d != "."; d = path.Dir(d) {
			if sections[d] == nil {
				s := &Page{Dir: d, Title: d + "/"}
				sections[d] = s
				groups[path.Dir(d)] = append(groups[path.Dir(d)], s)
			}
		}
	}

	for d, s := range sections {
		s.Children = groups[d]
		sortPages(s.Children)
	}
	t := &Tree{Root: root, Pages: groups["."]}
	sortPages(t.Pages)
	// The root index page is the home page. It comes first and needs no
	// order (issue #96).
	for i, p := range t.Pages {
		if p.Path != "" && p.Dir == "" && isIndexPage(p.Path) {
			p.Home = true
			t.Pages = append(append([]*Page{p}, t.Pages[:i]...), t.Pages[i+1:]...)
			break
		}
	}
	markPages(t.Pages, "")
	markTooDeep(t.Pages, 0)
	hideEmptyDirs(t.Pages)
	return t, nil
}

// markTooDeep marks the pages whose headings would go beyond h6 in print,
// and the pages above them, and reports whether it marked any. depth is
// the pages' level in the tree: 0 for the topics, 1 for their sections, 2
// for chapters. Print demotes the headings of a page below a chapter by
// one level for each level further down (see bookmaker.Flatten).
func markTooDeep(pages []*Page, depth int) bool {
	marked := false
	for _, p := range pages {
		if level := p.maxHeading + max(depth-2, 0); p.maxHeading > 0 && level > 6 {
			p.TooDeep = fmt.Sprintf("Headings too deep: at this level of the tree, the page's h%d renders as h%d. "+
				"Print output sets headings beyond h6 as emphasized text.", p.maxHeading, level)
		}
		if markTooDeep(p.Children, depth+1) && p.TooDeep == "" {
			p.TooDeep = "Headings too deep: a page below renders headings as h7 or deeper in print."
		}
		marked = marked || p.TooDeep != ""
	}
	return marked
}

// hideEmptyDirs hides a synthetic directory node whose pages are all
// hidden, since it would stand in the tree with nothing under it.
func hideEmptyDirs(pages []*Page) {
	for _, p := range pages {
		hideEmptyDirs(p.Children)
		if p.Path == "" && !p.HasVisibleChildren() {
			p.Hidden = true
		}
	}
}

// ownMarker returns the emoji marker implied by a page's own name suffix, or
// "" if the name has neither the _FW nor the _POL suffix. Section and
// directory nodes match on their directory name; plain files on the base
// name before the extension.
func ownMarker(p *Page) string {
	var name string
	if p.Path == "" || isIndexPage(p.Path) {
		name = path.Base(p.Dir)
	} else {
		name = stem(path.Base(p.Path))
	}
	switch {
	case strings.HasSuffix(name, "_FW"):
		return markerFW
	case strings.HasSuffix(name, "_POL"):
		return markerPOL
	}
	return ""
}

// markPages assigns each page its emoji marker. A page's own _FW/_POL suffix
// wins; otherwise it inherits the marker of its nearest marked ancestor.
func markPages(pages []*Page, inherited string) {
	for _, p := range pages {
		m := ownMarker(p)
		if m == "" {
			m = inherited
		}
		p.Marker = m
		markPages(p.Children, m)
	}
}

func fallbackTitle(f string) string {
	if isIndexPage(f) {
		if d := path.Dir(f); d != "." {
			return path.Base(d)
		}
	}
	return stem(path.Base(f))
}

// sortPages orders siblings: pages with an order field first (by order,
// then path), unordered pages after them (by path).
func sortPages(ps []*Page) {
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		switch {
		case a.Order != nil && b.Order != nil:
			if *a.Order != *b.Order {
				return *a.Order < *b.Order
			}
			return a.Path < b.Path
		case a.Order != nil:
			return true
		case b.Order != nil:
			return false
		default:
			return a.Path < b.Path
		}
	})
}

// Find returns the page with the given path, or nil.
func (t *Tree) Find(p string) *Page {
	return find(t.Pages, p)
}

func find(pages []*Page, p string) *Page {
	for _, pg := range pages {
		if pg.Path == p && p != "" {
			return pg
		}
		if hit := find(pg.Children, p); hit != nil {
			return hit
		}
	}
	return nil
}
