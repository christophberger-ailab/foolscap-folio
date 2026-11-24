package docrender

import (
	"context"
	"os/exec"
	"path/filepath"
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
