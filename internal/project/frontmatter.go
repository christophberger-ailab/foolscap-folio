// Package project loads a Quarto project, builds its page tree from the
// order frontmatter, and applies structural edits (reorder, reparent,
// create, delete) back to the files.
package project

import (
	"bytes"
	"fmt"
	"regexp"

	"go.yaml.in/yaml/v4"
)

// FrontMatter holds the frontmatter fields the sorter cares about.
// Order is nil when the file has no order field. Draft is Quarto's own
// `draft: true`, which keeps a page out of what a render publishes. Keys
// holds every other top-level key, among them folio's own `review`,
// `status` and `hide`; read them through Value. Folio and QM are the
// `folio:` and `qm:` mappings those keys sat in before (issue #94), still
// read so that existing pages keep their flags. The values are left untyped
// so that one entry of an unexpected type does not cost the page its title
// and order.
type FrontMatter struct {
	Title string         `yaml:"title"`
	Order *int           `yaml:"order"`
	Draft bool           `yaml:"draft"`
	Folio map[string]any `yaml:"folio"`
	QM    map[string]any `yaml:"qm"`
	Keys  map[string]any `yaml:",inline"`
}

// Flag reports whether the frontmatter sets key to true, see Value.
func (fm FrontMatter) Flag(key string) bool {
	v, _ := fm.Value(key)
	return v == true
}

// Value returns key's top-level value, or, when the top level does not
// mention key, its value under the legacy `folio:` and then `qm:`, and
// whether any of them mentions it.
func (fm FrontMatter) Value(key string) (any, bool) {
	for _, m := range []map[string]any{fm.Keys, fm.Folio, fm.QM} {
		if v, ok := m[key]; ok {
			return v, true
		}
	}
	return nil, false
}

var fmDelim = []byte("---\n")

// splitFrontmatter returns the frontmatter block (delimiters included)
// and the body. The block is empty if src has no frontmatter.
func splitFrontmatter(src []byte) (block, body []byte) {
	if !bytes.HasPrefix(src, fmDelim) {
		return nil, src
	}
	end := bytes.Index(src[len(fmDelim):], fmDelim)
	if end < 0 {
		return nil, src
	}
	n := len(fmDelim) + end + len(fmDelim)
	return src[:n], src[n:]
}

// ParseFrontmatter extracts title, order, draft, and folio's own keys from a
// page's frontmatter.
func ParseFrontmatter(src []byte) FrontMatter {
	var fm FrontMatter
	block, _ := splitFrontmatter(src)
	if block == nil {
		return fm
	}
	yaml.Unmarshal(block[len(fmDelim):len(block)-len(fmDelim)], &fm)
	return fm
}

var orderLine = regexp.MustCompile(`(?m)^order\s*:.*$`)

// SetOrder returns src with the frontmatter order field set to order,
// leaving everything else byte-for-byte intact. A missing order field or
// frontmatter block is created. src is not modified.
func SetOrder(src []byte, order int) []byte {
	line := fmt.Sprintf("order: %d", order)
	block, body := splitFrontmatter(src)
	if block == nil {
		return append([]byte("---\n"+line+"\n---\n\n"), body...)
	}
	from, to := len(block)-len(fmDelim), len(block)-len(fmDelim) // insert before closing ---
	if loc := orderLine.FindIndex(block); loc != nil {
		from, to = loc[0], loc[1]+1 // replace the existing line
	}
	out := make([]byte, 0, len(src)+len(line)+1)
	out = append(out, src[:from]...)
	out = append(out, line...)
	out = append(out, '\n')
	return append(out, src[to:]...)
}
