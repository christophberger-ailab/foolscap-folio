// Package svgimage turns SVG images into raster images for the writers
// that cannot embed SVG (DOCX, PDF), and holds the placeholder that stands
// in for an image that is not there.
//
// The images are drawn by resvg (see resvg.go), their text in the Go
// fonts.
package svgimage

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/image/colornames"
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

// maxPixels is the size of the largest image Rasterize draws, some
// 10000×10000 pixels: a cover page is 1654 pixels wide.
const maxPixels = 100e6

// Rasterize draws an SVG image at scale times its natural size on a
// transparent background.
func Rasterize(data []byte, scale float64) (image.Image, error) {
	d, err := parse(data)
	if err != nil {
		return nil, err
	}
	w, h := math.Round(d.w*scale), math.Round(d.h*scale)
	if w*h > maxPixels {
		return nil, fmt.Errorf("svg: an image of %.0f×%.0f pixels is too large to draw", w, h)
	}
	return draw(data, max(1, int(w)), max(1, int(h)))
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
	w, h float64
	vb   [4]float64 // x, y, w, h
}

// style is what a text inherits from the elements around it.
type style struct {
	fill   string
	size   float64
	anchor string
	weight string
	family string
}

// parse reads the size of an image from its root element.
func parse(data []byte) (doc, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return doc{}, errNotSVG
		}
		if err != nil {
			return doc{}, err
		}
		if t, ok := tok.(xml.StartElement); ok {
			if t.Name.Local != "svg" {
				return doc{}, errNotSVG
			}
			var d doc
			return d, d.size(t.Attr)
		}
	}
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

var (
	reStyle = regexp.MustCompile(`(?s)<style[^>]*>(.*?)</style>`)
	reRule  = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
)

// classes reads the class rules (".name { ... }") of an image's style
// sheets: their declarations, by class name. Other selectors are left out.
func classes(data []byte) map[string]string {
	out := map[string]string{}
	for _, m := range reStyle.FindAllSubmatch(data, -1) {
		css := html.UnescapeString(strings.NewReplacer("<![CDATA[", "", "]]>", "").Replace(string(m[1])))
		for _, r := range reRule.FindAllStringSubmatch(css, -1) {
			for _, sel := range strings.Split(r[1], ",") {
				if sel = strings.TrimSpace(sel); strings.HasPrefix(sel, ".") && !strings.ContainsAny(sel[1:], " .#:>[+~") {
					out[sel[1:]] += ";" + r[2]
				}
			}
		}
	}
	return out
}

// inherit applies an element's presentation attributes, then the rules of
// its classes in cls, then its style attribute to what it inherits.
func inherit(st style, attrs []xml.Attr, cls map[string]string) style {
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
		case "font-family":
			// The first family, unquoted.
			first, _, _ := strings.Cut(v, ",")
			st.family = strings.Trim(strings.TrimSpace(first), `'"`)
		}
	}
	decls := func(s string) {
		for _, decl := range strings.Split(s, ";") {
			if k, v, ok := strings.Cut(decl, ":"); ok {
				set(strings.TrimSpace(k), v)
			}
		}
	}
	for _, a := range attrs {
		set(a.Name.Local, a.Value)
	}
	for _, c := range strings.Fields(attr(attrs, "class")) {
		decls(cls[c])
	}
	decls(attr(attrs, "style"))
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
