package markdown

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// AnchorPrefix starts the identifiers the flattener gives the pages'
// title headings. A reference addresses such a title by its text instead.
const AnchorPrefix = "bm-"

// PageClass is the class of the empty div the flattener puts where a page
// starts in a book: `::: {.folio-page page="guide/install" pageid="install"}`.
// Targets and Resolve read the page of what follows from it.
const PageClass = "folio-page"

// Target is something a cross-reference can point to: a heading or a
// figure.
type Target struct {
	// Page is the page's path in the project, as the tree names it
	// ("guide/install"); PageID is the `id:` of its front matter.
	Page, PageID string
	// Key is what a reference names: the identifier the heading or
	// image has on its own page, which a page title takes from its text.
	Key string
	// ID is the identifier in the rendered document, a link's fragment.
	ID string
	// Name is the heading's text or the figure's caption, without its
	// number; Number is "1.2", "Chapter 3" or "Figure 4", "" for none.
	Name, Number string
	Figure       bool
	// Node is the heading or the image.
	Node ast.Node
}

// Targets lists the headings and figures of a document, in document
// order, with the keys references use. The keys are unique per page: a
// heading keeps the identifier it declares, else its text's slug, and a
// second one alike gets -1, -2, … appended, as AssignIDs does. A figure
// is an image with an identifier or with a caption (alt text, alone in
// its paragraph); one without an identifier of its own gets one, made
// from its caption, unique in the document. A heading's key wins over a
// figure's, with a warning.
//
// Page markers (PageClass) set the page of what follows; page and pageID
// are the page before the first one. Call it after AssignIDs, and after
// NumberHeadings and NumberFigures for the numbers.
func Targets(d *Document, page, pageID string, warn func(string, ...any)) []Target {
	type segment struct {
		page, pageID string
		items        []ast.Node
	}
	segs := []*segment{{page: page, pageID: pageID}}
	used := map[string]bool{}
	_ = ast.Walk(d.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		a := AttrsOf(n)
		if a.ID != "" {
			used[a.ID] = true
		}
		if v, ok := n.AttributeString("id"); ok {
			if s, ok := v.(string); ok {
				used[s] = true
			}
		}
		switch x := n.(type) {
		case *Div:
			if x.Attrs.Has(PageClass) {
				p, _ := x.Attrs.Get("page")
				id, _ := x.Attrs.Get("pageid")
				segs = append(segs, &segment{page: p, pageID: id})
			}
		case *ast.Heading:
			s := segs[len(segs)-1]
			s.items = append(s.items, x)
		case *ast.Image:
			if a.ID != "" || figureImage(x.Parent(), d.Source) == x {
				s := segs[len(segs)-1]
				s.items = append(s.items, x)
			}
		}
		return ast.WalkContinue, nil
	})

	var out []Target
	for _, s := range segs {
		seen := map[string]int{}
		unique := func(k string) string {
			if c, dup := seen[k]; dup {
				seen[k] = c + 1
				k = fmt.Sprintf("%s-%d", k, c+1)
				for seen[k] > 0 {
					k += "-1"
				}
			}
			seen[k]++
			return k
		}
		var ts []Target
		for _, n := range s.items {
			h, ok := n.(*ast.Heading)
			if !ok {
				continue
			}
			name := HeadingName(h, d.Source)
			key := HeadingID(h)
			if autoID(h) {
				key = Slug(name)
			}
			ts = append(ts, Target{Page: s.page, PageID: s.pageID, Key: unique(key), ID: HeadingID(h),
				Name: name, Number: HeadingNumber(h), Node: h})
		}
		headings := seen
		figs := map[string]bool{}
		for _, n := range s.items {
			img, ok := n.(*ast.Image)
			if !ok {
				continue
			}
			a := AttrsOf(img)
			name := strings.TrimSpace(PlainText(img, d.Source))
			key := a.ID
			if key == "" {
				key = Slug(name)
			}
			if headings[key] > 0 || figs[key] {
				if warn != nil {
					warn("figure %q on %s: a heading or figure there has the identifier %q already; the figure cannot be referenced", name, pageName(s.page), key)
				}
				key = ""
			}
			figs[key] = key != ""
			if a.ID == "" && key != "" {
				id := key
				for i := 1; used[id]; i++ {
					id = fmt.Sprintf("%s-%d", key, i)
				}
				used[id] = true
				a.ID = id
				setAttrs(img, a)
				img.SetAttributeString("id", id)
			}
			ts = append(ts, Target{Page: s.page, PageID: s.pageID, Key: key, ID: a.ID,
				Name: name, Number: FigureLabel(img), Figure: true, Node: img})
		}
		// Back into document order.
		pos := map[ast.Node]int{}
		for i, n := range s.items {
			pos[n] = i
		}
		ordered := make([]Target, len(s.items))
		for _, t := range ts {
			ordered[pos[t.Node]] = t
		}
		out = append(out, ordered...)
	}
	return out
}

func pageName(p string) string { return fmt.Sprintf("page %q", p) }

// Index finds targets by their key.
type Index map[string][]Target

// NewIndex indexes targets; those without a key are left out.
func NewIndex(ts ...[]Target) Index {
	x := Index{}
	for _, l := range ts {
		for _, t := range l {
			if t.Key != "" {
				x[t.Key] = append(x[t.Key], t)
			}
		}
	}
	return x
}

// Lookup finds the target of a reference made on page: "key", or
// "page:key", where page is a page's `id:` or its path. An unqualified
// key prefers a target on page itself. n is the number of targets the
// reference could mean: 0 for none, more than 1 for an ambiguous one,
// of which t is the first.
func (x Index) Lookup(ref, page string) (t Target, n int) {
	if q, key, ok := strings.Cut(ref, ":"); ok {
		for _, c := range x[key] {
			if q != "" && (q == c.PageID || q == c.Page || q+"/index" == c.Page || q == c.Page+"/index") {
				return c, 1
			}
		}
	}
	cs := x[ref]
	for _, c := range cs {
		if c.Page == page {
			return c, 1
		}
	}
	if len(cs) == 0 {
		return Target{}, 0
	}
	return cs[0], len(cs)
}

// Resolver resolves a document's cross-references: the links whose
// destination is a fragment, `[text](#key)`. A link without text gets the
// target's number and name, "1.2, Installing", or with the class `name`
// or `number` only that.
type Resolver struct {
	// Page is the page of the document, before any page marker.
	Page string
	// Local are the targets the output can link to.
	Local Index
	// All returns the targets of the whole project, so that a reference
	// to one this output leaves out shows its name. It is called once,
	// for the first reference Local does not have. May be nil.
	All func() Index
	// Href is a link's destination for a target of Local.
	Href func(Target) string
	// Warn receives the references that cannot be linked. May be nil.
	Warn func(format string, args ...any)
}

// Resolve resolves the references of d. A fragment that names an element
// of the document that is not a target, such as a span or an HTML
// element's id, is a link as before. A reference to a target that Local
// does not have shows the target's name, or the link's text, without a
// link. A reference to nothing at all stays a link, or shows "??key" if
// it has no text. All of them are warned about, and so is an ambiguous
// one.
func (r Resolver) Resolve(d *Document) {
	ids := map[string]bool{}
	type ref struct {
		link *ast.Link
		page string
	}
	var refs []ref
	page := r.Page
	_ = ast.Walk(d.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if id := AttrsOf(n).ID; id != "" {
			ids[id] = true
		}
		switch x := n.(type) {
		case *ast.RawHTML:
			rawIDs(ids, x.Segments, d.Source)
		case *ast.HTMLBlock:
			rawIDs(ids, x.Lines(), d.Source)
		case *Div:
			if x.Attrs.Has(PageClass) {
				page, _ = x.Attrs.Get("page")
			}
		case *ast.Link:
			if len(x.Destination) > 1 && x.Destination[0] == '#' {
				refs = append(refs, ref{x, page})
			}
		}
		return ast.WalkContinue, nil
	})
	warn := func(format string, args ...any) {
		if r.Warn != nil {
			r.Warn(format, args...)
		}
	}
	var all Index
	for _, x := range refs {
		l := x.link
		key := string(l.Destination[1:])
		if u, err := url.PathUnescape(key); err == nil {
			key = u
		}
		a := AttrsOf(l)
		style := ""
		for _, s := range []string{"name", "number"} {
			if a.Has(s) {
				style = s
			}
		}
		empty := emptyLink(l, d.Source)
		where := fmt.Sprintf("reference %q on %s", "#"+key, pageName(x.page))
		t, n := r.Local.Lookup(key, x.page)
		if n == 0 {
			if ids[key] {
				continue // an anchor of the document, not a target
			}
			if _, k, ok := strings.Cut(key, ":"); ok && ids[k] {
				l.Destination = []byte("#" + k)
				continue
			}
			if all == nil && r.All != nil {
				all = r.All()
			}
			if t, n = all.Lookup(key, x.page); n > 0 {
				warn("%s: %q is not part of this output; it is shown without a link", where, t.Name)
				t.Number = ""
				unlink(l, refText(t, style, key, warn, where), empty)
				continue
			}
			warn("%s: no heading or figure has this identifier", where)
			if empty {
				unlink(l, "??"+key, empty)
			}
			continue
		}
		if n > 1 {
			warn("%s: %d headings or figures have this identifier; it links to the first, on %s. Give the one you mean an identifier of its own, or name its page as in #page:%s",
				where, n, pageName(t.Page), t.Key)
		}
		l.Destination = []byte(r.Href(t))
		dropClasses(l, "name", "number")
		if empty {
			l.RemoveChildren(l)
			l.AppendChild(l, ast.NewString([]byte(refText(t, style, key, warn, where))))
		}
	}
}

var htmlID = regexp.MustCompile(`(?i)\b(?:id|name)\s*=\s*["']([^"']+)["']`)

// rawIDs adds the ids and anchor names of raw HTML to ids.
func rawIDs(ids map[string]bool, segs *text.Segments, src []byte) {
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		for _, m := range htmlID.FindAllSubmatch(seg.Value(src), -1) {
			ids[string(m[1])] = true
		}
	}
}

// refText is what a reference without text shows of t.
func refText(t Target, style, key string, warn func(string, ...any), where string) string {
	name := t.Name
	if name == "" {
		name = t.Number
	}
	switch {
	case style == "number" && t.Number != "":
		return t.Number
	case style == "number":
		warn("%s: the target has no number; its name is shown", where)
	case style == "name" || t.Number == "":
	default:
		return t.Number + ", " + t.Name
	}
	if name == "" {
		warn("%s: the target has neither a name nor a number; give the link a text", where)
		return "??" + key
	}
	return name
}

// emptyLink reports whether a link has no text.
func emptyLink(l *ast.Link, src []byte) bool {
	for c := l.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok || strings.TrimSpace(string(t.Segment.Value(src))) != "" {
			return false
		}
	}
	return true
}

// unlink replaces a link with its children, or with text if empty.
func unlink(l *ast.Link, text string, empty bool) {
	p := l.Parent()
	if empty {
		p.ReplaceChild(p, l, ast.NewString([]byte(text)))
		return
	}
	for c := l.FirstChild(); c != nil; {
		next := c.NextSibling()
		p.InsertBefore(p, l, c)
		c = next
	}
	p.RemoveChild(p, l)
}

// dropClasses takes classes off a node's attributes and its HTML class.
func dropClasses(n ast.Node, classes ...string) {
	a := AttrsOf(n)
	var keep []string
	for _, c := range a.Classes {
		if !slices.Contains(classes, c) {
			keep = append(keep, c)
		}
	}
	if len(keep) == len(a.Classes) {
		return
	}
	a.Classes = keep
	old := n.Attributes()
	n.RemoveAttributes()
	for _, at := range old {
		if string(at.Name) != "class" {
			n.SetAttribute(at.Name, at.Value)
		}
	}
	setAttrs(n, a)
	if len(keep) > 0 {
		n.SetAttributeString("class", strings.Join(keep, " "))
	}
}
