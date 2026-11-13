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

var (
	jsPunctSpace    = regexp.MustCompile(`\s*([{}()\[\];:,.?=+\-*/<>!&|])\s*`)
	jsSpace         = regexp.MustCompile(`\s+`)
	jsTrailingComma = regexp.MustCompile(`,([)\]}])`)
)

// normalizeJS brings a script, or a piece of one, into a form in which what
// a code formatter (Prettier, VSCodium's format-on-save) decides does not
// matter: single and double quotes compare equal, there is no whitespace
// around punctuation, so a call wrapped over several lines matches the same
// call on one, any other run of whitespace is a single space, and a
// trailing comma before a closing bracket is dropped.
func normalizeJS(s string) string {
	s = strings.ReplaceAll(s, "'", `"`)
	s = jsPunctSpace.ReplaceAllString(s, "$1")
	s = jsSpace.ReplaceAllString(s, " ")
	return jsTrailingComma.ReplaceAllString(s, "$1")
}

// jsContains reports whether the script js holds want, regardless of quote
// style, line wrapping and trailing commas.
func jsContains(js, want string) bool {
	return strings.Contains(normalizeJS(js), normalizeJS(want))
}

// assetContains is strings.Contains for an asset's source, but for a
// stylesheet it matches as cssContains does and for a script as
// jsContains does, so that reformatting a file does not fail the tests.
func assetContains(file, src, want string) bool {
	switch {
	case strings.HasSuffix(file, ".css"):
		return cssContains(src, want)
	case strings.HasSuffix(file, ".js"):
		return jsContains(src, want)
	}
	return strings.Contains(src, want)
}

func TestJSContains(t *testing.T) {
	js := "var tip = title ? title + \"\\n\" + src : src;\n" +
		"var body = renderBlocks(\n  convertDivs(stripHeadingAttrs(page.body), lineMap),\n  lineMap,\n  page.offset,\n);\n" +
		"var kinds = { \"<<\": \"start\", \"<<+\": \"reply\" };\n"
	for _, want := range []string{
		"var tip = title ? title + '\\n' + src : src;",
		"renderBlocks(convertDivs(stripHeadingAttrs(page.body), lineMap), lineMap, page.offset)",
		"'<<+': 'reply'",
		"{'<<': 'start', '<<+': 'reply'}",
	} {
		if !jsContains(js, want) {
			t.Errorf("jsContains(%q) = false, want true", want)
		}
	}
	for _, want := range []string{
		"var tip = title + src",
		"renderBlocks(lineMap, page.offset)",
		"'<<+': 'start'",
		"vartip",
	} {
		if jsContains(js, want) {
			t.Errorf("jsContains(%q) = true, want false", want)
		}
	}
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
