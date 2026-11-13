package docrender

import (
	"archive/zip"
	"bytes"
	"cmp"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const wordNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// CoverDOCX prepends a cover page in a section of its own: a page-anchored
// PNG, if cover names one, and the texts, each in a frame where its box is
// on the image or else as paragraphs from a third down the page. All
// original package parts, styles, relationships and body section properties
// survive. The output is staged beside the destination and replaced only
// after closing.
func CoverDOCX(path, cover string, texts ...CoverText) error {
	if cover == "" && len(texts) == 0 {
		return fmt.Errorf("a cover needs an image or a text")
	}
	if cover != "" {
		if err := ValidateCover(cover); err != nil {
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
	if len(doc) == 0 || len(files["word/_rels/document.xml.rels"]) == 0 || len(files["[Content_Types].xml"]) == 0 {
		return fmt.Errorf("incomplete DOCX package")
	}
	// The body's own section is read for its page size only. Its margins,
	// headers/footers, page numbering and other settings belong to body.
	bodyStart, _, w, h, err := bodyGeometry(doc)
	if err != nil {
		return err
	}
	// Local namespaces avoid assumptions about the document root's prefixes.
	// The cover section has no header/footer references; the first section
	// consequently has none. The following original section keeps its own.
	// Units: EMU for the picture, twips (635 EMU) for everything else.
	pageCX, pageCY := int64(w)*635, int64(h)*635
	offX, offY, cx, cy := int64(0), int64(0), pageCX, pageCY
	picture, margins := "", fmt.Sprintf(`w:top="%d" w:right="1417" w:bottom="1417" w:left="1417"`, (h+1)/3)
	if cover != "" {
		id, err := embedCover(files, cover)
		if err != nil {
			return err
		}
		c, err := imageSize(cover)
		if err != nil {
			return err
		}
		// Shrink to fit (never crop), preserving the aspect ratio. The image is
		// anchored at the bottom-right page corner, where the brand graphic ends;
		// any leftover space appears at the top or left.
		scale := math.Min(float64(pageCX)/float64(c.Width), float64(pageCY)/float64(c.Height))
		cx = min(int64(math.Round(float64(c.Width)*scale)), pageCX)
		cy = min(int64(math.Round(float64(c.Height)*scale)), pageCY)
		offX, offY = pageCX-cx, pageCY-cy
		margins = `w:top="0" w:right="0" w:bottom="0" w:left="0"`
		picture = fmt.Sprintf(`<w:r><w:drawing><wp:anchor xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" distT="0" distB="0" distL="0" distR="0" simplePos="0" relativeHeight="0" behindDoc="1" locked="0" layoutInCell="1" allowOverlap="1"><wp:simplePos x="0" y="0"/><wp:positionH relativeFrom="page"><wp:posOffset>%d</wp:posOffset></wp:positionH><wp:positionV relativeFrom="page"><wp:posOffset>%d</wp:posOffset></wp:positionV><wp:extent cx="%d" cy="%d"/><wp:effectExtent l="0" t="0" r="0" b="0"/><wp:wrapNone/><wp:docPr id="2147483646" name="folio cover"/><wp:cNvGraphicFramePr/><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr><pic:cNvPr id="0" name="Cover"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:anchor></w:drawing></w:r>`,
			offX, offY, cx, cy, id, cx, cy)
	}
	var fragment strings.Builder
	for _, t := range texts {
		frame := ""
		if b := t.Box; b != nil {
			twips := func(off int64, frac float64, size int64) int64 {
				return int64(math.Round((float64(off) + frac*float64(size)) / 635))
			}
			frame = fmt.Sprintf(`<w:framePr w:w="%d" w:hRule="auto" w:hAnchor="page" w:vAnchor="page" w:x="%d" w:y="%d" w:wrap="around"/>`,
				twips(0, b.W, cx), twips(offX, b.X, cx), twips(offY, b.Y, cy))
		}
		var props, text strings.Builder
		if t.Font != "" {
			var name strings.Builder
			_ = xml.EscapeText(&name, []byte(t.Font))
			fmt.Fprintf(&props, `<w:rFonts w:ascii="%[1]s" w:hAnsi="%[1]s" w:eastAsia="%[1]s" w:cs="%[1]s"/>`, name.String())
		}
		if t.Role == "title" {
			props.WriteString("<w:b/>")
		}
		if _, err := hex.DecodeString(t.Color); err == nil && len(t.Color) == 6 {
			props.WriteString(`<w:color w:val="` + t.Color + `"/>`)
		}
		half := int(math.Round(2 * cmp.Or(t.Size, 24)))
		fmt.Fprintf(&props, `<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, half, half)
		_ = xml.EscapeText(&text, []byte(t.Text))
		fmt.Fprintf(&fragment, `<w:p xmlns:w="%s"><w:pPr>%s<w:spacing w:before="0" w:after="120"/></w:pPr><w:r><w:rPr>%s</w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p>`,
			wordNS, frame, props.String(), text.String())
	}
	fmt.Fprintf(&fragment, `<w:p xmlns:w="%s" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:pPr><w:spacing w:before="0" w:after="0"/><w:sectPr><w:type w:val="nextPage"/><w:pgSz w:w="%d" w:h="%d"/><w:pgMar %s w:header="0" w:footer="0" w:gutter="0"/></w:sectPr></w:pPr>%s</w:p>`,
		wordNS, w, h, margins, picture)
	files["word/document.xml"] = append(append(append([]byte{}, doc[:bodyStart]...), fragment.String()...), doc[bodyStart:]...)
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

// embedCover adds the PNG at cover to the package's parts and returns the
// id of its relationship to the document.
func embedCover(files map[string][]byte, cover string) (string, error) {
	rels := files["word/_rels/document.xml.rels"]
	id := "qmCover"
	for bytes.Contains(rels, []byte(`Id="`+id+`"`)) {
		id += "x"
	}
	media := "folio-cover.png"
	for files["word/media/"+media] != nil {
		media = "x" + media
	}
	image, err := os.ReadFile(cover)
	if err != nil {
		return "", err
	}
	files["word/media/"+media] = image
	rels, err = appendXML(rels, fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/%s"/>`, id, media))
	if err != nil {
		return "", err
	}
	files["word/_rels/document.xml.rels"] = rels
	types, err := appendXML(files["[Content_Types].xml"], fmt.Sprintf(`<Override PartName="/word/media/%s" ContentType="image/png"/>`, media))
	if err != nil {
		return "", err
	}
	files["[Content_Types].xml"] = types
	return id, nil
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
