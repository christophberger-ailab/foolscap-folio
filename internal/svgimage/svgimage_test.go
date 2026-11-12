package svgimage

import (
	"bytes"
	"image"
	"image/color"
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
