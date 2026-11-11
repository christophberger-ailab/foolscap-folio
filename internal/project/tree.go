package project

import (
	"bytes"
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
	BadFences bool     // the file has unmatched Quarto block fences
	Draft     bool     // the page's frontmatter says `draft: true`
	Marker    string   // emoji marker from a _FW/_POL suffix (own or inherited)
	Flags     []string // status emojis from the folio: frontmatter and the body, see PageFlags
	Hidden    bool     // the page's frontmatter says `folio: {hide: true}`
	Children  []*Page
}

// Emoji markers for the _FW and _POL name suffixes.
const (
	markerFW  = "\U0001F692" // 🚒
	markerPOL = "\U0001F693" // 🚔
)

// pageFlags lists the status flags a page can carry, in the order the tree
// shows them. A flag is set by `folio: {<Key>: true}` in the frontmatter, or
// by In reporting true for the page's file when In is not nil. A new flag
// is one more entry here.
var pageFlags = []struct {
	Key   string
	Emoji string
	In    func(src []byte) bool
}{
	{Key: "todo", Emoji: "\U0001F6A7", In: hasTodo},       // 🚧
	{Key: "review", Emoji: "\U0001F440"},                  // 👀
	{Key: "comment", Emoji: "\U0001F4AC", In: hasComment}, // 💬
}

// PageFlags returns the emojis of the status flags src carries, each at
// most once, in the order of pageFlags.
func PageFlags(src []byte) []string {
	return pageFlagsOf(src, ParseFrontmatter(src))
}

func pageFlagsOf(src []byte, fm FrontMatter) []string {
	var flags []string
	for _, f := range pageFlags {
		if fm.Flag(f.Key) || f.In != nil && f.In(src) {
			flags = append(flags, f.Emoji)
		}
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

// IsHidden reports whether src's frontmatter says `folio: {hide: true}`, which
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

// Tree is the page tree of a Quarto project.
type Tree struct {
	Root  string // absolute path of the project
	Pages []*Page
}

// Load scans root for .qmd and .md files and builds the page tree.
//
// index.qmd or index.md inside a directory represents that directory's
// section and is ordered among the pages one level up, as is a name.qmd or
// name.md file next to a name/ directory. Directories starting with "_" or "." are ignored.
func Load(root string) (*Tree, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")) {
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
			Path:      f,
			Title:     fm.Title,
			Order:     fm.Order,
			Draft:     fm.Draft,
			Flags:     pageFlagsOf(src, fm),
			Hidden:    isHidden(fm),
			BadFences: !BalancedFences(src),
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
	markPages(t.Pages, "")
	hideEmptyDirs(t.Pages)
	return t, nil
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

// Profiles returns the profile names of the _quarto-<name>.yml files in
// root that configure a book (contain a top-level book key).
func Profiles(root string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(root, "_quarto-*.yml"))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, m := range matches {
		src, err := os.ReadFile(m)
		if err != nil {
			return nil, err
		}
		if !hasBook(src) {
			continue
		}
		names = append(names, strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "_quarto-"), ".yml"))
	}
	sort.Strings(names)
	return names, nil
}
