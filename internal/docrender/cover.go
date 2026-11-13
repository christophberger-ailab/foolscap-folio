package docrender

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/signintech/gopdf"
)

const wordNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// Cover is the page in front of a document: a full-page image, the
// book's title and subtitle, or both.
type Cover struct {
	// Image is a PNG drawn over the page, anchored at its bottom-right
	// corner; "" for none.
	Image           string
	Title, Subtitle CoverText
	// Anchor is the ID of the book's title, which links to it lead to.
	Anchor string
}

func (c Cover) empty() bool { return c.Image == "" && c.Title.Text == "" && c.Subtitle.Text == "" }

// CoverText is a text on the cover. It wraps to a column at X, W wide,
// with the top of its first line at Y, in fractions of the image's width
// and height, or of the page's without an image.
type CoverText struct {
	Text string
	// Font is the family the DOCX names; the PDF sets the text in the
	// font of its role, "title" or "subtitle".
	Font   string
	Size   float64 // pt
	Color  [3]uint8
	X, Y   float64
	W      float64
	Center bool
	// Below puts the subtitle in the title's column, below the title,
	// instead of at its own X, Y and W.
	Below bool
}

// place returns where the cover image goes on a page of w×h: scaled to
// fill the page, cropping, or to fit it, at its bottom-right corner.
func place(c image.Config, w, h float64, fill bool) (x, y, iw, ih float64) {
	scale := math.Min(w/float64(c.Width), h/float64(c.Height))
	if fill {
		scale = math.Max(w/float64(c.Width), h/float64(c.Height))
	}
	iw, ih = float64(c.Width)*scale, float64(c.Height)*scale
	return w - iw, h - ih, iw, ih
}

// cover draws the cover on a page of its own.
func (w *pdfWriter) cover(c Cover) error {
	p := w.p
	p.AddPage()
	w.page++
	x, y, iw, ih := 0.0, 0.0, pageW, pageH
	if c.Image != "" {
		if err := ValidateCover(c.Image); err != nil {
			return err
		}
		cfg, err := imageSize(c.Image)
		if err != nil {
			return err
		}
		// The brand graphic ends at the bottom-right edges, so any overflow
		// from filling the page is cropped at the top/left instead.
		x, y, iw, ih = place(cfg, pageW, pageH, true)
		if err = p.Image(c.Image, x, y, &gopdf.Rect{W: iw, H: ih}); err != nil {
			return err
		}
	}
	if c.Anchor != "" {
		p.SetXY(0, 0)
		p.SetAnchor(c.Anchor)
	}
	bottom := 0.0
	for _, t := range []struct {
		text CoverText
		role string
	}{{c.Title, "title"}, {c.Subtitle, "subtitle"}} {
		if t.text.Text == "" {
			continue
		}
		if t.text.Below {
			t.text.X, t.text.W, t.text.Center = c.Title.X, c.Title.W, c.Title.Center
			t.text.Y = (bottom + t.text.Size*0.6 - y) / ih
		}
		lines, end, err := w.coverLines(t.text, t.role, x, y, iw, ih)
		if err != nil {
			return err
		}
		bottom = end
		p.SetTextColor(t.text.Color[0], t.text.Color[1], t.text.Color[2])
		for _, l := range lines {
			if err = w.draw(Run{Text: l.text, Role: t.role}, t.text.Size, l.x, l.baseline); err != nil {
				return err
			}
		}
	}
	return nil
}

// coverLine is a line of a cover text, placed.
type coverLine struct {
	text               string
	x, baseline, width float64
}

// coverLines wraps t to its column on the image at x, y, iw × ih, and
// returns its lines and the bottom of the last.
func (w *pdfWriter) coverLines(t CoverText, role string, x, y, iw, ih float64) ([]coverLine, float64, error) {
	left, width, top := x+t.X*iw, t.W*iw, y+t.Y*ih
	lh := t.Size * 1.2
	var lines []coverLine
	add := func(s string) error {
		lw, err := w.measure(Run{Text: s, Role: role}, t.Size)
		if err != nil {
			return err
		}
		l := coverLine{text: s, x: left, width: lw, baseline: top + float64(len(lines))*lh + w.fonts[role].ascent*t.Size}
		if t.Center {
			l.x += (width - lw) / 2
		}
		lines = append(lines, l)
		return nil
	}
	cur := ""
	for _, word := range strings.Fields(t.Text) {
		next := strings.TrimSpace(cur + " " + word)
		nw, err := w.measure(Run{Text: next, Role: role}, t.Size)
		if err != nil {
			return nil, 0, err
		}
		if nw > width && cur != "" {
			if err = add(cur); err != nil {
				return nil, 0, err
			}
			next = word
		}
		cur = next
	}
	if cur != "" {
		if err := add(cur); err != nil {
			return nil, 0, err
		}
	}
	return lines, top + float64(len(lines))*lh, nil
}

// CoverDOCX prepends the cover in its own section: the image anchored to
// the page, the title and subtitle in frames positioned on it. All
// original package parts, styles, relationships and body section
// properties survive. The output is staged beside the destination and
// replaced only after closing.
func CoverDOCX(path string, c Cover) error {
	if c.Image != "" {
		if err := ValidateCover(c.Image); err != nil {
			return err
		}
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer z.Close()
	files := map[string][]byte{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			return err
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			return err
		}
		files[f.Name] = b
	}
	doc := files["word/document.xml"]
	rels := files["word/_rels/document.xml.rels"]
	types := files["[Content_Types].xml"]
	if len(doc) == 0 || len(rels) == 0 || len(types) == 0 {
		return fmt.Errorf("incomplete DOCX package")
	}
	// The body's section is read to validate it, but not rewritten. Its
	// margins, headers/footers, page numbering and other settings belong
	// to the body.
	bodyStart, _, w, h, err := bodyGeometry(doc)
	if err != nil {
		return err
	}
	// The image's box on the page, in twips.
	x, y, iw, ih := 0.0, 0.0, float64(w), float64(h)
	drawing := ""
	if c.Image != "" {
		if drawing, x, y, iw, ih, err = coverImage(files, c.Image, w, h); err != nil {
			return err
		}
	}
	var text strings.Builder
	frame := ""
	for _, t := range []CoverText{c.Title, c.Subtitle} {
		if t.Text == "" {
			continue
		}
		before := 0
		if t.Below {
			// Word joins paragraphs with the same frame into one.
			t.X, t.Y, t.W, t.Center = c.Title.X, c.Title.Y, c.Title.W, c.Title.Center
			before = int(math.Round(t.Size * 0.6 * 20))
		}
		frame = fmt.Sprintf(`<w:framePr w:w="%d" w:wrap="around" w:hAnchor="page" w:vAnchor="page" w:x="%d" w:y="%d"/>`,
			int(math.Round(t.W*iw)), int(math.Round(x+t.X*iw)), int(math.Round(y+t.Y*ih)))
		fmt.Fprintf(&text, `<w:p xmlns:w="%s"><w:pPr>%s<w:spacing w:before="%d" w:after="0"/>`, wordNS, frame, before)
		if t.Center {
			text.WriteString(`<w:jc w:val="center"/>`)
		}
		text.WriteString(`</w:pPr><w:r><w:rPr>`)
		if t.Font != "" {
			f := xmlText(t.Font)
			fmt.Fprintf(&text, `<w:rFonts w:ascii="%s" w:hAnsi="%s" w:eastAsia="%s" w:cs="%s"/>`, f, f, f, f)
		}
		sz := int(math.Round(t.Size * 2))
		fmt.Fprintf(&text, `<w:color w:val="%02X%02X%02X"/><w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p>`,
			t.Color[0], t.Color[1], t.Color[2], sz, sz, xmlText(t.Text))
	}
	bookmark := ""
	if c.Anchor != "" {
		// Named as the DOCX writer names a heading's bookmark: the ID's
		// first 40 characters.
		name := []rune(c.Anchor)
		bookmark = fmt.Sprintf(`<w:bookmarkStart w:id="2147483645" w:name="%s"/><w:bookmarkEnd w:id="2147483645"/>`, xmlText(string(name[:min(len(name), 40)])))
	}
	// Local namespaces avoid assumptions about the document root's
	// prefixes. The cover section has no header/footer references; the
	// first section consequently has none. The following original section
	// keeps its own.
	fragment := text.String() + fmt.Sprintf(`<w:p xmlns:w="%s" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:pPr><w:spacing w:before="0" w:after="0"/><w:sectPr><w:type w:val="nextPage"/><w:pgSz w:w="%d" w:h="%d"/><w:pgMar w:top="0" w:right="0" w:bottom="0" w:left="0" w:header="0" w:footer="0" w:gutter="0"/></w:sectPr></w:pPr>%s%s</w:p>`,
		wordNS, w, h, bookmark, drawing)
	files["word/document.xml"] = append(append(append([]byte{}, doc[:bodyStart]...), []byte(fragment)...), doc[bodyStart:]...)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".folio-cover-*.docx")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if info, err := os.Stat(path); err == nil {
		// CreateTemp creates the file private; keep the document's own mode.
		if err := tmp.Chmod(info.Mode().Perm()); err != nil {
			tmp.Close()
			return err
		}
	}
	zw := zip.NewWriter(tmp)
	for _, f := range z.File {
		header := f.FileHeader
		out, err := zw.CreateHeader(&header)
		if err != nil {
			tmp.Close()
			return err
		}
		if _, err = out.Write(files[f.Name]); err != nil {
			tmp.Close()
			return err
		}
		delete(files, f.Name)
	}
	for name, b := range files {
		out, err := zw.Create(name)
		if err != nil {
			tmp.Close()
			return err
		}
		if _, err = out.Write(b); err != nil {
			tmp.Close()
			return err
		}
	}
	if err = zw.Close(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = z.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// coverImage adds the cover image to the package files and returns the
// run that anchors it to a page of w×h twips, and its box on the page.
func coverImage(files map[string][]byte, cover string, w, h int) (run string, x, y, iw, ih float64, err error) {
	rels, types := files["word/_rels/document.xml.rels"], files["[Content_Types].xml"]
	id := "qmCover"
	for bytes.Contains(rels, []byte(`Id="`+id+`"`)) {
		id += "x"
	}
	media := "folio-cover.png"
	for files["word/media/"+media] != nil {
		media = "x" + media
	}
	data, err := os.ReadFile(cover)
	if err != nil {
		return
	}
	files["word/media/"+media] = data
	rel := fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/%s"/>`, id, media)
	if rels, err = appendXML(rels, rel); err != nil {
		return
	}
	files["word/_rels/document.xml.rels"] = rels
	if types, err = appendXML(types, fmt.Sprintf(`<Override PartName="/word/media/%s" ContentType="image/png"/>`, media)); err != nil {
		return
	}
	files["[Content_Types].xml"] = types
	c, err := imageSize(cover)
	if err != nil {
		return
	}
	// Shrink to fit (never crop), preserving the aspect ratio. The image is
	// anchored at the bottom-right page corner, where the brand graphic ends;
	// any leftover space appears at the top or left. Units: EMU (1 twip = 635).
	x, y, iw, ih = place(c, float64(w), float64(h), false)
	pageCX, pageCY := int64(w)*635, int64(h)*635
	cx := min(int64(math.Round(iw*635)), pageCX)
	cy := min(int64(math.Round(ih*635)), pageCY)
	offX, offY := pageCX-cx, pageCY-cy
	run = fmt.Sprintf(`<w:r><w:drawing><wp:anchor xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" distT="0" distB="0" distL="0" distR="0" simplePos="0" relativeHeight="0" behindDoc="1" locked="0" layoutInCell="1" allowOverlap="1"><wp:simplePos x="0" y="0"/><wp:positionH relativeFrom="page"><wp:posOffset>%d</wp:posOffset></wp:positionH><wp:positionV relativeFrom="page"><wp:posOffset>%d</wp:posOffset></wp:positionV><wp:extent cx="%d" cy="%d"/><wp:effectExtent l="0" t="0" r="0" b="0"/><wp:wrapNone/><wp:docPr id="2147483646" name="folio cover"/><wp:cNvGraphicFramePr/><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr><pic:cNvPr id="0" name="Cover"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:anchor></w:drawing></w:r>`,
		offX, offY, cx, cy, id, cx, cy)
	return
}

// xmlText escapes s for XML text and attribute values.
func xmlText(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func bodyGeometry(doc []byte) (start int, section []byte, w, h int, err error) {
	w, h = 11906, 16838
	d := xml.NewDecoder(bytes.NewReader(doc))
	depth := 0
	sectionStart := -1
	for {
		before := int(d.InputOffset())
		token, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			err = e
			return
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Space == wordNS && t.Name.Local == "body" {
				start = int(d.InputOffset())
			}
			if t.Name.Space == wordNS && t.Name.Local == "sectPr" && depth == 3 {
				sectionStart = before
			}
			if sectionStart >= 0 && t.Name.Local == "pgSz" {
				for _, a := range t.Attr {
					if a.Name.Local == "w" {
						w, _ = strconv.Atoi(a.Value)
					}
					if a.Name.Local == "h" {
						h, _ = strconv.Atoi(a.Value)
					}
				}
			}
		case xml.EndElement:
			if sectionStart >= 0 && depth == 3 && t.Name.Local == "sectPr" {
				section = doc[sectionStart:int(d.InputOffset())]
				sectionStart = -1
			}
			depth--
		}
	}
	if start == 0 || w <= 0 || h <= 0 {
		err = fmt.Errorf("invalid DOCX body/page geometry")
	}
	return
}
func appendXML(b []byte, fragment string) ([]byte, error) {
	d := xml.NewDecoder(bytes.NewReader(b))
	depth := 0
	for {
		offset := int(d.InputOffset())
		t, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch t.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				if !strings.HasPrefix(string(b[offset:]), "</") {
					return nil, fmt.Errorf("self-closing package XML root")
				}
				return append(append(append([]byte{}, b[:offset]...), []byte(fragment)...), b[offset:]...), nil
			}
		}
	}
}
