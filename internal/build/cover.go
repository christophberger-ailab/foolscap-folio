package build

import (
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
// (folio.yml's `cover:`), image, is the whole cover, as it always was. Otherwise
// the cover is the template _brand.yml's meta.coverpage names, the title
// and subtitle set where its elements with the IDs "title" and
// "subtitle" are; without one, it is the title and subtitle centred on a
// blank page. done removes the files the cover was drawn into.
func coverPage(sel config.Selection, b *brand, book Book, image string, log Logf) (c docrender.Cover, done func(), err error) {
	log = log.or()
	done = func() {}
	c.Anchor = book.Anchor
	if image != "" {
		c.Image = image
		return c, done, nil
	}
	title := docrender.CoverText{Text: book.Title, Font: b.font("title", b.typography("headings")),
		Size: b.size("title", titleSize), Color: textColor, X: 0.1, Y: 0.3, W: 0.8, Center: true}
	sub := docrender.CoverText{Text: book.Subtitle, Font: b.font("subtitle", b.typography("base")),
		Size: b.size("subtitle", subtitleSize), Color: textColor, X: 0.1, W: 0.8, Center: true, Below: true}
	c.Title, c.Subtitle = title, sub
	if b == nil || b.Meta.Coverpage == "" {
		return c, done, nil
	}
	tpl, err := coverTemplate(sel, b.Meta.Coverpage)
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

// fill puts a cover text where a template's placeholder was, in its color.
// The text's column is wider than the placeholder, which was drawn for a
// sample text: to the image's edge less the placeholder's margin (at most
// a tenth of the width), on both sides if the placeholder is centred.
func fill(t *docrender.CoverText, s svgimage.Slot) {
	m := math.Max(0, math.Min(0.1, math.Min(s.X, 1-s.X-s.W)))
	t.Below, t.Y = false, s.Y
	t.Center = s.Anchor == "middle" || math.Abs(s.X+s.W/2-0.5) < 0.02
	if t.Center {
		t.X, t.W = m, 1-2*m
	} else {
		t.X, t.W = s.X, math.Max(s.W, 1-m-s.X)
	}
	if s.Fill != nil {
		r, g, b, _ := color.RGBAModel.Convert(s.Fill).RGBA()
		t.Color = [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
	}
}

// coverTemplate returns the cover page template for the selection: of
// <name>_{topic}{audience-template}.svg, <name>_{topic}.svg,
// <name>{audience-template}.svg and <name>.svg in the folder of
// meta.coverpage, the first there is; "" for none.
func coverTemplate(sel config.Selection, coverpage string) (string, error) {
	base, err := foliocore.ProjectPath(sel.Config.Root, strings.TrimPrefix(coverpage, "/"))
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

// typography returns the family of a typography element, or "".
func (b *brand) typography(element string) string {
	if b == nil {
		return ""
	}
	if element == "headings" && b.Typography.Headings.Family != "" {
		return b.Typography.Headings.Family
	}
	return b.Typography.Base.Family
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
