package docrender

import (
	"archive/zip"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
)

// A cover text wraps to its column, is centered in it if asked, and its
// first line's top is at the column's top.
func TestCoverLines(t *testing.T) {
	w := testWriter(t)
	text := CoverText{Text: "A rather long book title that cannot fit on one line", Size: 36, X: 0.1, Y: 0.3, W: 0.8, Center: true}
	lines, bottom, err := w.coverLines(text, "title", 0, 0, pageW, pageH)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) < 2 {
		t.Fatalf("not wrapped: %v", lines)
	}
	for _, l := range lines {
		if l.width > 0.8*pageW+0.01 || math.Abs(l.x+l.width/2-pageW/2) > 0.01 {
			t.Errorf("line %q at %.1f, %.1f wide: not centered in the column", l.text, l.x, l.width)
		}
	}
	if top := lines[0].baseline - w.fonts["title"].ascent*36; math.Abs(top-0.3*pageH) > 0.01 {
		t.Errorf("first line top %.1f, want %.1f", top, 0.3*pageH)
	}
	if bottom <= lines[len(lines)-1].baseline {
		t.Errorf("bottom %.1f above the last baseline", bottom)
	}

	// Left-aligned text starts at the column's left edge, here mapped onto
	// an image that does not start at the page's corner.
	text.Center, text.Text = false, "Short"
	lines, _, err = w.coverLines(text, "title", 100, 50, 200, 400)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].x != 120 || math.Abs(lines[0].baseline-w.fonts["title"].ascent*36-170) > 0.01 {
		t.Errorf("lines %+v", lines)
	}

	// Placed by its baseline, as an SVG text is.
	text.Baseline = true
	if lines, _, _ = w.coverLines(text, "title", 100, 50, 200, 400); math.Abs(lines[0].baseline-170) > 0.01 {
		t.Errorf("baseline %.2f, want 170", lines[0].baseline)
	}
}

func TestPDFCoverTitleAndHeadingFonts(t *testing.T) {
	dir := t.TempDir()
	mono := filepath.Join(dir, "mono.ttf")
	os.WriteFile(mono, gomono.TTF, 0o644)
	doc := &Document{Blocks: []Node{
		{T: "Header", C: []byte(`[2,["h",[],[]],[{"t":"Str","c":"Heading"}]]`)},
		{T: "Para", C: []byte(`[{"t":"Link","c":[["",[],[]],[{"t":"Str","c":"Back"}],["#book",""]]}]`)},
	}}
	out := filepath.Join(dir, "out.pdf")
	cover := Cover{
		Title:    CoverText{Text: "Book", Size: 40, X: 0.1, Y: 0.3, W: 0.8, Center: true},
		Subtitle: CoverText{Text: "For developers", Size: 20, X: 0.1, W: 0.8, Center: true, Below: true},
		Anchor:   "book",
	}
	err := PDF(out, doc, Options{Cover: cover, Title: "Book", Fonts: map[string]string{"h2": mono}, Sizes: map[string]float64{"h2": 30}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), "/BaseFont /GoMono") {
		t.Error("h2 font not embedded")
	}
	if n := strings.Count(string(b), "/Type /Page\n"); n != 2 {
		t.Errorf("%d pages, want cover and body", n)
	}
}

func TestHeadingRoleAndSize(t *testing.T) {
	w := testWriter(t)
	w.sizes = map[string]float64{"h3": 25}
	var traced []piece
	w.trace = func(p []piece) { traced = append(traced, p...) }
	y := w.y
	if err := w.body([]Block{{Level: 3, Runs: []Run{{Text: "Three"}}}}); err != nil {
		t.Fatal(err)
	}
	if len(traced) != 1 || traced[0].Role != "h3" {
		t.Fatalf("traced %+v", traced)
	}
	// 12pt above, then one line of 25pt.
	if got := w.y - y; got < 12+25*1.35 {
		t.Errorf("heading took %.1fpt", got)
	}
}

func TestCoverDOCXTitleWithoutImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.docx")
	writeDOCX(t, path, `<w:p><w:r><w:t>Body</w:t></w:r></w:p><w:sectPr><w:pgSz w:w="11906" w:h="16838"/></w:sectPr>`)
	cover := Cover{
		Title:    CoverText{Text: "Tom & Jerry", Font: "Aptos Display", Size: 36, Color: [3]uint8{0x12, 0x34, 0x56}, X: 0.1, Y: 0.3, W: 0.8, Center: true},
		Subtitle: CoverText{Text: "For developers", Size: 18, X: 0.1, W: 0.8, Center: true, Below: true},
		Anchor:   "book",
	}
	if err := CoverDOCX(path, cover); err != nil {
		t.Fatal(err)
	}
	parts := readDOCX(t, path)
	doc := parts["word/document.xml"]
	frame := `<w:framePr w:w="9525" w:wrap="around" w:hAnchor="page" w:vAnchor="page" w:x="1191" w:y="5051"/>`
	for _, want := range []string{
		frame, `<w:jc w:val="center"/>`, `w:ascii="Aptos Display"`, `<w:color w:val="123456"/>`, `<w:sz w:val="72"/>`,
		`<w:t xml:space="preserve">Tom &amp; Jerry</w:t>`, `<w:sz w:val="36"/>`, `w:name="book"`, `w:val="nextPage"`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %s", want)
		}
	}
	// The subtitle follows in the title's frame: Word joins paragraphs
	// with the same frame properties.
	if strings.Count(doc, frame) != 2 {
		t.Errorf("subtitle not in the title's frame:\n%s", doc)
	}
	if strings.Contains(doc, "<w:drawing>") || parts["word/media/folio-cover.png"] != "" {
		t.Error("image added without one")
	}
	if strings.Index(doc, "Tom &amp;") > strings.Index(doc, "Body") {
		t.Error("cover after the body")
	}
}

func writeDOCX(t *testing.T, path, body string) {
	t.Helper()
	parts := map[string]string{
		"word/document.xml":            `<w:document xmlns:w="` + wordNS + `"><w:body>` + body + `</w:body></w:document>`,
		"word/_rels/document.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`,
		"[Content_Types].xml":          `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`,
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, s := range parts {
		w, _ := zw.Create(name)
		io.WriteString(w, s)
	}
	zw.Close()
	f.Close()
}

func readDOCX(t *testing.T, path string) map[string]string {
	t.Helper()
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	got := map[string]string{}
	for _, f := range z.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		r.Close()
		got[f.Name] = string(b)
	}
	return got
}
