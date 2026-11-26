package markdown

import (
	"fmt"
	"strings"
	"testing"
)

// xref renders md after numbering it as a book would and resolving its
// references against its own targets, and returns the HTML and the
// warnings.
func xref(t *testing.T, md string, number bool, all ...Target) (string, []string) {
	t.Helper()
	d := Parse([]byte(md))
	d.Prepare(Options{})
	if number {
		NumberHeadings(d, false, false)
		NumberFigures(d, "Figure")
	}
	var warns []string
	warn := func(f string, a ...any) { warns = append(warns, fmt.Sprintf(f, a...)) }
	ts := Targets(d, "p", "", warn)
	Resolver{Page: "p", Local: NewIndex(ts), All: func() Index { return NewIndex(all) },
		Href: func(t Target) string { return "#" + t.ID }, Warn: warn}.Resolve(d)
	h, err := HTML(d)
	if err != nil {
		t.Fatal(err)
	}
	return h, warns
}

func wantHTML(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, got)
		}
	}
}

func TestRefFillsInHeading(t *testing.T) {
	h, w := xref(t, "## Install it\n\nSee [](#install-it) and [this section](#install-it).\n", false)
	wantHTML(t, h, `<a href="#install-it">Install it</a>`, `<a href="#install-it">this section</a>`)
	if len(w) > 0 {
		t.Errorf("warnings: %q", w)
	}
}

func TestRefNumberAndName(t *testing.T) {
	md := "# Pages\n\n## Front matter {#fm}\n\n[](#fm) [](#fm){.name} [](#fm){.number} [](#pages){.number}\n"
	h, w := xref(t, md, true)
	wantHTML(t, h, `<a href="#fm">1.1, Front matter</a>`, `<a href="#fm">Front matter</a>`,
		`<a href="#fm">1.1</a>`, `<a href="#pages">1</a>`)
	if strings.Contains(h, `class="name"`) || strings.Contains(h, `class="number"`) {
		t.Errorf("style class left on the link:\n%s", h)
	}
	if len(w) > 0 {
		t.Errorf("warnings: %q", w)
	}
}

// Without numbering a reference shows the name, and .number falls back
// to it with a warning.
func TestRefUnnumbered(t *testing.T) {
	h, w := xref(t, "## Notes\n\n[](#notes) [](#notes){.number}\n", false)
	wantHTML(t, h, `<a href="#notes">Notes</a> <a href="#notes">Notes</a>`)
	if len(w) != 1 || !strings.Contains(w[0], "no number") {
		t.Errorf("warnings: %q", w)
	}
}

func TestRefUnknown(t *testing.T) {
	h, w := xref(t, "## A\n\n[](#nope) and [text](#gone)\n\n<a id=\"raw\"></a>[raw](#raw)\n", false)
	wantHTML(t, h, `??nope and <a href="#gone">text</a></p>`, `<a href="#raw">raw</a>`)
	if len(w) != 2 {
		t.Errorf("warnings: %q", w)
	}
}

// An anchor of the page that is no target, such as a span's, is linked
// as before.
func TestRefToSpanAnchor(t *testing.T) {
	h, w := xref(t, "[here]{#spot}\n\n[go](#spot)\n", false)
	wantHTML(t, h, `<a href="#spot">go</a>`)
	if len(w) > 0 {
		t.Errorf("warnings: %q", w)
	}
}

func TestRefOutsideOutput(t *testing.T) {
	other := Target{Page: "q", Key: "elsewhere", Name: "Somewhere else", Number: "4.1"}
	h, w := xref(t, "[](#elsewhere) and [that](#elsewhere)\n", false, other)
	wantHTML(t, h, "<p>Somewhere else and that</p>")
	if len(w) != 2 || !strings.Contains(w[0], "not part of this output") {
		t.Errorf("warnings: %q", w)
	}
}

func TestRefFigure(t *testing.T) {
	md := "![My Image](a.png)\n\n![](b.png){#plain}\n\n[](#my-image) [](#my-image){.name} [](#my-image){.number} [see](#plain)\n"
	h, w := xref(t, md, true)
	wantHTML(t, h, `id="my-image"`, "<figcaption>Figure 1: My Image</figcaption>",
		`<a href="#my-image">Figure 1, My Image</a>`, `<a href="#my-image">My Image</a>`,
		`<a href="#my-image">Figure 1</a>`, `<a href="#plain">see</a>`)
	if len(w) > 0 {
		t.Errorf("warnings: %q", w)
	}
}

func TestRefFigureWithoutName(t *testing.T) {
	h, w := xref(t, "![](b.png){#plain}\n\n[](#plain)\n", false)
	wantHTML(t, h, `<a href="#plain">??plain</a>`)
	if len(w) != 1 {
		t.Errorf("warnings: %q", w)
	}
}

// A heading's identifier wins over a figure's.
func TestRefFigureCollides(t *testing.T) {
	h, w := xref(t, "## Architecture\n\n![Architecture](a.png)\n\n[](#architecture)\n", false)
	wantHTML(t, h, `<a href="#architecture">Architecture</a>`)
	if strings.Count(h, `id="architecture"`) != 1 || len(w) != 1 {
		t.Errorf("warnings %q in\n%s", w, h)
	}
}

const book = "::: {.folio-page page=\"t/a\"}\n:::\n\n# A {#bm-t-a}\n\n## Overview\n\n[](#overview)\n\n" +
	"::: {.folio-page page=\"t/b\" pageid=\"bee\"}\n:::\n\n# B {#bm-t-b}\n\n## Overview\n\n[](#overview) [](#a) [](#t/a:overview)\n\n" +
	"::: {.folio-page page=\"t/c\"}\n:::\n\n# C {#bm-t-c}\n\n[](#overview) [](#bee:overview)\n"

// In a book a key is per page: a reference prefers its own page's target,
// may name the page, and is ambiguous otherwise. A page's title is
// addressed by its text.
func TestRefAcrossPages(t *testing.T) {
	h, w := xref(t, book, false)
	wantHTML(t, h,
		`<h2 id="overview">Overview</h2>
<p><a href="#overview">Overview</a></p>`,
		`<h2 id="overview-2">Overview</h2>
<p><a href="#overview-2">Overview</a> <a href="#bm-t-a">A</a> <a href="#overview">Overview</a></p>`,
		`<p><a href="#overview">Overview</a> <a href="#overview-2">Overview</a></p>`)
	if len(w) != 1 || !strings.Contains(w[0], "2 headings") {
		t.Errorf("warnings: %q", w)
	}
}

// A chapter's prefix is its number.
func TestRefPrefixedTitle(t *testing.T) {
	h, _ := xref(t, "# Chapter 3: Pages {#bm-x folio-prefix=\"Chapter 3: \"}\n\n[](#pages) [](#pages){.name}\n", false)
	wantHTML(t, h, `<a href="#bm-x">Chapter 3, Pages</a> <a href="#bm-x">Pages</a>`)
}
