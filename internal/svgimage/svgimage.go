// Package svgimage turns SVG images into raster images for the writers
// that cannot embed SVG (DOCX, PDF), and holds the placeholder that stands
// in for an image that is not there.
//
// Shapes are drawn by oksvg, which covers the SVG subset icons and
// placeholders are made of. What oksvg leaves out is drawn here: images
// embedded as PNG or JPEG data URIs, in document order, and text, over
// everything else, in the Go fonts, moved and scaled by its transforms but not
// rotated.
package svgimage

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"html"
	"image"
	"image/color"
	_ "image/jpeg" // decodes embedded JPEG images
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/colornames"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
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
	sx, sy := float64(pw)/d.vb[2], float64(ph)/d.vb[3]
	// From the image's user space to its pixels.
	view := matrix{sx, 0, 0, sy, -d.vb[0] * sx, -d.vb[1] * sy}
	dasher := rasterx.NewDasher(pw, ph, rasterx.NewScannerGV(pw, ph, img, img.Bounds()))
	drawn := 0
	shapes := func(n int) { // draws the shapes up to the nth
		for ; drawn < min(n, len(icon.SVGPaths)); drawn++ {
			icon.SVGPaths[drawn].DrawTransformed(dasher, 1, icon.Transform)
		}
	}
	for _, p := range d.pictures {
		// The shapes before the picture: those of the image up to it.
		if before, err := oksvg.ReadIconStream(strings.NewReader(string(data[:p.at])+p.closing), oksvg.IgnoreErrorMode); err == nil {
			shapes(len(before.SVGPaths))
			p.draw(img, view)
		}
	}
	shapes(len(icon.SVGPaths))
	for _, t := range d.texts {
		x, y := view.apply(t.x, t.y)
		t.draw(img, x, y, t.size*sy)
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
	w, h     float64
	vb       [4]float64 // x, y, w, h
	texts    []text
	pictures []picture
}

type text struct {
	// x, y and size are in the image's user space.
	x, y, size float64
	m          matrix // the text element's transform
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
	family string
}

func parse(data []byte) (doc, error) {
	var d doc
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	stack := []style{{fill: "black", size: 16, anchor: "start"}}
	ms := []matrix{identity}
	var names []string // of the open elements
	cls := classes(data)
	var cur *text
	tspan := false // whether cur's origin is its first tspan's
	root := true
	for {
		at := int(dec.InputOffset())
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
			st := inherit(stack[len(stack)-1], t.Attr, cls)
			stack = append(stack, st)
			m := ms[len(ms)-1].mul(transform(attr(t.Attr, "transform")))
			ms = append(ms, m)
			names = append(names, t.Name.Local)
			switch {
			case t.Name.Local == "text" && cur == nil:
				x, y := m.apply(num(firstNum(attr(t.Attr, "x"))), num(firstNum(attr(t.Attr, "y"))))
				cur = &text{
					x: x, y: y, m: m,
					size: st.size * m.scale(), anchor: st.anchor, fill: parseColor(st.fill),
					bold: st.weight == "bold" || num(st.weight) >= 600,
				}
				tspan = false
			case t.Name.Local == "tspan" && cur != nil && !tspan && strings.TrimSpace(cur.s) == "":
				// The first line starts where its tspan says.
				if sx, sy := attr(t.Attr, "x"), attr(t.Attr, "y"); sx != "" || sy != "" {
					cur.x, cur.y = cur.m.apply(num(firstNum(sx)), num(firstNum(sy)))
					tspan = true
				}
			case t.Name.Local == "image" && !referenced(names):
				if p, ok := readPicture(t.Attr, m); ok {
					p.at = at
					for i := len(names) - 2; i >= 0; i-- {
						p.closing += "</" + names[i] + ">"
					}
					d.pictures = append(d.pictures, p)
				}
			}
		case xml.CharData:
			if cur != nil {
				cur.s += string(t)
			}
		case xml.EndElement:
			stack, ms, names = stack[:len(stack)-1], ms[:len(ms)-1], names[:len(names)-1]
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

// picture is an image element: its image, the rectangle it is drawn in
// and its transform; where it starts in the SVG text, and the end tags
// that close the elements around it.
type picture struct {
	at         int
	closing    string
	img        image.Image
	x, y, w, h float64
	stretch    bool // preserveAspectRatio="none"
	m          matrix
}

// referenced reports whether an element inside the open elements names
// is drawn only where something refers to it, if at all.
func referenced(names []string) bool {
	for _, n := range names[:len(names)-1] {
		switch n {
		case "defs", "symbol", "clipPath", "mask", "pattern", "marker":
			return true
		}
	}
	return false
}

// maxPixels bounds the size of an embedded image, against images that
// decode to far more memory than they take up.
const maxPixels = 1 << 26

// readPicture reads an image element whose image is embedded as a PNG or
// JPEG data URI. Linked images are left out: an SVG image must not read
// files or reach the network while it is rasterized.
func readPicture(attrs []xml.Attr, m matrix) (picture, bool) {
	meta, b64, ok := strings.Cut(attr(attrs, "href"), ",")
	meta = strings.ToLower(strings.TrimSpace(meta))
	if !ok || !strings.HasPrefix(meta, "data:image/") || !strings.HasSuffix(meta, ";base64") {
		return picture{}, false
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(b64), ""))
	if err != nil {
		return picture{}, false
	}
	if c, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil || c.Width*c.Height > maxPixels {
		return picture{}, false
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return picture{}, false
	}
	p := picture{img: img, x: num(attr(attrs, "x")), y: num(attr(attrs, "y")), m: m,
		w: px(attr(attrs, "width")), h: px(attr(attrs, "height")),
		stretch: strings.TrimSpace(attr(attrs, "preserveAspectRatio")) == "none"}
	b := img.Bounds()
	switch {
	case p.w <= 0 && p.h <= 0:
		p.w, p.h = float64(b.Dx()), float64(b.Dy())
	case p.h <= 0:
		p.h = p.w * float64(b.Dy()) / float64(b.Dx())
	case p.w <= 0:
		p.w = p.h * float64(b.Dx()) / float64(b.Dy())
	}
	return p, b.Dx() > 0 && b.Dy() > 0
}

// draw draws the picture on dst, view taking the image's user space to
// dst's pixels. Unless it is stretched, the picture keeps its aspect
// ratio, fitted in its rectangle and centred.
func (p picture) draw(dst *image.RGBA, view matrix) {
	b := p.img.Bounds()
	iw, ih := float64(b.Dx()), float64(b.Dy())
	sx, sy := p.w/iw, p.h/ih
	x, y := p.x, p.y
	if !p.stretch {
		s := min(sx, sy)
		x, y = x+(p.w-iw*s)/2, y+(p.h-ih*s)/2
		sx, sy = s, s
	}
	// From the picture's pixels to its user space, then on to dst's.
	m := view.mul(p.m).mul(matrix{sx, 0, 0, sy, x - float64(b.Min.X)*sx, y - float64(b.Min.Y)*sy})
	draw.CatmullRom.Transform(dst, f64.Aff3{m[0], m[2], m[4], m[1], m[3], m[5]}, p.img, b, draw.Over, nil)
}
