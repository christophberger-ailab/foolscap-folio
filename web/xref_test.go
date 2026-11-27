package web

import (
	"encoding/json"
	"fmt"
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
// its own headings and those of the project's other pages. Issue #178: a
// reference to another page names that page's file, so that a click on it
// opens the page; every target names its key, which the preview puts on
// the heading or figure it scrolls to.
func TestXrefsResolvesThePagesReferences(t *testing.T) {
	srv, root := testServer(t)
	writeConfig(t, root, strings.Replace(testConfig, "pdf: {type: pdf}", "pdf: {type: pdf, numbered_headings: true, numbered_figures: true}", 1))
	text := "---\ntitle: Second\norder: 1\n---\n# Second\n\n## Einführung\n\n![A map](map.png)\n\nSee [](#einfuehrung), [](#third), [](#nope), [](#chapter2/third:third) and [](#fig-overview).\n"
	files := map[string]string{
		"chapter2/second.qmd": text,
		"chapter2/third.qmd":  "---\ntitle: Third\norder: 2\n---\n# Third\n\n![Overview](o.png){#fig-overview}\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A heading the editor has and the file not yet: numbered once saved.
	text += "\n## Unsaved\n\n[](#unsaved)\n"
	got := postXrefs(t, srv, "chapter2/second.qmd", text)
	third := "chapter2/third.qmd"
	want := map[string]xref{
		"einfuehrung":          {Name: "Einführung", Number: "1.1", Key: "einführung"},
		"third":                {Name: "Third", Number: "2", Key: "third", Path: third},
		"chapter2/third:third": {Name: "Third", Number: "2", Key: "third", Path: third},
		"fig-overview":         {Name: "Overview", Number: "Figure 2", Key: "fig-overview", Path: third},
		"unsaved":              {Name: "Unsaved", Key: "unsaved"},
	}
	if len(got.Refs) != len(want) {
		t.Errorf("refs %+v, want %+v", got.Refs, want)
	}
	for k, w := range want {
		if got.Refs[k] != w {
			t.Errorf("ref %q: %+v, want %+v", k, got.Refs[k], w)
		}
	}
	if s := fmt.Sprint(got.Headings); s != "[{second 1} {einführung 1.1} {unsaved }]" {
		t.Errorf("headings %s", s)
	}
	if s := fmt.Sprint(got.Figures); s != "[{a-map Figure 1}]" {
		t.Errorf("figures %s", s)
	}
}

// UI.3-4-10, issue #178: a page without a single reference still gets its headings
// and figures numbered, and the keys a reference from elsewhere scrolls
// to; with nothing numbered, the numbers are left out.
func TestXrefsNumbersAPageWithoutReferences(t *testing.T) {
	srv, root := testServer(t)
	text := "---\ntitle: Third\norder: 2\n---\n# Third\n\n![Overview](o.png)\n"
	if err := os.WriteFile(filepath.Join(root, "chapter2/third.qmd"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got := postXrefs(t, srv, "chapter2/third.qmd", text)
	if s := fmt.Sprint(got.Headings, got.Figures); s != "[{third }] [{overview }]" {
		t.Errorf("unnumbered: %s", s)
	}
	writeConfig(t, root, strings.Replace(testConfig, "docx: {type: docx}", "docx: {type: docx, numbered_headings: true, numbered_figures: true}", 1))
	got = postXrefs(t, srv, "chapter2/third.qmd", text)
	if s := fmt.Sprint(got.Headings, got.Figures); s != "[{third 2}] [{overview Figure 1}]" {
		t.Errorf("numbered: %s", s)
	}
}

func postXrefs(t *testing.T, srv http.Handler, path, text string) xrefsView {
	t.Helper()
	rec := post(t, srv, "/xrefs", url.Values{"path": {path}, "text": {text}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var v xrefsView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	return v
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

// UI.3-4-12, issue #178: the preview keeps a history of the pages opened and the
// references followed, which ← and → go back and forward through. A page
// opened anew drops what lay ahead; a step arrives at its own entry; a
// reload of the page shown adds nothing.
func TestPreviewPageHistory(t *testing.T) {
	script := jsFunc(t, "assets/static/app.js", "historyPush") + jsFunc(t, "assets/static/app.js", "historyArrive") +
		jsFunc(t, "assets/static/app.js", "historyStep") + `
const h = {entries: [], at: -1, going: null};
const show = (r) => console.log(h.entries.map(e => e.path + (e.key ? "#" + e.key : "")).join(" ") + " @" + h.at + (r ? " place " + r.path + "#" + r.key : ""));
show(historyArrive(h, "a.qmd"));
show(historyArrive(h, "a.qmd"));
show(historyArrive(h, "b.qmd"));
h.going = {entry: {path: "c.qmd", key: "fig-x", scroll: null}, at: -1};
show(historyArrive(h, "c.qmd"));
console.log(historyStep(h, 1), historyStep(h, -1), historyStep({entries: [], at: -1}, -1));
h.going = {entry: h.entries[0], at: 0};
show(historyArrive(h, "a.qmd"));
h.going = {entry: {path: "x.qmd", key: "", scroll: null}, at: -1}; // the open was cancelled
show(historyArrive(h, "d.qmd"));
`
	want := "a.qmd @0\na.qmd @0\na.qmd b.qmd @1\na.qmd b.qmd c.qmd#fig-x @2 place c.qmd#fig-x\nnull 1 null\na.qmd b.qmd c.qmd#fig-x @0 place a.qmd#\na.qmd d.qmd @1\n"
	if got := runNode(t, script); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The back and forward buttons sit below the preview with the flip
// buttons, and start out of use: there is no history yet.
func TestContentServesHistoryButtons(t *testing.T) {
	srv, _ := testServer(t)
	body := get(t, srv, "/content?path=index.qmd").Body.String()
	for _, button := range []string{`id="page-back"`, `id="page-forward"`} {
		at := strings.Index(body, button)
		if at < 0 {
			t.Errorf("editor pane has no %s button", button)
			continue
		}
		if rest := body[at:]; !strings.Contains(rest[:min(len(rest), 80)], "disabled") {
			t.Errorf("%s does not start out of use", button)
		}
	}
}
