package docrender

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
)

func draftDoc(src string) *markdown.Document {
	d := markdown.Parse([]byte(src))
	d.Prepare(markdown.Options{Draft: true})
	return d
}

// A draft's commented passage is marked, and its thread follows it as a
// note; a todo is a marked TODO.
func TestDraftRuns(t *testing.T) {
	bb, err := FromMarkdown(draftDoc(`Say <!-- << {author="Ann" date="2026-11-01"} Check --><!-- <<+ {author="Bob"} Done -->this<!-- >> --> and ==TODO==.` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	runs := bb[0].Runs
	want := []Run{{Text: "Say "}, {Text: "this", Mark: true}, {Note: "Ann, 2026-11-01: Check\n» Bob: Done"}, {Text: " and "}, {Text: "TODO", Bold: true, Mark: true}, {Text: "."}}
	if len(runs) != len(want) {
		t.Fatalf("runs = %+v", runs)
	}
	for i := range want {
		if runs[i] != want[i] {
			t.Errorf("run %d = %+v, want %+v", i, runs[i], want[i])
		}
	}
}

// The notes stand in a column of their own right of the text, which is
// narrower for it.
func TestDraftNotesInTheMargin(t *testing.T) {
	pdftotext, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not installed")
	}
	path := filepath.Join(t.TempDir(), "out.pdf")
	body := strings.Repeat("word ", 200)
	if err := PDFMarkdown(context.Background(), path, draftDoc("Say <!-- << Margin note here -->this<!-- >> --> "+body+"\n"), Options{}); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(pdftotext, "-layout", path, "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	first := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	if !strings.HasPrefix(first, "Say this word") || !strings.Contains(first, "Margin note") {
		t.Errorf("note not beside its line:\n%s", out)
	}
	plain := filepath.Join(t.TempDir(), "plain.pdf")
	if err := PDFMarkdown(context.Background(), plain, draftDoc("Say "+body+"\n"), Options{}); err != nil {
		t.Fatal(err)
	}
	ref, _ := exec.Command(pdftotext, "-layout", plain, "-").Output()
	if strings.Count(string(out), "\n") <= strings.Count(string(ref), "\n") {
		t.Errorf("text column not narrowed for the notes")
	}
}

// RENDER.DRAFT.7: a bookmarked page's 🔖 is set in the top right corner of
// the page its title is on, above the text, which it does not push down.
func TestDraftBookmarkInTheCorner(t *testing.T) {
	pdftotext, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not installed")
	}
	words := func(src string) string {
		path := filepath.Join(t.TempDir(), "out.pdf")
		if err := PDFMarkdown(context.Background(), path, draftDoc(src), Options{}); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(pdftotext, "-bbox", path, "-").Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	filler := strings.Repeat("word ", 900)
	out := words("# Before\n\n" + filler + "\n\n# Marked\n\n::: {.folio-bookmark}\n🔖\n:::\n\nText\n")
	// pdftotext reads the emoji back as some other character, so the
	// bookmark is the one word set in the top margin.
	top := regexp.MustCompile(`xMin="[\d.]+" yMin="([\d.]+)" xMax="([\d.]+)" yMax="([\d.]+)">[^<]*<`)
	for i, page := range strings.Split(out, "<page ")[1:] {
		var marks []string
		for _, m := range top.FindAllStringSubmatch(page, -1) {
			if yMax, _ := strconv.ParseFloat(m[3], 64); yMax < margin {
				marks = append(marks, m[0])
				if xMax, _ := strconv.ParseFloat(m[2], 64); xMax < pageW-margin-2 || xMax > pageW-margin+2 {
					t.Errorf("🔖 ends at x %.1f, want %.1f", xMax, pageW-margin)
				}
			}
		}
		if want := strings.Contains(page, ">Marked<"); (len(marks) == 1) != want {
			t.Errorf("page %d: marks in the top margin %v, want one: %v", i+1, marks, want)
		}
	}
	plain := words("# Before\n\n" + filler + "\n\n# Marked\n\nText\n")
	at := regexp.MustCompile(`yMin="([\d.]+)"[^>]*>Text<`)
	if a, b := at.FindStringSubmatch(out), at.FindStringSubmatch(plain); a == nil || b == nil || a[1] != b[1] {
		t.Errorf("the bookmark moved the text: %v, %v", a, b)
	}
}
