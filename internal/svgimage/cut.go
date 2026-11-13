package svgimage

import (
	"bytes"
	"encoding/xml"
	"errors"
	"image/color"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Box is a rectangle on an image, in fractions of its width and height.
type Box struct{ X, Y, W, H float64 }

// Slot is an element cut out of an SVG image: where it was drawn, and its
// fill, text anchor and font family (as inherited, classes applied), for
// what takes its place.
type Slot struct {
	Box
	// Fill is nil when the element's fill is none or unreadable.
	Fill   color.Color
	Anchor string
	Family string
	// Text is set for a text element. Its sample text's first line starts
	// at X (or is centred on it or ends there, as Anchor says) with its
	// baseline at Baseline, in Size; Baseline and Size are fractions of
	// the image's height. The Box is approximate: Size high above the
	// baseline, without a width.
	Text           bool
	Baseline, Size float64
}

// element is an element of an SVG text: its byte range, its tokens'
// range and its parent's index (-1 for the root).
type element struct {
	name        string
	start, end  int
	first, last int
	parent      int
	id          string
	style       style
	// m is the transform from the element's coordinates to the image's.
	m matrix
}

// Cut removes the elements whose id is one of ids, ignoring case, from an
// SVG image. It returns the image without them and, by the id as given,
// the slot of each. An element that draws nothing, transparent or not,
// has no slot, as has an id that no element carries.
func Cut(data []byte, ids ...string) ([]byte, map[string]Slot, error) {
	d, err := parse(data)
	if err != nil {
		return nil, nil, err
	}
	var els []element
	var toks []xml.Token
	open := []int{}
	styles := []style{{fill: "black", size: 16, anchor: "start"}}
	cls := classes(data)
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		before := int(dec.InputOffset())
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		toks = append(toks, xml.CopyToken(tok))
		switch t := tok.(type) {
		case xml.StartElement:
			parent := -1
			if len(open) > 0 {
				parent = open[len(open)-1]
			}
			st := inherit(styles[len(styles)-1], t.Attr, cls)
			styles = append(styles, st)
			m := identity
			if parent >= 0 {
				m = els[parent].m
			}
			els = append(els, element{name: t.Name.Local, start: before, first: len(toks) - 1, parent: parent,
				id: attr(t.Attr, "id"), style: st, m: m.mul(transform(attr(t.Attr, "transform")))})
			open = append(open, len(els)-1)
		case xml.EndElement:
			e := &els[open[len(open)-1]]
			e.end, e.last = int(dec.InputOffset()), len(toks)-1
			open, styles = open[:len(open)-1], styles[:len(styles)-1]
		}
	}

	slots := map[string]Slot{}
	var cuts []edit
	for _, id := range ids {
		i := byID(els, id)
		if i < 0 {
			continue
		}
		e := els[i]
		cuts = append(cuts, edit{e.start, e.end, ""})
		slot := Slot{Fill: parseColor(e.style.fill), Anchor: e.style.anchor, Family: e.style.family}
		if e.name == "text" {
			slot.Text = true
			x, y := textOrigin(toks[e.first : e.last+1])
			x, y = e.m.apply(x, y)
			size := e.style.size * e.m.scale()
			slot.X, slot.Baseline, slot.Size = (x-d.vb[0])/d.vb[2], (y-d.vb[1])/d.vb[3], size/d.vb[3]
			slot.Box = Box{X: slot.X, Y: slot.Baseline - slot.Size, H: slot.Size}
			slots[id] = slot
		} else if slot.Box, err = locate(data, els, toks, i); err == nil {
			slots[id] = slot
		}
	}
	return apply(data, cuts), slots, nil
}

// byID returns the index of the first element whose id is id, ignoring
// case, or -1.
func byID(els []element, id string) int {
	for i, e := range els {
		if e.id != "" && strings.EqualFold(e.id, id) {
			return i
		}
	}
	return -1
}

// locate rasterizes the image with nothing but element i, drawn opaque,
// and returns the box its pixels cover.
func locate(data []byte, els []element, toks []xml.Token, i int) (Box, error) {
	chain := map[int]bool{}
	for p := els[i].parent; p >= 0; p = els[p].parent {
		chain[p] = true
	}
	var edits []edit
	for j, e := range els {
		if j != i && chain[e.parent] && !chain[j] && e.name != "defs" && e.name != "style" {
			edits = append(edits, edit{e.start, e.end, ""})
		}
	}
	edits = append(edits, edit{els[i].start, els[i].end, opaque(toks[els[i].first : els[i].last+1])})
	d, err := parse(data)
	if err != nil {
		return Box{}, errNothingDrawn
	}
	scale := 1000 / max(d.w, d.h)
	img, err := Rasterize(apply(data, edits), scale)
	if err != nil {
		return Box{}, errNothingDrawn
	}
	b := img.Bounds()
	x0, y0, x1, y1 := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x+1), max(y1, y+1)
			}
		}
	}
	if x1 <= x0 || y1 <= y0 {
		return Box{}, errNothingDrawn
	}
	w, h := float64(b.Dx()), float64(b.Dy())
	return Box{float64(x0) / w, float64(y0) / h, float64(x1-x0) / w, float64(y1-y0) / h}, nil
}

// paint are the attributes that could hide an element or change its
// color; opaque drops them.
var paint = map[string]bool{
	"fill": true, "stroke": true, "opacity": true, "fill-opacity": true, "stroke-opacity": true,
	"style": true, "class": true, "display": true, "visibility": true, "filter": true, "mask": true,
}

// opaque writes an element's tokens again, its paint attributes dropped
// and its fill black, so that it shows wherever it draws anything.
func opaque(toks []xml.Token) string {
	var b strings.Builder
	for i, tok := range toks {
		switch t := tok.(type) {
		case xml.StartElement:
			b.WriteString("<" + t.Name.Local)
			if i == 0 {
				b.WriteString(` fill="#000"`)
			}
			for _, a := range t.Attr {
				if a.Name.Space == "" && !paint[a.Name.Local] {
					b.WriteString(" " + a.Name.Local + `="`)
					xml.EscapeText(&b, []byte(a.Value))
					b.WriteString(`"`)
				}
			}
			b.WriteString(">")
		case xml.EndElement:
			b.WriteString("</" + t.Name.Local + ">")
		case xml.CharData:
			xml.EscapeText(&b, t)
		}
	}
	return b.String()
}

// edit replaces data[start:end] with text.
type edit struct {
	start, end int
	text       string
}

// apply makes the edits, leaving out any inside an earlier one.
func apply(data []byte, edits []edit) []byte {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out []byte
	at := 0
	for _, e := range edits {
		if e.start < at {
			continue
		}
		out = append(append(out, data[at:e.start]...), e.text...)
		at = e.end
	}
	return append(out, data[at:]...)
}

var errNothingDrawn = errors.New("the element draws nothing")

// textOrigin returns where a text element's first line starts: the x and
// y of its first tspan that has them, else its own.
func textOrigin(toks []xml.Token) (x, y float64) {
	for i, tok := range toks {
		if t, ok := tok.(xml.StartElement); ok && (i == 0 || t.Name.Local == "tspan") {
			if sx, sy := attr(t.Attr, "x"), attr(t.Attr, "y"); sx != "" || sy != "" {
				x, y = num(firstNum(sx)), num(firstNum(sy))
				if i > 0 {
					return x, y
				}
			}
		}
	}
	return x, y
}

// firstNum is the first of a list of numbers, as x and y may hold one per
// glyph.
func firstNum(s string) string {
	f := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// matrix is an SVG transform, a b c d e f.
type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

// mul is m followed by n applied first: the transform of an element (n)
// inside one with m.
func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[2]*n[1], m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3], m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4], m[1]*n[4] + m[3]*n[5] + m[5],
	}
}

func (m matrix) apply(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

// scale is the factor lengths grow by, on average.
func (m matrix) scale() float64 { return math.Sqrt(math.Abs(m[0]*m[3] - m[1]*m[2])) }

var reTransform = regexp.MustCompile(`(matrix|translate|scale|rotate|skewX|skewY)\s*\(([^)]*)\)`)

// transform reads a transform attribute: its matrix, translate, scale and
// rotate (about the origin) functions; skews are left out.
func transform(s string) matrix {
	m := identity
	for _, f := range reTransform.FindAllStringSubmatch(s, -1) {
		var v []float64
		for _, a := range strings.FieldsFunc(f[2], func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }) {
			v = append(v, num(a))
		}
		n := identity
		switch {
		case f[1] == "matrix" && len(v) == 6:
			n = matrix(v)
		case f[1] == "translate" && len(v) >= 1:
			n[4] = v[0]
			if len(v) > 1 {
				n[5] = v[1]
			}
		case f[1] == "scale" && len(v) >= 1:
			n[0], n[3] = v[0], v[0]
			if len(v) > 1 {
				n[3] = v[1]
			}
		case f[1] == "rotate" && len(v) == 1:
			sin, cos := math.Sincos(v[0] * math.Pi / 180)
			n = matrix{cos, sin, -sin, cos, 0, 0}
		}
		m = m.mul(n)
	}
	return m
}
