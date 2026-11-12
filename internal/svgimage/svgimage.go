// Package svgimage turns SVG images into raster images for the writers
// that cannot embed SVG (DOCX, PDF), and holds the placeholder that stands
// in for an image that is not there.
//
// Shapes are drawn by oksvg, which covers the SVG subset icons and
// placeholders are made of; text, which oksvg leaves out, is drawn here in
// the Go fonts, unrotated and untransformed.
package svgimage

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/colornames"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// PlaceholderPath is where a project keeps the image that stands in for a
// missing one, relative to the project root.
const PlaceholderPath = "assets/static/_placeholder.svg"

//go:embed placeholder.svg
var builtIn []byte

// Placeholder returns the project's placeholder image, or the one built
// into folio when the project has none.
func Placeholder(root string) []byte {
	if data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(PlaceholderPath))); err == nil {
		return data
	}
	return builtIn
}

// IsSVG reports whether name has the .svg extension.
func IsSVG(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".svg")
}

var errNotSVG = errors.New("not an SVG image, or one without a size")

// Size returns the natural size of an SVG image in pixels: its width and
// height in px, or the size of its viewBox, the one following the other's
// aspect ratio when only one is given.
func Size(data []byte) (w, h float64, err error) {
	d, err := parse(data)
	if err != nil {
		return 0, 0, err
	}
	return d.w, d.h, nil
}

// Rasterize draws an SVG image at scale times its natural size on a
// transparent background.
func Rasterize(data []byte, scale float64) (image.Image, error) {
	d, err := parse(data)
	if err != nil {
		return nil, err
	}
	icon, err := oksvg.ReadIconStream(strings.NewReader(string(data)), oksvg.IgnoreErrorMode)
	if err != nil {
		return nil, err
	}
	pw, ph := max(1, int(d.w*scale+0.5)), max(1, int(d.h*scale+0.5))
	icon.ViewBox.X, icon.ViewBox.Y, icon.ViewBox.W, icon.ViewBox.H = d.vb[0], d.vb[1], d.vb[2], d.vb[3]
	icon.SetTarget(0, 0, float64(pw), float64(ph))
	img := image.NewRGBA(image.Rect(0, 0, pw, ph))
	icon.Draw(rasterx.NewDasher(pw, ph, rasterx.NewScannerGV(pw, ph, img, img.Bounds())), 1)
	sx, sy := float64(pw)/d.vb[2], float64(ph)/d.vb[3]
	for _, t := range d.texts {
		t.draw(img, (t.x-d.vb[0])*sx, (t.y-d.vb[1])*sy, t.size*sy)
	}
	return img, nil
}

// PNG is Rasterize, encoded as PNG.
func PNG(data []byte, scale float64) ([]byte, error) {
	img, err := Rasterize(data, scale)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type doc struct {
	w, h  float64
	vb    [4]float64 // x, y, w, h
	texts []text
}

type text struct {
	x, y, size float64
	anchor     string
	bold       bool
	fill       color.Color
	s          string
}

// style is what a text inherits from the elements around it.
type style struct {
	fill   string
	size   float64
	anchor string
	weight string
}

func parse(data []byte) (doc, error) {
	var d doc
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	stack := []style{{fill: "black", size: 16, anchor: "start"}}
	var cur *text
	root := true
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return doc{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if root {
				if t.Name.Local != "svg" {
					return doc{}, errNotSVG
				}
				if err := d.size(t.Attr); err != nil {
					return doc{}, err
				}
				root = false
			}
			st := inherit(stack[len(stack)-1], t.Attr)
			stack = append(stack, st)
			if t.Name.Local == "text" && cur == nil {
				cur = &text{
					x: num(attr(t.Attr, "x")), y: num(attr(t.Attr, "y")),
					size: st.size, anchor: st.anchor, fill: parseColor(st.fill),
					bold: st.weight == "bold" || num(st.weight) >= 600,
				}
			}
		case xml.CharData:
			if cur != nil {
				cur.s += string(t)
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
			if t.Name.Local == "text" && cur != nil {
				if cur.s = strings.Join(strings.Fields(cur.s), " "); cur.s != "" && cur.fill != nil {
					d.texts = append(d.texts, *cur)
				}
				cur = nil
			}
		}
	}
	if root {
		return doc{}, errNotSVG
	}
	return d, nil
}

// size reads the root element's width, height and viewBox.
func (d *doc) size(attrs []xml.Attr) error {
	w, h := px(attr(attrs, "width")), px(attr(attrs, "height"))
	vb := strings.FieldsFunc(attr(attrs, "viewBox"), func(r rune) bool { return r == ',' || r == ' ' })
	if len(vb) == 4 {
		for i, f := range vb {
			d.vb[i] = num(f)
		}
	}
	if d.vb[2] > 0 && d.vb[3] > 0 {
		switch {
		case w == 0 && h == 0:
			w, h = d.vb[2], d.vb[3]
		case h == 0:
			h = w * d.vb[3] / d.vb[2]
		case w == 0:
			w = h * d.vb[2] / d.vb[3]
		}
	}
	if w <= 0 || h <= 0 {
		return errNotSVG
	}
	if d.vb[2] <= 0 || d.vb[3] <= 0 {
		d.vb = [4]float64{0, 0, w, h}
	}
	d.w, d.h = w, h
	return nil
}

func inherit(st style, attrs []xml.Attr) style {
	set := func(k, v string) {
		v = strings.TrimSpace(v)
		switch k {
		case "fill":
			st.fill = v
		case "font-size":
			if s := px(v); s > 0 {
				st.size = s
			}
		case "text-anchor":
			st.anchor = v
		case "font-weight":
			st.weight = v
		}
	}
	for _, a := range attrs {
		set(a.Name.Local, a.Value)
	}
	for _, decl := range strings.Split(attr(attrs, "style"), ";") {
		if k, v, ok := strings.Cut(decl, ":"); ok {
			set(strings.TrimSpace(k), v)
		}
	}
	return st
}

func attr(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func num(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// px reads a length in pixels; any other unit reads as none.
func px(s string) float64 {
	return num(strings.TrimSuffix(strings.TrimSpace(s), "px"))
}

// parseColor reads #rgb, #rrggbb and color names; none and what it cannot
// read are nil, drawing nothing.
func parseColor(s string) color.Color {
	s = strings.ToLower(strings.TrimSpace(s))
	if c, ok := colornames.Map[s]; ok {
		return c
	}
	if !strings.HasPrefix(s, "#") {
		return nil
	}
	hex := s[1:]
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || len(hex) != 6 {
		return nil
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}
}

var regular, bold = mustFont(goregular.TTF), mustFont(gobold.TTF)

func mustFont(ttf []byte) *opentype.Font {
	f, err := opentype.Parse(ttf)
	if err != nil {
		panic(err)
	}
	return f
}

// draw writes the text with its baseline at y, anchored at x.
func (t text) draw(img *image.RGBA, x, y, size float64) {
	f := regular
	if t.bold {
		f = bold
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return
	}
	defer face.Close()
	d := &font.Drawer{Dst: img, Src: image.NewUniform(t.fill), Face: face}
	adv := float64(d.MeasureString(t.s)) / 64
	switch t.anchor {
	case "middle":
		x -= adv / 2
	case "end":
		x -= adv
	}
	d.Dot = fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)}
	d.DrawString(t.s)
}
