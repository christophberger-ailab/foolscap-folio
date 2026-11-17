package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The preview draws a Mermaid diagram the way a render does: it sends the
// diagram's source to /diagram and shows the SVG folio's renderer returns
// (#90).
func TestDiagramDrawsMermaid(t *testing.T) {
	srv, _ := testServer(t)
	draw := func(src string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest("POST", "/diagram", strings.NewReader(src)))
		return rec
	}
	rec := draw("graph TD\n  A[Start] --> B[End]\n")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
		t.Errorf("Content-Type %q", ct)
	}
	if body := rec.Body.String(); !strings.HasPrefix(body, "<svg") || !strings.Contains(body, "Start") {
		t.Errorf("not the diagram:\n%.300s", body)
	}
	if rec := draw("this is not a diagram"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("broken diagram: status %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if rec := draw(strings.Repeat("x", maxDiagram+1)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("huge diagram: status %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

// preview.js swaps a ```{mermaid} block for the diagram from /diagram, and
// only that spelling: ```mermaid stays code, as in a render.
func TestPreviewDrawsDiagrams(t *testing.T) {
	preview, err := assets.ReadFile("assets/static/preview.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`function drawDiagrams(`, `"/diagram"`, `drawDiagrams(el)`, `"mermaid"`} {
		if !assetContains("assets/static/preview.js", string(preview), want) {
			t.Errorf("preview.js does not draw diagrams, missing %q", want)
		}
	}
}
