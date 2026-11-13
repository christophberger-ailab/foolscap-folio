package docx

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// This file fills the page numbers into a document's table of contents.
//
// Only a word processor knows on which page a heading ends up. Write
// therefore leaves the contents without page numbers and marks them as
// out of date, for Word to work out on opening. Where LibreOffice is
// installed, Paginate asks it instead: it lays out a copy of the document
// and tells the page of every heading, and the contents get these numbers
// in the form Word writes them itself.

// officeTimeout bounds one run of LibreOffice.
const officeTimeout = 2 * time.Minute

// office returns the path of LibreOffice's soffice program, or "".
var office = findOffice

func findOffice() string {
	if testing.Testing() {
		return "" // tests name the program themselves
	}
	for _, name := range []string{"soffice", "libreoffice"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	var places []string
	switch runtime.GOOS {
	case "darwin":
		places = []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice"}
	case "windows":
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if dir := os.Getenv(env); dir != "" {
				places = append(places, filepath.Join(dir, "LibreOffice", "program", "soffice.exe"))
			}
		}
	}
	for _, p := range places {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

const (
	tocInstr  = `<w:instrText xml:space="preserve"> TOC \o "1-3" \h \z \u </w:instrText>`
	tocEnd    = `<w:p><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>`
	probeText = "folio-toc-"
)

// reTOCEntry matches an entry of the contents as toc writes it: its style,
// what leads up to the end of its link (in the first entry the start of the
// TOC field too), the bookmark it links to, and the link's end.
var reTOCEntry = regexp.MustCompile(`(<w:pStyle w:val="[^"]*"/>)(</w:pPr>(?:<w:r><w:fldChar w:fldCharType="begin"[^>]*/></w:r>` +
	`<w:r><w:instrText[^>]*>[^<]*</w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r>)?` +
	`<w:hyperlink w:anchor="([^"]*)" w:history="1">.*?)(</w:hyperlink></w:p>)`)

// Paginate writes the page of every heading into the table of contents of
// the DOCX at path, which Write made, and reports whether it did. It needs
// LibreOffice; without it, and for a document without contents, the
// document stays as it is.
func Paginate(path string) (bool, error) {
	soffice := office()
	if soffice == "" {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	p, err := readPkg(data)
	if err != nil {
		return false, err
	}
	doc := string(p.get("word/document.xml"))
	start := strings.Index(doc, tocInstr)
	if start < 0 {
		return false, nil
	}
	start = strings.LastIndex(doc[:start], "<w:p>")
	end := strings.Index(doc[max(start, 0):], tocEnd)
	sect := strings.LastIndex(doc, "<w:sectPr")
	if start < 0 || end < 0 || sect < 0 {
		return false, nil
	}
	end += start
	tab := textWidth(doc[sect:])
	var anchors []string
	for _, m := range reTOCEntry.FindAllStringSubmatch(doc[start:end], -1) {
		anchors = append(anchors, m[3])
	}
	if len(anchors) == 0 {
		return false, nil
	}
	// fill gives every entry a dot leader and a page number.
	fill := func(page func(i int) string) string {
		i := -1
		toc := reTOCEntry.ReplaceAllStringFunc(doc[start:end], func(e string) string {
			i++
			m := reTOCEntry.FindStringSubmatch(e)
			return m[1] + fmt.Sprintf(`<w:tabs><w:tab w:val="right" w:leader="dot" w:pos="%d"/></w:tabs>`, tab) + m[2] +
				`<w:r><w:tab/></w:r><w:r><w:fldChar w:fldCharType="begin"/></w:r>` +
				`<w:r><w:instrText xml:space="preserve"> PAGEREF ` + m[3] + ` \h </w:instrText></w:r>` +
				`<w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>` + page(i) + `</w:t></w:r>` +
				`<w:r><w:fldChar w:fldCharType="end"/></w:r>` + m[4]
		})
		return doc[:start] + strings.Replace(toc, ` w:dirty="true"`, "", 1) + doc[end:]
	}

	// The probe is the document as it will be, numbers apart, followed by
	// one link to every heading. In the PDF LibreOffice makes of it, each
	// link names the page it leads to.
	var links strings.Builder
	for i, a := range anchors {
		fmt.Fprintf(&links, `<w:hyperlink w:anchor="%s"><w:r><w:t xml:space="preserve">%s%d </w:t></w:r></w:hyperlink>`, a, probeText, i)
	}
	probe := fill(func(int) string { return "000" })
	at := strings.LastIndex(probe, "<w:sectPr")
	p.set("word/document.xml", []byte(probe[:at]+"<w:p>"+links.String()+"</w:p>"+probe[at:]))
	pages, err := layout(soffice, p)
	if err != nil {
		return false, err
	}
	if len(pages) != len(anchors) {
		return false, fmt.Errorf("LibreOffice told the page of %d of %d headings", len(pages), len(anchors))
	}

	p.set("word/document.xml", []byte(fill(func(i int) string { return strconv.Itoa(pages[i]) })))
	// The numbers are in; nothing needs Word to ask about updating.
	p.set("word/settings.xml", bytes.Replace(p.get("word/settings.xml"), []byte(`<w:updateFields w:val="true"/>`), nil, 1))
	out, err := p.bytes()
	if err != nil {
		return false, err
	}
	return true, writeFile(path, out)
}

// layout has LibreOffice make a PDF of the probe and returns the page of
// every probe link.
func layout(soffice string, probe *pkg) (map[int]int, error) {
	dir, err := os.MkdirTemp("", "folio-pages-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	data, err := probe.bytes()
	if err != nil {
		return nil, err
	}
	src := filepath.Join(dir, "probe.docx")
	if err = os.WriteFile(src, data, 0o600); err != nil {
		return nil, err
	}
	// A profile of its own keeps this run apart from a LibreOffice the
	// user has open, which would otherwise take the request over.
	profile := filepath.Join(dir, "profile")
	if cache, err := os.UserCacheDir(); err == nil {
		profile = filepath.Join(cache, "folio", "libreoffice")
	}
	profile = filepath.ToSlash(profile)
	if !strings.HasPrefix(profile, "/") {
		profile = "/" + profile
	}
	ctx, cancel := context.WithTimeout(context.Background(), officeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, soffice, "-env:UserInstallation=file://"+profile, // #nosec G204 -- the program is LibreOffice as found by office
		"--headless", "--norestore", "--convert-to", "pdf", "--outdir", dir, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("LibreOffice: %w: %s", err, bytes.TrimSpace(out))
	}
	pdf, err := os.ReadFile(filepath.Join(dir, "probe.pdf"))
	if err != nil {
		return nil, fmt.Errorf("LibreOffice wrote no PDF: %w", err)
	}
	return probePages(pdf), nil
}

var (
	rePDFObj      = regexp.MustCompile(`(\d+)\s+0\s+obj`)
	rePDFKids     = regexp.MustCompile(`/Kids\s*\[([^\]]*)\]`)
	rePDFRef      = regexp.MustCompile(`(\d+)\s+0\s+R`)
	rePDFContents = regexp.MustCompile(`/Contents\s*<([0-9A-Fa-f]+)>`)
	rePDFDest     = regexp.MustCompile(`/Dest\s*\[\s*(\d+)\s+0\s+R`)
)

// probePages reads from a PDF the page each probe link leads to, by the
// link's number. It reads what LibreOffice writes — a page tree and link
// annotations with their text, uncompressed — and returns less than was
// asked for, or nothing, from a PDF made differently.
func probePages(pdf []byte) map[int]int {
	kids := map[string][]string{} // page tree node -> its children
	child := map[string]bool{}
	dest := map[int]string{} // probe link -> page object
	for _, obj := range bytes.Split(pdf, []byte("endobj")) {
		id := rePDFObj.FindSubmatch(obj)
		if id == nil {
			continue
		}
		switch {
		case bytes.Contains(obj, []byte("/Type/Pages")):
			if m := rePDFKids.FindSubmatch(obj); m != nil {
				for _, k := range rePDFRef.FindAllSubmatch(m[1], -1) {
					kids[string(id[1])] = append(kids[string(id[1])], string(k[1]))
					child[string(k[1])] = true
				}
			}
		case bytes.Contains(obj, []byte("/Subtype/Link")):
			c, d := rePDFContents.FindSubmatch(obj), rePDFDest.FindSubmatch(obj)
			if c == nil || d == nil {
				continue
			}
			text := strings.TrimSpace(pdfText(string(c[1])))
			if n, err := strconv.Atoi(strings.TrimPrefix(text, probeText)); err == nil && strings.HasPrefix(text, probeText) {
				dest[n] = string(d[1])
			}
		}
	}
	// Number the pages by walking the tree from its root.
	number := map[string]int{}
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		if ks, ok := kids[id]; ok && depth < 64 {
			for _, k := range ks {
				walk(k, depth+1)
			}
			return
		}
		number[id] = len(number) + 1
	}
	for id := range kids {
		if !child[id] {
			walk(id, 0)
		}
	}
	pages := map[int]int{}
	for n, obj := range dest {
		if page, ok := number[obj]; ok {
			pages[n] = page
		}
	}
	return pages
}

// pdfText decodes a PDF hex string in UTF-16 with a byte order mark.
func pdfText(h string) string {
	b, err := hex.DecodeString(h)
	if err != nil || len(b) < 2 || b[0] != 0xFE || b[1] != 0xFF {
		return ""
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 2; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
	}
	return string(utf16.Decode(u))
}
