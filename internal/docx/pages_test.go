package docx

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
)

// A PDF as LibreOffice writes it: a page tree and link annotations that
// name their text and the page they lead to.
const probePDF = "%PDF-1.7\n" +
	"1 0 obj\n<</Type/Page/Parent 9 0 R>>\nendobj\n" +
	"4 0 obj\n<</Type/Page/Parent 9 0 R>>\nendobj\n" +
	"7 0 obj\n<</Type/Page/Parent 8 0 R>>\nendobj\n" +
	"8 0 obj\n<</Type/Pages/Parent 9 0 R/Kids[ 7 0 R ]/Count 1>>\nendobj\n" +
	"9 0 obj\n<</Type/Pages/Kids[ 1 0 R 4 0 R\n8 0 R ]/Count 3>>\nendobj\n" +
	// folio-toc-0 -> page 2, folio-toc-1 -> page 3; other links do not count.
	"20 0 obj\n<</Type/Annot/Subtype/Link/Rect[1 2 3 4]/Contents<FEFF0066006F006C0069006F002D0074006F0063002D0030>/Dest[4 0 R/XYZ 70 400 0]>>\nendobj\n" +
	"21 0 obj\n<</Type/Annot/Subtype/Link/Contents<FEFF0066006F006C0069006F002D0074006F0063002D00310020>/Dest[7 0 R/XYZ 70 400 0]>>\nendobj\n" +
	"22 0 obj\n<</Type/Annot/Subtype/Link/Contents<FEFF0048>/A<</S/URI/URI(https://example.org)>>>>\nendobj\n" +
	"23 0 obj\n<</Type/Annot/Subtype/Link/Contents<FEFF0048>/Dest[1 0 R/XYZ 70 400 0]>>\nendobj\n"

func TestProbePages(t *testing.T) {
	got := probePages([]byte(probePDF))
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("pages %v, want 0:2 1:3", got)
	}
	if got := probePages([]byte("%PDF-1.7 nothing")); len(got) != 0 {
		t.Errorf("pages %v from a PDF without links", got)
	}
}

func tocDoc(t *testing.T) string {
	t.Helper()
	d := markdown.Parse([]byte("# One\n\nText.\n\n::: pagebreak\n:::\n\n## Two & more\n\n::: pagebreak\n:::\n\n# Three\n"))
	d.Prepare(markdown.Options{})
	path := filepath.Join(t.TempDir(), "out.docx")
	if err := Write(path, d, Options{TOC: true}); err != nil {
		t.Fatal(err)
	}
	return path
}

// Without an office suite the document stays as it is: the word processor
// that opens it works the contents out.
func TestPaginateWithoutOffice(t *testing.T) {
	path := tocDoc(t)
	before, _ := os.ReadFile(path)
	if filled, err := Paginate(path); filled || err != nil {
		t.Errorf("Paginate = %v, %v", filled, err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("the document changed")
	}
}

// With LibreOffice at hand, the contents get the page of every heading,
// as Word writes them, and nothing asks to be updated any more.
func TestPaginate(t *testing.T) {
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("no LibreOffice")
	}
	defer func(f func() string) { office = f }(office)
	office = func() string { return soffice }

	path := tocDoc(t)
	if filled, err := Paginate(path); !filled || err != nil {
		t.Fatalf("Paginate = %v, %v", filled, err)
	}
	p := readParts(t, path)
	doc := p["word/document.xml"]
	// The contents are on page 1 with the first heading; a page break
	// precedes each of the others.
	for anchor, page := range map[string]string{"one": "1", "two--more": "2", "three": "3"} {
		want := `<w:r><w:tab/></w:r><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> PAGEREF ` +
			anchor + ` \h </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>` + page + `</w:t></w:r>` +
			`<w:r><w:fldChar w:fldCharType="end"/></w:r></w:hyperlink></w:p>`
		if !strings.Contains(doc, want) {
			t.Errorf("no page %s for %s in\n%s", page, anchor, doc)
		}
	}
	if n := len(regexp.MustCompile(`<w:pStyle w:val="TOC\d"/><w:tabs><w:tab w:val="right" w:leader="dot" w:pos="9072"/></w:tabs>`).FindAllString(doc, -1)); n != 3 {
		t.Errorf("%d entries with a dot leader, want 3", n)
	}
	lacks(t, doc, `w:dirty="true"`, "folio-toc-")
	lacks(t, p["word/settings.xml"], "updateFields")

	// A document without contents is left alone.
	plain := filepath.Join(t.TempDir(), "plain.docx")
	d := markdown.Parse([]byte("# One\n"))
	d.Prepare(markdown.Options{})
	if err := Write(plain, d, Options{}); err != nil {
		t.Fatal(err)
	}
	if filled, err := Paginate(plain); filled || err != nil {
		t.Errorf("no contents: Paginate = %v, %v", filled, err)
	}
}
