package build

import (
	"cmp"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/docrender"
	"github.com/christophberger-ailab/foolscap-folio/internal/foliocore"
	"github.com/christophberger-ailab/foolscap-folio/internal/svgimage"
)

// Book is what a document's cover shows: the title and subtitle of the
// topic's index page, their variables resolved, and the ID of the title,
// which in-book links to the index page lead to.
type Book struct {
	Title, Subtitle, Anchor string
}

// The cover's default font sizes in pt, and text color.
const titleSize, subtitleSize = 36, 20

var textColor = [3]uint8{30, 35, 42}

// coverWidth is the width in pixels a cover template is drawn at: A4 at
// 200 dpi.
const coverWidth = 1654

// coverPage returns the cover of a document. A topic's own cover image
// (folio.yml's `cover:`), image, is the whole cover, as it always was.
// Otherwise the cover is the template _brand.yml's meta.coverpage names,
// the title and subtitle put in place of its elements with the IDs "title"
// and "subtitle"; without one, it is the title and subtitle centred on a
// blank page. done removes the files the cover was drawn into.
func coverPage(sel config.Selection, b *brand, book Book, image string, log Logf) (c docrender.Cover, done func(), err error) {
	log = log.or()
	done = func() {}
	c.Anchor = book.Anchor
	if image != "" {
		c.Image = image
		return c, done, nil
	}
	look := b.style()
	c.Title = docrender.CoverText{Text: book.Title, Font: cmp.Or(b.font("title"), look.HeadingFont, look.Font),
		Size: b.size("title", titleSize), Color: rgb(cmp.Or(look.HeadingColor, look.Color)), X: 0.1, Y: 0.3, W: 0.8, Center: true}
	c.Subtitle = docrender.CoverText{Text: book.Subtitle, Font: cmp.Or(b.font("subtitle"), look.Font),
		Size: b.size("subtitle", subtitleSize), Color: rgb(look.Color), X: 0.1, W: 0.8, Center: true, Below: true}
	if b == nil || b.Meta.Coverpage == "" {
		return c, done, nil
	}
	tpl, err := coverTemplate(sel, b.dir, b.Meta.Coverpage)
	if err != nil || tpl == "" {
		if err == nil {
			log("warning: topic %s: %s: cover page template %s not found; the cover shows the title alone",
				sel.Topic.Name, BrandFile, b.Meta.Coverpage)
		}
		return c, done, err
	}
	data, err := os.ReadFile(tpl)
	if err != nil {
		return c, done, err
	}
	rest, slots, err := svgimage.Cut(data, "title", "subtitle")
	if err != nil {
		return c, done, fmt.Errorf("cover page template %s: %w", rel(sel.Config.Root, tpl), err)
	}
	for _, t := range []struct {
		id   string
		text *docrender.CoverText
	}{{"title", &c.Title}, {"subtitle", &c.Subtitle}} {
		if b.Meta.Fonts[t.id].Size == "" {
			t.text.Size = 0 // the placeholder's, if it has one
		}
		if b.font(t.id) == "" {
			t.text.Font = ""
		}
		slot, ok := slots[t.id]
		if !ok {
			if t.text.Text != "" {
				log("warning: topic %s: cover page template %s has no element with the id %q, so the cover shows no %s",
					sel.Topic.Name, rel(sel.Config.Root, tpl), t.id, t.id)
			}
			t.text.Text = ""
			continue
		}
		fill(t.text, slot)
		if t.text.Size == 0 {
			t.text.Size = map[string]float64{"title": titleSize, "subtitle": subtitleSize}[t.id]
		}
		t.text.Font = cmp.Or(t.text.Font, look.HeadingFont, look.Font)
	}
	w, _, err := svgimage.Size(rest)
	if err != nil {
		return c, done, err
	}
	png, err := svgimage.PNG(rest, coverWidth/w)
	if err != nil {
		return c, done, fmt.Errorf("cover page template %s: %w", rel(sel.Config.Root, tpl), err)
	}
	if c.Image, err = tempPNG(png); err != nil {
		return c, done, err
	}
	return c, func() { os.Remove(c.Image) }, nil
}

// fill puts a cover text in place of a template's placeholder: in its
// color, and for a text element in its font and size unless the brand
// sets them. The text's column is wider than the placeholder, which was
// drawn for a sample text: to the image's edge less the placeholder's
// margin (at most a tenth of the width), on both sides if the placeholder
// is centred.
func fill(t *docrender.CoverText, s svgimage.Slot) {
	t.Below, t.Y = false, s.Y
	if s.Fill != nil {
		r, g, b, _ := color.RGBAModel.Convert(s.Fill).RGBA()
		t.Color = [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
	}
	if s.Text {
		t.Baseline, t.Y = true, s.Baseline
		if t.Size == 0 && s.Size > 0 {
			t.Size = s.Size * a4Height
		}
		t.Font = cmp.Or(t.Font, fontName(s.Family))
	}
	m := math.Max(0, math.Min(0.1, math.Min(s.X, 1-s.X-s.W)))
	t.Center = s.Anchor == "middle" || !s.Text && math.Abs(s.X+s.W/2-0.5) < 0.02
	switch {
	case t.Center && s.Text:
		half := math.Min(s.X, 1-s.X) - m
		t.X, t.W = s.X-half, 2*half
	case t.Center:
		t.X, t.W = m, 1-2*m
	default:
		t.X, t.W = s.X, math.Max(s.W, 1-m-s.X)
	}
}

// a4Height is the height of the PDF's page in pt, which a template's font
// sizes are scaled to.
const a4Height = 841.89

// rgb reads RRGGBB, or is the default text color.
func rgb(hex string) [3]uint8 {
	var c [3]uint8
	if _, err := fmt.Sscanf(hex, "%02X%02X%02X", &c[0], &c[1], &c[2]); err != nil || len(hex) != 6 {
		return textColor
	}
	return c
}

// coverTemplate returns the cover page template for the selection: of
// <name>_{topic}{audience-template}.svg, <name>_{topic}.svg,
// <name>{audience-template}.svg and <name>.svg in the folder of
// meta.coverpage, the first there is; "" for none. The path is relative to
// dir, the brand file's: the project root, or the user's config directory.
func coverTemplate(sel config.Selection, dir, coverpage string) (string, error) {
	base, err := foliocore.ProjectPath(dir, strings.TrimPrefix(coverpage, "/"))
	if err != nil {
		return "", fmt.Errorf("%s: meta: coverpage: %w", BrandFile, err)
	}
	if !svgimage.IsSVG(base) {
		return "", fmt.Errorf("%s: meta: coverpage: %s is not an SVG image", BrandFile, coverpage)
	}
	base = strings.TrimSuffix(base, filepath.Ext(base))
	audience, topic := sel.Vars()["audience-template"], "_"+sel.Topic.Name
	for _, suffix := range []string{topic + audience, topic, audience, ""} {
		p := base + suffix + ".svg"
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", nil
}

// tempPNG writes a PNG to a temporary file. The caller removes it.
func tempPNG(data []byte) (string, error) {
	f, err := os.CreateTemp("", "folio-cover-*.png")
	if err != nil {
		return "", err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Close()
	} else {
		f.Close()
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
