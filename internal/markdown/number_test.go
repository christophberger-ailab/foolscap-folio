package markdown

import (
	"slices"
	"testing"

	"github.com/yuin/goldmark/ast"
)

func headingTexts(d *Document) []string {
	var out []string
	_ = ast.Walk(d.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering {
			out = append(out, PlainText(h, d.Source))
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return out
}

func numbered(md string, sectionPrefixed, chapterPrefixed bool) []string {
	d := Parse([]byte(md))
	d.Prepare(Options{})
	NumberHeadings(d, sectionPrefixed, chapterPrefixed)
	return headingTexts(d)
}

const numberBook = "# Book {.unnumbered}\n\n## Foreword\n\n::: folio-toc\n:::\n\n" +
	":::: folio-section\n# Part One\n\n## Intro\n::::\n\n# Pages\n\n## Front matter\n\n### Fields\n\n## Callouts {-}\n\n## Levels\n\n" +
	":::: folio-section\n# Part Two\n::::\n\n# Images\n\n## Paths\n"

// Chapters are numbered through the book and their headings below them;
// the preliminaries, the sections and unnumbered headings are not
// (issue #124).
func TestNumberHeadings(t *testing.T) {
	want := []string{"Book", "Foreword", "Part One", "Intro", "1 Pages", "1.1 Front matter", "1.1.1 Fields",
		"Callouts", "1.2 Levels", "Part Two", "2 Images", "2.1 Paths"}
	if got := numbered(numberBook, false, false); !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	// A chapter prefix numbers the chapter's title already.
	want[4], want[10] = "Pages", "Images"
	if got := numbered(numberBook, true, true); !slices.Equal(got, want) {
		t.Errorf("with prefixes:\ngot  %q\nwant %q", got, want)
	}
}

// A book without chapters numbers its sections instead.
func TestNumberHeadingsOfSections(t *testing.T) {
	md := ":::: folio-section\n# First\n\n## Setup\n::::\n\n:::: folio-section\n# Second\n\n## Use\n::::\n"
	if got, want := numbered(md, false, false), []string{"1 First", "1.1 Setup", "2 Second", "2.1 Use"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := numbered(md, true, false), []string{"First", "1.1 Setup", "Second", "2.1 Use"}; !slices.Equal(got, want) {
		t.Errorf("with a section prefix: got %q, want %q", got, want)
	}
	// Without a book's structure, every heading counts.
	if got, want := numbered("# A\n\n## B\n\n# C\n", false, false), []string{"1 A", "1.1 B", "2 C"}; !slices.Equal(got, want) {
		t.Errorf("plain: got %q, want %q", got, want)
	}
}
