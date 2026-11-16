package build

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// issue84SVG is the template of issue #84 (Illustrator's export), its
// screenshot swapped for a small green square: besides the placeholders,
// a line, a star, a text placed by a transform and an embedded image.
func issue84SVG(t *testing.T) string {
	sq := image.NewNRGBA(image.Rect(0, 0, 35, 36))
	for i := range sq.Pix {
		sq.Pix[i] = []byte{0, 0x80, 0, 0xff}[i%4]
	}
	var b bytes.Buffer
	if err := png.Encode(&b, sq); err != nil {
		t.Fatal(err)
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="595.276" height="841.89" viewBox="0 0 595.276 841.89">
  <defs>
    <style>
      .cls-1{fill:#404040;stroke:#000000;stroke-linejoin:round}
      .cls-2{font-family:&apos;Source Sans 3&apos;, sans-serif;font-size:12px;fill:#333333}
      .cls-3{fill:#fbb03b}
      .cls-4{font-family:Aptos, sans-serif;font-size:16px;font-weight:bold;fill:#404040}
      .cls-5{font-family:Aptos, sans-serif;font-size:36.004px;fill:#404040}
    </style>
  </defs>
  <title>Artboard 1</title>
  <g id="Layer_1" data-name="Layer 1">
    <line id="vertical_bar" data-name="vertical bar" x1="159.771" y1="72.603" x2="159.771" y2="789.322" class="cls-1"/>
    <text transform="matrix(1 0 0 1 36.907 778.682)" x="0" y="0" xml:space="preserve" class="cls-2"><tspan x="0" y="0">example.com</tspan></text>
    <polygon points="153.665,139.483 100.803,138.654 70.152,181.732 54.605,131.201 4.164,115.361 47.418,84.96 46.895,32.094 89.174,63.836 139.291,47.002 122.168,97.021" class="cls-3"/>
    <text id="subtitle" transform="matrix(1 0 0 1 202.396 290.391)" x="0" y="0" xml:space="preserve" class="cls-4"><tspan x="0" y="0">Subtitle</tspan></text>
    <text id="title" transform="matrix(0.3 0 0 1 197.542 188.773)" x="0" y="0" xml:space="preserve" class="cls-5"><tspan x="0" y="0" textLength="207.328" lengthAdjust="spacingAndGlyphs">Title</tspan></text>
    <image id="Screenshot" width="35" height="36" transform="matrix(0.75 0 0 0.75 197.542 761.137)" preserveAspectRatio="none" xlink:href="data:image/png;base64,` +
		base64.StdEncoding.EncodeToString(b.Bytes()) + `"/>
  </g>
</svg>`
}

// RENDER DOCUMENT 5-7, #84: everything of the template but its
// placeholders is drawn on the cover -- the text placed by a transform and
// the embedded image too.
func TestCoverDrawsTheWholeTemplate(t *testing.T) {
	sel := project(t, map[string]string{
		"folio.yml":                  coverConfig,
		"book/index.qmd":             "---\ntitle: Book\n---\n",
		BrandFile:                    "meta:\n  coverpage: /assets/templates/cover.svg\n",
		"assets/templates/cover.svg": issue84SVG(t),
	})
	b, err := loadBrand(sel.Config.Root, "")
	if err != nil {
		t.Fatal(err)
	}
	c, done, err := coverPage(sel, b, Book{Title: "Book"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	f, err := os.Open(c.Image)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	s := float64(img.Bounds().Dx()) / 595.276 // template units to pixels
	area := func(x0, y0, x1, y1 float64) image.Rectangle {
		return image.Rect(int(x0*s), int(y0*s), int(x1*s), int(y1*s))
	}
	for _, c := range []struct {
		what string
		r    image.Rectangle
		want color.NRGBA
	}{
		{"the text placed by a transform", area(36, 768, 110, 781), color.NRGBA{0x33, 0x33, 0x33, 0xff}},
		{"the embedded image", area(199, 763, 222, 786), color.NRGBA{0, 0x80, 0, 0xff}},
		{"the star", area(60, 90, 110, 130), color.NRGBA{0xfb, 0xb0, 0x3b, 0xff}},
	} {
		if !inkedIn(img, c.r, c.want) {
			t.Errorf("%s is not drawn", c.what)
		}
	}
	if inkedIn(img, area(200, 160, 270, 195), color.NRGBA{0x40, 0x40, 0x40, 0xff}) {
		t.Error("the title placeholder is drawn on the image")
	}
}

// inkedIn reports whether r holds a pixel close to want.
func inkedIn(img image.Image, r image.Rectangle, want color.NRGBA) bool {
	near := func(a, b uint8) bool { return int(a)+0x30 >= int(b) && int(b)+0x30 >= int(a) }
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A > 0xc0 && near(c.R, want.R) && near(c.G, want.G) && near(c.B, want.B) {
				return true
			}
		}
	}
	return false
}
