package project

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
)

// Rename records a path that a Move moved on disk. From and To are
// project-relative and name either a page file or a directory; a directory
// took everything below it along. Callers holding a page path from before
// the move — the web UI holds the one open in its editor — follow it to
// where it now lives with Remap.
type Rename struct{ From, To string }

// Remap follows the page path p through the renames a Move performed, and
// returns where that page lives now. A path that did not move comes back
// unchanged, which is also what an empty rename list gives.
func Remap(renames []Rename, p string) string {
	for _, r := range renames {
		switch {
		case p == r.From:
			return r.To
		case strings.HasPrefix(p, r.From+"/"):
			return r.To + strings.TrimPrefix(p, r.From)
		}
	}
	return p
}

// Move makes the page at src the child at position pos of the page at
// parent ("" means the project root). It moves files when the parent
// changes, renumbers the destination siblings sequentially, and closes the
// order gap in the source group. It leaves the pages' headings as they
// are: their levels are the author's, and a print render adjusts them to
// the tree level (issue #96). The receiver is stale afterwards; reload the tree.
//
// It returns the renames it performed on disk, so that a caller can follow
// a page path it was holding to the file's new place; a move within one
// sibling group renames nothing and returns none.
func (t *Tree) Move(src, parent string, pos int) ([]Rename, error) {
	sp := t.Find(src)
	if sp == nil {
		return nil, fmt.Errorf("page %s not found", src)
	}
	if src == parent {
		return nil, fmt.Errorf("cannot move %s into itself", src)
	}
	destDir, destSiblings, err := t.resolveParent(parent)
	if err != nil {
		return nil, err
	}
	leaf := t.Find(parent)
	if !t.isLeaf(leaf) {
		leaf = nil
	}

	newPath := src
	var renames []Rename
	if srcDir := groupDir(src); destDir != srcDir {
		if newPath, renames, err = t.reparent(sp, destDir); err != nil {
			return nil, err
		}
		// Close the order gap left in the source group.
		n := 0
		for _, p := range t.group(srcDir) {
			if p != sp && p.Order != nil {
				n++
				if err := t.setOrder(p.Path, n); err != nil {
					return renames, err
				}
			}
		}
	}

	// Renumber the destination group in its new display order.
	list := make([]*Page, 0, len(destSiblings)+1)
	for _, p := range destSiblings {
		if p != sp {
			list = append(list, p)
		}
	}
	pos = min(max(pos, 0), len(list))
	list = append(list[:pos], append([]*Page{sp}, list[pos:]...)...)
	for i, p := range list {
		file := p.Path
		if p == sp {
			file = newPath
		}
		if err := t.setOrder(file, i+1); err != nil {
			return renames, err
		}
	}
	// A leaf that received its first child becomes a section, last, so
	// that the renumbering above still finds it where it was.
	if leaf != nil {
		r, err := t.toSection(leaf)
		return append(renames, r...), err
	}
	return renames, nil
}

// isLeaf reports whether p is a page in a file of its own (name.md or
// name.qmd) with no children directory beside it yet.
func (t *Tree) isLeaf(p *Page) bool {
	if p == nil || p.Path == "" || p.Dir == "" || isIndexPage(p.Path) {
		return false
	}
	_, err := os.Stat(t.abs(p.Dir))
	return os.IsNotExist(err)
}

// toSection moves the page in a file of its own, name.md, to
// name/index.md, keeping its extension, so that a page that gets its first
// child is a section with its children in its own directory (issue #119).
// The directory must exist; it returns the rename it made.
func (t *Tree) toSection(p *Page) ([]Rename, error) {
	r := Rename{p.Path, p.Dir + "/index" + path.Ext(p.Path)}
	for _, taken := range []string{p.Dir + "/index.md", p.Dir + "/index.qmd"} {
		if _, err := os.Stat(t.abs(taken)); err == nil {
			return nil, fmt.Errorf("%s already exists", taken)
		}
	}
	if err := os.Rename(t.abs(r.From), t.abs(r.To)); err != nil {
		return nil, err
	}
	return []Rename{r}, nil
}

// reparent moves the page's files under destDir. It returns the page's new
// path and the renames it made.
func (t *Tree) reparent(sp *Page, destDir string) (string, []Rename, error) {
	src := sp.Path
	// The unit to move: an index page section moves its directory; anything
	// else moves the file, plus its resource/children directory if present.
	var renames []Rename
	var newPath string
	if base := path.Base(src); isIndexPage(src) {
		dir := path.Dir(src)
		newDir := join(destDir, path.Base(dir))
		renames = append(renames, Rename{dir, newDir})
		newPath = newDir + "/" + base
	} else {
		newPath = join(destDir, base)
		renames = append(renames, Rename{src, newPath})
		if fi, err := os.Stat(t.abs(sp.Dir)); err == nil && fi.IsDir() {
			renames = append(renames, Rename{sp.Dir, join(destDir, path.Base(sp.Dir))})
		}
	}
	for _, r := range renames {
		if destDir == r.From || strings.HasPrefix(destDir+"/", r.From+"/") {
			return "", nil, fmt.Errorf("cannot move %s into its own subtree", src)
		}
		if _, err := os.Stat(t.abs(r.To)); err == nil {
			return "", nil, fmt.Errorf("%s already exists", r.To)
		}
	}
	if destDir != "." {
		if err := os.MkdirAll(t.abs(destDir), 0o755); err != nil {
			return "", nil, err
		}
	}
	for i, r := range renames {
		if err := os.Rename(t.abs(r.From), t.abs(r.To)); err != nil {
			// The renames already done still moved files; report them so
			// the caller can follow a path it holds rather than lose it.
			return "", renames[:i], err
		}
		os.Remove(t.abs(path.Dir(r.From))) // drop the source dir if now empty
	}

	return newPath, renames, nil
}

// CreatePage creates a new page as name.md under parent ("" for root),
// ordered after the last ordered sibling. A parent in a file of its own
// that has no children yet, parent.md, becomes a section first:
// parent/index.md, with the new page beside it (issue #119). It returns
// the new page's path and the renames it made, like Move.
func (t *Tree) CreatePage(parent, name string) (string, []Rename, error) {
	destDir, siblings, err := t.resolveParent(parent)
	if err != nil {
		return "", nil, err
	}
	pp := t.Find(parent)
	leaf := t.isLeaf(pp)
	order := 0
	for _, p := range siblings {
		if p.Order != nil && *p.Order > order {
			order = *p.Order
		}
	}
	rel, err := t.newPageFile(destDir, name, order+1)
	if err != nil || !leaf {
		return rel, nil, err
	}
	renames, err := t.toSection(pp)
	return rel, renames, err
}

// CreatePageAfter creates a new page as name.md in the sibling
// group of the page at after, immediately following it. Like Move, it
// renumbers the whole group sequentially, so previously unordered
// siblings become ordered. It returns the new page's path.
func (t *Tree) CreatePageAfter(after, name string) (string, error) {
	ap := t.Find(after)
	if ap == nil {
		return "", fmt.Errorf("page %s not found", after)
	}
	dir := groupDir(after)
	rel, err := t.newPageFile(dir, name, 0)
	if err != nil {
		return "", err
	}
	n := 0
	for _, p := range t.group(dir) {
		n++
		if err := t.setOrder(p.Path, n); err != nil {
			return "", err
		}
		if p == ap {
			n++
			if err := t.setOrder(rel, n); err != nil {
				return "", err
			}
		}
	}
	return rel, nil
}

// newPageFile writes destDir/name.md from the new-page template and
// returns its path (issue #119). The file name is the typed name in lower
// case, with a dash for each run of spaces (issue #118); a name a folder or
// a page file already has is refused, and so is index, which is a
// section's own page. The page has frontmatter
// with the order and `status: new`, and an explanation div headed by the
// name as typed, on one line, as a level-1 heading, which every page
// starts with wherever it sits in the tree (issue #96). It writes no
// title: a `title:` would override that heading.
func (t *Tree) newPageFile(destDir, name string, order int) (string, error) {
	file := stem(strings.ToLower(strings.Join(strings.Fields(name), "-")))
	if file == "" || file == "index" || strings.ContainsAny(file, `/\`) || strings.HasPrefix(file, "_") || strings.HasPrefix(file, ".") {
		return "", fmt.Errorf("invalid page name %q", name)
	}
	dir := join(destDir, file)
	for _, taken := range []string{dir, dir + ".qmd", dir + ".md"} {
		if _, err := os.Stat(t.abs(taken)); err == nil {
			return "", fmt.Errorf("%s already exists", taken)
		}
	}
	rel := dir + ".md"
	content := fmt.Sprintf("---\norder: %d\nstatus: new\n---\n\n::: explanation\n# %s\n\n:::\n", order, strings.Join(strings.Fields(name), " "))
	if err := os.MkdirAll(filepath.Dir(t.abs(rel)), 0o755); err != nil {
		return "", err
	}
	return rel, os.WriteFile(t.abs(rel), []byte(content), 0o644)
}

// DeletePage moves the page — and, for a section, its whole directory —
// to the system trash, falling back to a _trash directory in the project.
func (t *Tree) DeletePage(p string) error {
	sp := t.Find(p)
	if sp == nil {
		return fmt.Errorf("page %s not found", p)
	}
	var victims []string
	if isIndexPage(p) && path.Dir(p) != "." {
		victims = []string{t.abs(path.Dir(p))}
	} else {
		victims = []string{t.abs(p)}
		if fi, err := os.Stat(t.abs(sp.Dir)); err == nil && fi.IsDir() {
			victims = append(victims, t.abs(sp.Dir))
		}
	}
	return trash(t.Root, victims)
}

// RenamePage gives the page at p a new name in the directory it is in.
//
// A page in a file of its own (name.qmd or name.md) gets the file name
// name, keeping its extension, and its children directory (name/ next to
// the file) is renamed along with it, so the children stay with their
// page. A section page (name/index.qmd or name/index.md), which is what a
// page becomes with its first child, is named by its directory: the directory is renamed,
// taking the section's pages along, and the index page keeps its name. The
// project's own index page has no name to change. The page's frontmatter and headings are left as
// they are: the page keeps its place in the tree.
//
// It returns the renames it performed, like Move; renaming a page to the
// name it has renames nothing.
func (t *Tree) RenamePage(p, name string) ([]Rename, error) {
	sp := t.Find(p)
	if sp == nil {
		return nil, fmt.Errorf("page %s not found", p)
	}
	section := isIndexPage(p)
	if section && path.Dir(p) == "." {
		return nil, fmt.Errorf("the project's own %s cannot be renamed", path.Base(p))
	}
	newName := stem(strings.TrimSpace(name))
	if newName == "" || strings.ContainsAny(newName, `/\`) || strings.HasPrefix(newName, "_") || strings.HasPrefix(newName, ".") {
		return nil, fmt.Errorf("invalid name %q", name)
	}
	if newName == "index" {
		return nil, fmt.Errorf("index.qmd and index.md are reserved for a section's own page")
	}

	var renames []Rename
	var newDir string // the children directory the new name implies
	if section {
		dir := path.Dir(p)
		newDir = join(path.Dir(dir), newName)
		renames = []Rename{{dir, newDir}}
	} else {
		dir := path.Dir(p)
		newDir = join(dir, newName)
		renames = []Rename{{p, newDir + path.Ext(p)}}
		if fi, err := os.Stat(t.abs(sp.Dir)); err == nil && fi.IsDir() {
			renames = append(renames, Rename{sp.Dir, newDir})
		}
	}
	if renames[0].To == renames[0].From {
		return nil, nil
	}
	// Both names a page can go by are checked, whichever the page uses: a
	// directory already there would make a renamed file the section of
	// pages that are not its children, and a file already there would
	// compete with a renamed section for the same name.
	for _, r := range []Rename{{sp.Dir, newDir}, {p, newDir + path.Ext(p)}} {
		// A rename that changes only the case finds the page itself on a
		// case-insensitive file system; that is not a collision.
		if to, err := os.Stat(t.abs(r.To)); err == nil {
			if from, err := os.Stat(t.abs(r.From)); err != nil || !os.SameFile(from, to) {
				return nil, fmt.Errorf("%s already exists", r.To)
			}
		}
	}
	for i, r := range renames {
		if err := os.Rename(t.abs(r.From), t.abs(r.To)); err != nil {
			return renames[:i], err
		}
	}
	return renames, nil
}

// TrashCommands are tried in order to reach the system trash. Set the
// list to nil to force the in-project _trash fallback (tests do).
var TrashCommands = [][]string{{"gio", "trash"}, {"trash-put"}, {"trash"}}

// trash moves the given absolute paths to the system trash if a trash
// command is available, else into <root>/_trash.
func trash(root string, victims []string) error {
	for _, cmd := range TrashCommands {
		if _, err := exec.LookPath(cmd[0]); err != nil {
			continue
		}
		if exec.Command(cmd[0], append(cmd[1:], victims...)...).Run() == nil {
			return nil
		}
	}
	dir := filepath.Join(root, "_trash")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, v := range victims {
		dst := filepath.Join(dir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(v)))
		if err := os.Rename(v, dst); err != nil {
			return err
		}
	}
	return nil
}

// resolveParent resolves a parent page path ("" for root) to the directory
// that holds its children and the current child pages.
func (t *Tree) resolveParent(parent string) (string, []*Page, error) {
	if parent == "" {
		return ".", t.Pages, nil
	}
	pp := t.Find(parent)
	if pp == nil {
		return "", nil, fmt.Errorf("parent %s not found", parent)
	}
	if pp.Dir == "" {
		return "", nil, fmt.Errorf("%s cannot contain child pages", parent)
	}
	return pp.Dir, pp.Children, nil
}

// groupDir returns the directory of the sibling group a page path belongs to.
func groupDir(p string) string {
	d := path.Dir(p)
	if isIndexPage(p) && d != "." {
		return path.Dir(d)
	}
	return d
}

// group returns the pages of the sibling group for the given directory.
func (t *Tree) group(dir string) []*Page {
	if dir == "." {
		return t.Pages
	}
	for _, p := range t.Pages {
		if g := groupIn(p, dir); g != nil {
			return g
		}
	}
	return nil
}

func groupIn(p *Page, dir string) []*Page {
	if p.Dir == dir && len(p.Children) > 0 {
		return p.Children
	}
	for _, c := range p.Children {
		if g := groupIn(c, dir); g != nil {
			return g
		}
	}
	return nil
}

// setOrder writes order into the frontmatter of the page file at rel.
func (t *Tree) setOrder(rel string, order int) error {
	abs := t.abs(rel)
	src, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	out := SetOrder(src, order)
	if string(out) == string(src) {
		return nil
	}
	return os.WriteFile(abs, out, 0o644)
}

func (t *Tree) abs(rel string) string {
	return filepath.Join(t.Root, filepath.FromSlash(rel))
}

// join joins slash paths, treating "." as the root.
func join(dir, name string) string {
	if dir == "." {
		return name
	}
	return dir + "/" + name
}

// isIndexPage reports whether p is an index.qmd or index.md page.
func isIndexPage(p string) bool {
	return stem(path.Base(p)) == "index" && config.IsContentFile(p)
}

// stem returns name without a .qmd or .md extension.
func stem(name string) string {
	if config.IsContentFile(name) {
		return strings.TrimSuffix(name, path.Ext(name))
	}
	return name
}
