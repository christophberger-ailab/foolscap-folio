package docrender

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
)

// A book flattened by folio has its preliminaries, a contents marker, and
// sections in divs of their own (issue #96).
const book = "# Book {#b}\n\nPrelims.\n\n## Foreword\n\n::: folio-toc\n:::\n\n" +
	":::: folio-section\n# Part One\n\nIntro.\n\n## Intro detail\n::::\n\n# Chapter One\n\n## Chapter detail\n"

func bookBlocks(t *testing.T) []Block {
	t.Helper()
	d := markdown.Parse([]byte(book))
	d.Prepare(markdown.Options{})
	bb, err := FromMarkdown(d)
	if err != nil {
		t.Fatal(err)
	}
	return bb
}

func TestSectionAndContentsMarker(t *testing.T) {
	bb := bookBlocks(t)
	prelims, rest := splitTOC(bb)
	if len(prelims) != 3 || text(prelims[1].Runs) != "Prelims." {
		t.Fatalf("prelims = %+v", prelims)
	}
	var got []string
	for _, b := range rest {
		if b.Section {
			got = append(got, text(b.Runs))
		}
	}
	if len(got) != 3 || got[0] != "Part One" || got[2] != "Intro detail" {
		t.Errorf("section blocks = %q", got)
	}
	// The contents list the section's title, not its intro's headings,
	// and nothing of the preliminaries.
	var toc []string
	for _, e := range tocEntries(rest) {
		toc = append(toc, e.title)
	}
	if want := []string{"Part One", "Chapter One", "Chapter detail"}; len(toc) != 3 || toc[0] != want[0] || toc[1] != want[1] || toc[2] != want[2] {
		t.Errorf("contents = %q, want %q", toc, want)
	}
	// Without a marker the contents come first, as before.
	if p, r := splitTOC(rest); p != nil || len(r) != len(rest) {
		t.Errorf("without a marker: prelims %v", p)
	}
}

// A section's headings are set in the section's font roles.
func TestSectionHeadingRoles(t *testing.T) {
	_, rest := splitTOC(bookBlocks(t))
	w := testWriter(t)
	w.sizes = map[string]float64{"section-h1": 30, "h1": 20}
	roles := map[string]string{}
	w.trace = func(line []piece) {
		var s string
		for _, p := range line {
			s += p.Text
		}
		roles[strings.TrimSpace(s)] = line[0].Role
	}
	if err := w.body(rest); err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]string{
		"Part One": "section-h1", "Intro detail": "section-h2", "Chapter One": "h1", "Chapter detail": "h2", "Intro.": "",
	} {
		if roles[text] != want {
			t.Errorf("%q in role %q, want %q (%v)", text, roles[text], want, roles)
		}
	}
}

// The whole layout runs with preliminaries before the contents.
func TestPDFWithPreliminaries(t *testing.T) {
	d := markdown.Parse([]byte(book))
	d.Prepare(markdown.Options{})
	out := filepath.Join(t.TempDir(), "b.pdf")
	if err := PDFMarkdown(context.Background(), out, d, Options{TOC: true, Cover: Cover{Image: coverPNG(t)}}); err != nil {
		t.Fatal(err)
	}
}
