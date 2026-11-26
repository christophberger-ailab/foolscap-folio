package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #161: the preview pane shows a cross-reference without a link
// text as the heading it points to, numbered when a format numbers the
// headings. /xrefs resolves the references of the editor's text, against
// its own headings and those of the project's other pages.
func TestXrefsResolvesThePagesReferences(t *testing.T) {
	srv, root := testServer(t)
	writeConfig(t, root, strings.Replace(testConfig, "pdf: {type: pdf}", "pdf: {type: pdf, numbered_headings: true}", 1))
	text := "---\ntitle: Second\norder: 1\n---\n# Second\n\n## Einführung\n\nSee [](#einfuehrung), [](#third), [](#nope) and [](#chapter2/third:third).\n"
	if err := os.WriteFile(filepath.Join(root, "chapter2/second.qmd"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	// A heading the editor has and the file not yet: numbered once saved.
	text += "\n## Unsaved\n\n[](#unsaved)\n"
	rec := post(t, srv, "/xrefs", url.Values{"path": {"chapter2/second.qmd"}, "text": {text}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var got xrefsView
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	want := map[string]xref{
		"einfuehrung":          {Name: "Einführung", Number: "1.1", ID: "einführung"},
		"third":                {Name: "Third", Number: "2"},
		"chapter2/third:third": {Name: "Third", Number: "2"},
		"unsaved":              {Name: "Unsaved", ID: "unsaved"},
	}
	if len(got.Refs) != len(want) {
		t.Errorf("refs %+v, want %+v", got.Refs, want)
	}
	for k, w := range want {
		if got.Refs[k] != w {
			t.Errorf("ref %q: %+v, want %+v", k, got.Refs[k], w)
		}
	}
	if strings.Join(got.Headings, " ") != "second einführung unsaved" {
		t.Errorf("headings %q", got.Headings)
	}
}

func TestXrefsRefusesAPathOutsideTheProject(t *testing.T) {
	srv, _ := testServer(t)
	if rec := post(t, srv, "/xrefs", url.Values{"path": {"../x.md"}, "text": {"[](#a)"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("status %d", rec.Code)
	}
}

// The pane shows a reference as a render does: "1.2, Name", the name or
// the number alone with {.name} or {.number}, "??key" for no target.
func TestPreviewXrefText(t *testing.T) {
	script := jsFunc(t, "assets/static/preview.js", "xrefText") + jsFunc(t, "assets/static/preview.js", "xrefStyle") + `
const x = {name: "Einführung", number: "1.1"}, n = {name: "Notes"};
for (const [t, k, s] of [[x, "a", ""], [x, "a", "name"], [x, "a", "number"], [n, "b", ""], [n, "b", "number"], [null, "c", ""]]) {
  console.log(xrefText(t, s, k));
}
for (const s of ["{.name} rest", "{.number}", "{#id .x}.", "no block", ""]) {
  const r = xrefStyle(s);
  console.log(r.style + "|" + r.length);
}`
	want := "1.1, Einführung\nEinführung\n1.1\nNotes\nNotes\n??c\nname|7\nnumber|9\n|8\n|0\n|0\n"
	if got := runNode(t, script); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
