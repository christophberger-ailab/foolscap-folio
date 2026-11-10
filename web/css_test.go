package web

import (
	"regexp"
	"strings"
	"testing"
)

var (
	cssPunctSpace = regexp.MustCompile(`\s*([{}:;,>+~()])\s*`)
	cssSpace      = regexp.MustCompile(`\s+`)
	cssHexColor   = regexp.MustCompile(`#[0-9A-Fa-f]{3,8}\b`)
)

// normalizeCSS brings a stylesheet, or a piece of one, into a form in which
// layout does not matter: no whitespace around punctuation (`a > b` and
// `a>b` compare equal), any other run of whitespace as a single space, and
// hex colors in lower case (#0758E5 and #0758e5 compare equal).
func normalizeCSS(s string) string {
	s = cssPunctSpace.ReplaceAllString(s, "$1")
	s = cssSpace.ReplaceAllString(s, " ")
	return cssHexColor.ReplaceAllStringFunc(s, strings.ToLower)
}

// cssContains reports whether the stylesheet css holds want, regardless of
// optional whitespace and the case of hex colors.
func cssContains(css, want string) bool {
	return strings.Contains(normalizeCSS(css), normalizeCSS(want))
}

// assetContains is strings.Contains for an asset's source, but for a
// stylesheet it matches as cssContains does.
func assetContains(file, src, want string) bool {
	if strings.HasSuffix(file, ".css") {
		return cssContains(src, want)
	}
	return strings.Contains(src, want)
}

func TestCSSContains(t *testing.T) {
	css := "li.page.draft > .row,\nli.page.draft > .row > .title {\n  color: var(--muted);\n}\n" +
		".callout-note {\n  --callout-color: #0758e5;\n  --callout-header: #DAE6FB;\n}\n"
	for _, want := range []string{
		"li.page.draft>.row",
		"li.page.draft > .row",
		"li.page.draft  >  .row,li.page.draft>.row>.title{color:var(--muted)",
		"--callout-color: #0758E5;\n  --callout-header: #dae6fb;",
		"--callout-color:#0758e5;--callout-header:#dae6fb;",
	} {
		if !cssContains(css, want) {
			t.Errorf("cssContains(%q) = false, want true", want)
		}
	}
	for _, want := range []string{
		"li.page.draft.row",
		"--callout-color: #0758e6;",
		"--callout-header: #dae6fb;\n  --callout-color: #0758e5;",
	} {
		if cssContains(css, want) {
			t.Errorf("cssContains(%q) = true, want false", want)
		}
	}
}
