package project

import "bytes"

// crlf reports whether src ends its first line with CRLF, the way a page
// checked out on Windows does. The first line break decides, as it does
// for most editors.
func crlf(src []byte) bool {
	i := bytes.IndexByte(src, '\n')
	return i > 0 && src[i-1] == '\r'
}

// newline returns the line ending src uses.
func newline(src []byte) string {
	if crlf(src) {
		return "\r\n"
	}
	return "\n"
}

// MatchNewlines returns text with the line endings of like, the file it is
// about to replace. The browser's editor hands back LF whatever the file
// had, and writing that over a CRLF file makes every line a change to Git
// (issue #129). A file with LF endings, or none yet, gets text as it is.
func MatchNewlines(text, like []byte) []byte {
	if !crlf(like) {
		return text
	}
	lf := bytes.ReplaceAll(text, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))
}
