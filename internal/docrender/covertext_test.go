package docrender

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
)

// minimalDOCX writes a DOCX with one body paragraph and returns its path.
func minimalDOCX(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "book.docx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, s := range map[string]string{
		"word/document.xml":            `<w:document xmlns:w="` + wordNS + `"><w:body><w:p><w:r><w:t>Body</w:t></w:r></w:p><w:sectPr><w:pgSz w:w="11906" w:h="16838"/></w:sectPr></w:body></w:document>`,
		"word/_rels/document.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`,
		"[Content_Types].xml":          `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`,
	} {
		w, _ := zw.Create(name)
		io.WriteString(w, s)
	}
	zw.Close()
	f.Close()
	return path
}

func documentXML(t *testing.T, path string) string {
	t.Helper()
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	r, err := z.Open("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	return string(b)
}

// A cover without an image is a page of its own with the title and the
// subtitle in their fonts and sizes, the title a third down the page.
func TestCoverDOCXTitlePage(t *testing.T) {
	path := minimalDOCX(t)
	err := CoverDOCX(path, "",
		CoverText{Role: "title", Text: "Tom & Jerry", Font: "Aptos Display", Size: 36, Color: "112233"},
		CoverText{Role: "subtitle", Text: "For developers", Size: 20})
	if err != nil {
		t.Fatal(err)
	}
	doc := documentXML(t, path)
	for _, want := range []string{
		`Tom &amp; Jerry`, `w:ascii="Aptos Display"`, `<w:sz w:val="72"/>`, `<w:color w:val="112233"/>`,
		`For developers`, `<w:sz w:val="40"/>`, `w:val="nextPage"`, `<w:pgMar w:top="5613"`, `<w:t>Body</w:t>`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %s in\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "framePr") || strings.Contains(doc, "w:drawing") {
		t.Errorf("a cover without an image has a frame or a picture:\n%s", doc)
	}
	if strings.Index(doc, "Jerry") > strings.Index(doc, "For developers") || strings.Index(doc, "nextPage") > strings.Index(doc, "<w:t>Body") {
		t.Error("title, subtitle, section break and body are out of order")
	}
}

// A text with a box sits where the box is on the cover image, which fits
// the page width here and ends at the bottom of the page.
func TestCoverDOCXTextOnImage(t *testing.T) {
	path := minimalDOCX(t)
	err := CoverDOCX(path, coverPNG(t), CoverText{Role: "title", Text: "Title", Size: 30, Box: &Box{X: 0.1, Y: 0.5, W: 0.5}})
	if err != nil {
		t.Fatal(err)
	}
	doc := documentXML(t, path)
	m := regexp.MustCompile(`<w:framePr w:w="(\d+)" w:hRule="auto" w:hAnchor="page" w:vAnchor="page" w:x="(\d+)" w:y="(\d+)"`).FindStringSubmatch(doc)
	if m == nil {
		t.Fatalf("no frame in\n%s", doc)
	}
	// The 200x100 image is 11906 twips wide and 5953 high, from y = 10885.
	for i, want := range []int{5953, 1191, 13862} {
		if got, _ := strconv.Atoi(m[i+1]); got < want-2 || got > want+2 {
			t.Errorf("frame value %d = %d, want %d", i, got, want)
		}
	}
	if !strings.Contains(doc, `<w:pgMar w:top="0"`) || strings.Index(doc, "framePr") > strings.Index(doc, "w:drawing") {
		t.Errorf("the frame does not come before the picture of a zero-margin section:\n%s", doc)
	}
}

func TestCoverDOCXNeedsContent(t *testing.T) {
	if err := CoverDOCX(minimalDOCX(t), ""); err == nil {
		t.Error("a cover with neither image nor text is accepted")
	}
}

// The PDF's cover page holds the title and the subtitle in their roles'
// fonts and sizes: below each other without boxes, else where the boxes are.
func TestPDFCoverText(t *testing.T) {
	type line struct {
		text, font string
		y          float64
	}
	lay := func(texts ...CoverText) []line {
		w := testWriter(t)
		var out []line
		w.trace = func(l []piece) {
			var s strings.Builder
			for _, p := range l {
				s.WriteString(p.Text)
			}
			out = append(out, line{strings.TrimSpace(s.String()), fontRole(l[0].Run), w.y})
		}
		if err := w.coverText(texts, 0, 0, pageW, pageH); err != nil {
			t.Fatal(err)
		}
		return out
	}
	got := lay(CoverText{Role: "title", Text: "The Book", Size: 36}, CoverText{Role: "subtitle", Text: "For you", Size: 20})
	if len(got) != 2 || got[0].text != "The Book" || got[0].font != "title" || got[1].text != "For you" || got[1].font != "subtitle" {
		t.Fatalf("lines = %+v", got)
	}
	if got[0].y != pageH/3 || got[1].y <= got[0].y+36 {
		t.Errorf("title at %v, subtitle at %v", got[0].y, got[1].y)
	}
	got = lay(CoverText{Role: "subtitle", Text: "one two", Size: 20, Box: &Box{X: 0.5, Y: 0.25, W: 0.1}})
	if len(got) != 2 || got[0].y != pageH/4 {
		t.Errorf("boxed lines = %+v, want two from a quarter down the page", got)
	}
}

// A document's first page is its cover whether that is an image or text,
// and the roles of the cover and of each heading level take fonts and
// sizes of their own.
func TestPDFCoverAndHeadingFonts(t *testing.T) {
	dir := t.TempDir()
	mono := filepath.Join(dir, "mono.ttf")
	os.WriteFile(mono, gomono.TTF, 0o644)
	fs, err := loadFonts(map[string]string{"h2": mono})
	if err != nil {
		t.Fatal(err)
	}
	if fs["h2"].name != "GoMono" || fs["h1"] != fs["heading"] || fs["title"] != fs["heading"] || fs["subtitle"] != fs["regular"] {
		t.Errorf("roles: h2 %q, h1 %q, title %q, subtitle %q", fs["h2"].name, fs["h1"].name, fs["title"].name, fs["subtitle"].name)
	}

	doc := &Document{Blocks: []Node{
		{T: "Header", C: []byte(`[2,["h",[],[]],[{"t":"Str","c":"Heading"}]]`)},
		{T: "Para", C: []byte(`[{"t":"Str","c":"Text"}]`)},
	}}
	out := filepath.Join(dir, "out.pdf")
	err = PDF(out, doc, Options{
		Fonts: map[string]string{"h2": mono}, Sizes: map[string]float64{"h2": 30},
		CoverText: []CoverText{{Role: "title", Text: "The Book", Size: 36}},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), "/BaseFont /GoMono") {
		t.Error("the font of level 2 headings is not embedded")
	}
	if n := strings.Count(string(b), "/Type /Page\n"); n != 2 {
		t.Errorf("%d pages, want the cover and one more", n)
	}

	w := testWriter(t)
	w.sizes = map[string]float64{"h2": 30}
	var first float64
	w.trace = func(l []piece) {
		if first == 0 {
			first = l[0].height
		}
	}
	y := w.y
	bb, _ := blocks(doc.Blocks, 0)
	if err := w.body(bb[:1]); err != nil {
		t.Fatal(err)
	}
	if h := w.y - y; h < 12+30*1.35 {
		t.Errorf("a 30pt heading takes %v", h)
	}
}
