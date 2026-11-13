package build

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/docrender"
	"github.com/christophberger-ailab/foolscap-folio/internal/foliocore"
	"github.com/christophberger-ailab/foolscap-folio/internal/svgimage"
)

// The cover's title and subtitle without a size in the brand, in points,
// and the width in pixels a cover image is drawn at: A4 at 200 dpi.
const (
	titleSize, subtitleSize = 36.0, 20.0
	coverPixels             = 1654
)

// coverFile finds the cover image for a selection's variables beside the
// one the brand names, path, which is relative to the project root: the
// file for the topic and the audience (<name>_<topic><audience-template>),
// for the topic, for the audience, and at last the file itself. It is ""
// when none of them is there.
func coverFile(root, path string, vars map[string]string) (string, error) {
	if !svgimage.IsSVG(path) {
		return "", fmt.Errorf("%s: meta: coverpage: %s is not an SVG image", BrandFile, path)
	}
	base, err := foliocore.ProjectPath(root, strings.TrimLeft(path, "/"))
	if err != nil {
		return "", fmt.Errorf("%s: meta: coverpage: %w", BrandFile, err)
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	topic, audience := "_"+vars["topic"], vars["audience-template"]
	suffixes := []string{topic, ""}
	if audience != "" {
		suffixes = []string{topic + audience, topic, audience, ""}
	}
	for _, s := range suffixes {
		file := stem + s + ext
		// A variable does not lead to another folder.
		if filepath.Dir(file) != filepath.Dir(base) {
			continue
		}
		if fi, err := os.Stat(file); err == nil && !fi.IsDir() {
			return file, nil
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return "", nil
}

// coverPage makes the cover page of a document whose topic has no cover of
// its own: the title and the subtitle in the brand's fonts, and behind
// them the brand's cover image, as a temporary PNG the caller removes.
//
// On an image, a text goes where the element with the id "title" or
// "subtitle" is, in that element's fill color; the element marks the
// place and is not drawn. A text the image has no element for is left
// out. Without an image the texts stand below each other.
func coverPage(sel config.Selection, b *brand, look brandLook, title, subtitle string, log Logf) (string, []docrender.CoverText, error) {
	texts := []docrender.CoverText{{
		Role: "title", Text: title, Font: cmp.Or(look.Fonts["title"], look.HeadingFont, look.Font),
		Size: cmp.Or(look.Sizes["title"], titleSize), Color: look.HeadingColor,
	}}
	if subtitle != "" {
		texts = append(texts, docrender.CoverText{
			Role: "subtitle", Text: subtitle, Font: cmp.Or(look.Fonts["subtitle"], look.Font),
			Size: cmp.Or(look.Sizes["subtitle"], subtitleSize), Color: look.HeadingColor,
		})
	}
	if b == nil || b.Meta.Coverpage == "" {
		return "", texts, nil
	}
	root := sel.Config.Root
	file, err := coverFile(root, b.Meta.Coverpage, sel.Vars())
	if err != nil {
		return "", nil, err
	}
	if file == "" {
		log("warning: %s: meta: coverpage: %s not found; the cover has no image", BrandFile, b.Meta.Coverpage)
		return "", texts, nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", nil, err
	}
	slots, picture, err := svgimage.Slots(data, "title", "subtitle")
	if err != nil {
		return "", nil, fmt.Errorf("cover %s: %w", rel(root, file), err)
	}
	// As wide as a page, and no image so tall that it takes the memory.
	width, height, _ := svgimage.Size(picture)
	png, err := svgimage.PNG(picture, min(coverPixels/width, 4*coverPixels/height))
	if err != nil {
		return "", nil, fmt.Errorf("cover %s: %w", rel(root, file), err)
	}
	placed := texts[:0]
	for _, t := range texts {
		slot, ok := slots[t.Role]
		if !ok {
			log("warning: cover %s has no element with the id %q; the %s is not on the cover", rel(root, file), t.Role, t.Role)
			continue
		}
		t.Box, t.Color = &docrender.Box{X: slot.X, Y: slot.Y, W: slot.W}, cmp.Or(slot.Fill, t.Color)
		placed = append(placed, t)
	}
	image, err := tempPNG(png)
	return image, placed, err
}

// tempPNG writes a cover image to a temporary file. The caller removes it.
func tempPNG(b []byte) (string, error) {
	f, err := os.CreateTemp("", "folio-cover-*.png")
	if err != nil {
		return "", err
	}
	if _, err = f.Write(b); err == nil {
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
