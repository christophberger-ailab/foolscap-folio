package svgimage

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

const square = `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20" viewBox="0 0 4 2">` +
	`<rect x="0" y="0" width="2" height="2" fill="#ff0000"/>` +
	`<text x="3" y="1.5" font-size="2" fill="#0000ff">X</text></svg>`

func TestSize(t *testing.T) {
	for _, c := range []struct {
		svg  string
		w, h float64
	}{
		{square, 40, 20},
		{`<svg width="30px" height="10px"/>`, 30, 10},
		{`<svg viewBox="0 0 8 6"/>`, 8, 6},
		{`<svg width="100%" viewBox="0 0 8 6"/>`, 8, 6},
		{`<svg width="16" viewBox="0 0 8 6"/>`, 16, 12},
	} {
		w, h, err := Size([]byte(c.svg))
		if err != nil {
			t.Fatalf("%s: %v", c.svg, err)
		}
		if w != c.w || h != c.h {
			t.Errorf("%s: size %vx%v, want %vx%v", c.svg, w, h, c.w, c.h)
		}
	}
}

func TestSizeRejectsNonSVG(t *testing.T) {
	for _, s := range []string{"", "not xml", `<html/>`, `<svg/>`} {
		if _, _, err := Size([]byte(s)); err == nil {
			t.Errorf("%q: no error", s)
		}
	}
}

func TestRasterizeDrawsShapesAndText(t *testing.T) {
	img, err := Rasterize([]byte(square), 2)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 80 || b.Dy() != 40 {
		t.Fatalf("bounds %v, want 80x40", b)
	}
	if r, g, b, a := img.At(10, 20).RGBA(); r>>8 != 0xff || g != 0 || b != 0 || a>>8 != 0xff {
		t.Errorf("rect pixel = %v, want red", img.At(10, 20))
	}
	if _, _, _, a := img.At(70, 2).RGBA(); a != 0 {
		t.Errorf("background pixel = %v, want transparent", img.At(70, 2))
	}
	blue := false
	for y := 0; y < 40 && !blue; y++ {
		for x := 40; x < 80; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.B > 0x80 && c.R < 0x40 && c.A > 0x80 {
				blue = true
				break
			}
		}
	}
	if !blue {
		t.Error("the text was not drawn")
	}
}

func TestPlaceholderPrefersTheProjects(t *testing.T) {
	root := t.TempDir()
	if got := Placeholder(root); string(got) != string(builtIn) || len(got) == 0 {
		t.Error("a project without a placeholder does not get the built-in one")
	}
	own := []byte(`<svg width="1" height="1"/>`)
	p := filepath.Join(root, filepath.FromSlash(PlaceholderPath))
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, own, 0o644)
	if got := Placeholder(root); string(got) != string(own) {
		t.Errorf("got %q, want the project's placeholder", got)
	}
}

func TestBuiltInPlaceholderRasterizes(t *testing.T) {
	if _, err := Rasterize(builtIn, 1); err != nil {
		t.Fatal(err)
	}
}

func TestPNG(t *testing.T) {
	b, err := PNG([]byte(square), 1)
	if err != nil {
		t.Fatal(err)
	}
	c, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || format != "png" || c.Width != 40 || c.Height != 20 {
		t.Errorf("PNG: %v %s %dx%d, want a 40x20 png", err, format, c.Width, c.Height)
	}
}

// found reports whether any pixel in r is close to c.
func found(img image.Image, r image.Rectangle, c color.NRGBA) bool {
	near := func(a, b uint8) bool { return max(a, b)-min(a, b) < 0x30 }
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			p := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if p.A > 0x80 && near(p.R, c.R) && near(p.G, c.G) && near(p.B, c.B) {
				return true
			}
		}
	}
	return false
}

func TestRasterizeDrawsTransformedText(t *testing.T) {
	// The text is at 0,0 but moved to 50,80 by its transform; its tspan
	// says where its line starts.
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100">` +
		`<g transform="translate(10 0)"><text transform="matrix(1 0 0 1 30 70)" x="0" y="0" font-size="12" fill="#0000ff">` +
		`<tspan x="5" y="10">XXXX</tspan></text></g></svg>`
	img, err := Rasterize([]byte(svg), 1)
	if err != nil {
		t.Fatal(err)
	}
	blue := color.NRGBA{0, 0, 0xff, 0xff}
	if !found(img, image.Rect(45, 68, 80, 80), blue) {
		t.Error("the text was not drawn where its transform puts it")
	}
	if found(img, image.Rect(0, 0, 40, 40), blue) {
		t.Error("the text was drawn at its untransformed origin")
	}
}

// redPNG is a 2x2 red PNG as a data URI.
func redPNG(t *testing.T) string {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for i := range img.Pix {
		img.Pix[i] = []uint8{0xff, 0, 0, 0xff}[i%4]
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestRasterizeDrawsEmbeddedImages(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="100" height="100">` +
		`<image width="20" height="40" transform="matrix(0.5 0 0 0.5 60 50)" preserveAspectRatio="none" xlink:href="` + redPNG(t) + `"/>` +
		`<image x="0" y="0" width="40" height="20" href="` + redPNG(t) + `"/></svg>`
	img, err := Rasterize([]byte(svg), 2)
	if err != nil {
		t.Fatal(err)
	}
	red := color.NRGBA{0xff, 0, 0, 0xff}
	// The first one covers 60..70 × 50..70, stretched; scale 2 doubles it.
	if !found(img, image.Rect(122, 102, 138, 138), red) {
		t.Error("the transformed image was not drawn")
	}
	if found(img, image.Rect(142, 102, 160, 138), red) {
		t.Error("the transformed image was drawn too wide")
	}
	// The second one keeps its aspect ratio: a 20x20 square centred in
	// 40x20, so at 10..30 × 0..20.
	if !found(img, image.Rect(22, 2, 58, 38), red) {
		t.Error("the image was not drawn")
	}
	if found(img, image.Rect(0, 0, 18, 40), red) || found(img, image.Rect(62, 0, 80, 40), red) {
		t.Error("the image did not keep its aspect ratio")
	}
}

func TestRasterizeIgnoresLinkedImages(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><image width="10" height="10" href="/etc/passwd"/></svg>`
	img, err := Rasterize([]byte(svg), 1)
	if err != nil {
		t.Fatal(err)
	}
	if found(img, img.Bounds(), color.NRGBA{}) {
		t.Error("a linked image drew something")
	}
}

func TestRasterizeKeepsImagesInDocumentOrder(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="30" height="10">` +
		`<rect width="30" height="10" fill="#00ff00"/>` +
		`<g><image width="10" height="10" href="` + redPNG(t) + `"/></g>` +
		`<rect x="10" width="20" height="10" fill="#0000ff"/>` +
		`<image x="20" width="10" height="10" href="` + redPNG(t) + `"/>` +
		`<defs><image id="unused" width="30" height="10" href="` + redPNG(t) + `"/></defs></svg>`
	img, err := Rasterize([]byte(svg), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		x    int
		want color.NRGBA
	}{{5, color.NRGBA{0xff, 0, 0, 0xff}}, {15, color.NRGBA{0, 0, 0xff, 0xff}}, {25, color.NRGBA{0xff, 0, 0, 0xff}}} {
		if !found(img, image.Rect(c.x-2, 3, c.x+2, 7), c.want) {
			t.Errorf("at x=%d: %v, want %v", c.x, img.At(c.x, 5), c.want)
		}
	}
}
