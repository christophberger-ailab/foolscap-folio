package svgimage

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// inked reports whether r holds a pixel close to want.
func inked(img image.Image, r image.Rectangle, want color.NRGBA) bool {
	r = r.Intersect(img.Bounds())
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

func near(a, b uint8) bool { return int(a)+0x30 >= int(b) && int(b)+0x30 >= int(a) }

func rasterize(t *testing.T, svg string) image.Image {
	t.Helper()
	img, err := Rasterize([]byte(svg), 1)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

var (
	red   = color.NRGBA{0xff, 0, 0, 0xff}
	green = color.NRGBA{0, 0x80, 0, 0xff}
	blue  = color.NRGBA{0, 0, 0xff, 0xff}
)

// RENDER DOCUMENT 4-8, #84: a text placed by a transform is drawn where
// the transform puts it, not at its own x and y.
func TestTextFollowsItsTransform(t *testing.T) {
	img := rasterize(t, `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="100">`+
		`<text transform="matrix(1 0 0 1 120 80)" x="0" y="0" font-size="20" fill="#0000ff">Hi</text></svg>`)
	if !inked(img, image.Rect(115, 60, 160, 85), blue) {
		t.Error("the text is not drawn where its transform puts it")
	}
	if inked(img, image.Rect(0, 0, 40, 20), blue) {
		t.Error("the text is drawn at its untransformed position")
	}
}

// #84: an <image> with a data URI is drawn.
func TestEmbeddedImageIsDrawn(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range src.Pix {
		src.Pix[i] = []byte{0, 0x80, 0, 0xff}[i%4]
	}
	var b bytes.Buffer
	if err := png.Encode(&b, src); err != nil {
		t.Fatal(err)
	}
	img := rasterize(t, `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="100" height="100">`+
		`<image x="50" y="50" width="40" height="40" preserveAspectRatio="none" xlink:href="data:image/png;base64,`+
		base64.StdEncoding.EncodeToString(b.Bytes())+`"/></svg>`)
	if !inked(img, image.Rect(60, 60, 80, 80), green) {
		t.Error("the embedded image is not drawn")
	}
}

// Shapes and text take their style from the stylesheet's classes, as
// diagram editors write them.
func TestStylesheetClassesApply(t *testing.T) {
	img := rasterize(t, `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50">`+
		`<style>.a{fill:#ff0000}.b{fill:#0000ff;font-size:30px}</style>`+
		`<rect class="a" x="0" y="0" width="40" height="40"/>`+
		`<text class="b" x="50" y="35">M</text></svg>`)
	if !inked(img, image.Rect(10, 10, 30, 30), red) {
		t.Error("the rect does not take its class's fill")
	}
	if !inked(img, image.Rect(50, 5, 100, 40), blue) {
		t.Error("the text does not take its class's fill")
	}
}

// A label of several lines, as tspans, has each line where its tspan
// puts it.
func TestTspansAreLines(t *testing.T) {
	img := rasterize(t, `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="120">`+
		`<text font-size="20" fill="#0000ff"><tspan x="10" y="25">One</tspan><tspan x="120" y="100">Two</tspan></text></svg>`)
	if !inked(img, image.Rect(5, 5, 60, 30), blue) {
		t.Error("the first line is missing")
	}
	if !inked(img, image.Rect(115, 80, 190, 105), blue) {
		t.Error("the second line is not drawn at its own position")
	}
}

// draw.io writes a label as HTML in a foreignObject, with a plain text
// beside it in a switch for renderers without HTML: that text is drawn.
func TestDrawioLabelFallsBackToText(t *testing.T) {
	img := rasterize(t, `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="60">`+
		`<switch><foreignObject x="0" y="0" width="200" height="60" requiredFeatures="http://www.w3.org/TR/SVG11/feature#Extensibility">`+
		`<div xmlns="http://www.w3.org/1999/xhtml" style="color:#0000ff">Label</div></foreignObject>`+
		`<text x="100" y="40" fill="#0000ff" font-family="Helvetica" font-size="24px" text-anchor="middle">Label</text></switch></svg>`)
	if !inked(img, image.Rect(60, 15, 140, 45), blue) {
		t.Error("the label's fallback text is not drawn")
	}
}

// An image too large to draw is an error that says so, and the renderer
// goes on working.
func TestHugeImageIsRefused(t *testing.T) {
	for _, svg := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg" width="100000" height="100000"/>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="20000" height="20000"/>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="1e300" height="1"/>`,
	} {
		if _, err := Rasterize([]byte(svg), 1); err == nil || !strings.Contains(err.Error(), "too large") {
			t.Errorf("%s: error %v, want one saying it is too large", svg, err)
		}
	}
	if _, err := Rasterize([]byte(square), 1); err != nil {
		t.Errorf("after a refusal: %v", err)
	}
}
