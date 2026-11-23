package project

import "testing"

// A repository cloned on Windows holds its pages with CRLF line endings,
// while the browser's editor hands back LF. Writing the editor's text as it
// stands turns every line into a change for Git (issue #129).
func TestMatchNewlines(t *testing.T) {
	for _, tc := range []struct{ name, text, like, want string }{
		{"crlf file", "a\nb\n", "x\r\ny\r\n", "a\r\nb\r\n"},
		{"crlf file, text already crlf", "a\r\nb\r\n", "x\r\n", "a\r\nb\r\n"},
		{"crlf file, mixed text", "a\r\nb\nc", "x\r\n", "a\r\nb\r\nc"},
		{"lf file", "a\nb\n", "x\ny\n", "a\nb\n"},
		{"lf first line decides", "a\nb\n", "x\ny\r\n", "a\nb\n"},
		{"no line break yet", "a\nb\n", "x", "a\nb\n"},
		{"empty file", "a\n", "", "a\n"},
	} {
		if got := string(MatchNewlines([]byte(tc.text), []byte(tc.like))); got != tc.want {
			t.Errorf("%s: MatchNewlines(%q, %q) = %q, want %q", tc.name, tc.text, tc.like, got, tc.want)
		}
	}
}

func TestParseFrontmatterCRLF(t *testing.T) {
	fm := ParseFrontmatter([]byte("---\r\ntitle: Win\r\norder: 3\r\n---\r\n\r\n# Win\r\n"))
	if fm.Title != "Win" || fm.Order == nil || *fm.Order != 3 {
		t.Errorf("ParseFrontmatter = title %q order %v, want Win 3", fm.Title, fm.Order)
	}
}

// The tree renumbering a CRLF page must neither miss its frontmatter --
// and add a second block above it -- nor mix LF into the file.
func TestSetOrderCRLF(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"replace", "---\r\ntitle: Win\r\norder: 3\r\n---\r\n# Win\r\n", "---\r\ntitle: Win\r\norder: 7\r\n---\r\n# Win\r\n"},
		{"insert", "---\r\ntitle: Win\r\n---\r\n# Win\r\n", "---\r\ntitle: Win\r\norder: 7\r\n---\r\n# Win\r\n"},
		{"create", "# Win\r\n", "---\r\norder: 7\r\n---\r\n\r\n# Win\r\n"},
	} {
		if got := string(SetOrder([]byte(tc.src), 7)); got != tc.want {
			t.Errorf("%s: SetOrder = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestFirstHeadingCRLF(t *testing.T) {
	if got := FirstHeading([]byte("---\r\norder: 1\r\n---\r\n\r\n# Win {.unnumbered}\r\n\r\ntext\r\n")); got != "Win" {
		t.Errorf("FirstHeading = %q, want Win", got)
	}
}
