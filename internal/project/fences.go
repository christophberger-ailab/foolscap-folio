package project

import (
	"bytes"
	"strings"
)

// BalancedFences reports whether all Quarto block fences in src are
// paired. An opening fence is three or more colons followed by text, a
// closing fence is three or more colons alone on a line. Frontmatter,
// fenced code blocks and HTML comments are left out of the check.
func BalancedFences(src []byte) bool {
	_, body := splitFrontmatter(src)
	depth, inCode, inComment := 0, false, false
	for _, b := range bytes.Split(body, []byte("\n")) {
		ln := string(b)
		if !inCode {
			ln, inComment = Uncomment(ln, inComment)
		}
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "```") || strings.HasPrefix(ln, "~~~") {
			inCode = !inCode
			continue
		}
		if inCode || !strings.HasPrefix(ln, ":::") {
			continue
		}
		if strings.TrimLeft(ln, ":") == "" {
			if depth--; depth < 0 {
				return false
			}
		} else {
			depth++
		}
	}
	return depth == 0
}

// Uncomment returns line without the parts that are inside HTML comments,
// given whether a comment is open at its start, and whether one is still
// open at its end.
func Uncomment(line string, open bool) (string, bool) {
	var b strings.Builder
	for {
		if open {
			_, rest, ok := strings.Cut(line, "-->")
			if !ok {
				return b.String(), true
			}
			line = rest
		}
		before, rest, ok := strings.Cut(line, "<!--")
		b.WriteString(before)
		if !ok {
			return b.String(), false
		}
		line, open = rest, true
	}
}
