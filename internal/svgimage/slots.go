package svgimage

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// Slot is an element of an SVG image that marks where a text goes: its
// bounding box as fractions of the image, and its fill as RRGGBB, "" for
// none.
type Slot struct {
	X, Y, W, H float64
	Fill       string
}

// matrix is an SVG transform: x' = a·x + c·y + e, y' = b·x + d·y + f.
type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

// mul is the transform that applies n first, then m.
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

var reTransform = regexp.MustCompile(`(\w+)\s*\(([^)]*)\)`)

// parseTransform reads a transform attribute; a function it does not know
// (the skews) is left out.
func parseTransform(s string) matrix {
	m := identity
	for _, f := range reTransform.FindAllStringSubmatch(s, -1) {
		var a []float64
		for _, v := range strings.FieldsFunc(f[2], func(r rune) bool { return r == ',' || r == ' ' }) {
			a = append(a, num(v))
		}
		arg := func(i int, def float64) float64 {
			if i < len(a) {
				return a[i]
			}
			return def
		}
		switch f[1] {
		case "matrix":
			if len(a) == 6 {
				m = m.mul(matrix(a))
			}
		case "translate":
			m = m.mul(matrix{1, 0, 0, 1, arg(0, 0), arg(1, 0)})
		case "scale":
			m = m.mul(matrix{arg(0, 1), 0, 0, arg(1, arg(0, 1)), 0, 0})
		case "rotate":
			sin, cos := math.Sincos(arg(0, 0) * math.Pi / 180)
			cx, cy := arg(1, 0), arg(2, 0)
			m = m.mul(matrix{1, 0, 0, 1, cx, cy}).mul(matrix{cos, sin, -sin, cos, 0, 0}).mul(matrix{1, 0, 0, 1, -cx, -cy})
		}
	}
	return m
}

// outline returns the points that bound a rect or a path, in its own
// coordinates; a curve is bounded by its control points.
func outline(e xml.StartElement) [][2]float64 {
	switch e.Name.Local {
	case "rect":
		x, y := num(attr(e.Attr, "x")), num(attr(e.Attr, "y"))
		w, h := num(attr(e.Attr, "width")), num(attr(e.Attr, "height"))
		return [][2]float64{{x, y}, {x + w, y}, {x, y + h}, {x + w, y + h}}
	case "path":
		var c oksvg.PathCursor
		if c.CompilePath(attr(e.Attr, "d")) != nil {
			return nil
		}
		var pts [][2]float64
		for i := 0; i < len(c.Path); {
			n := 0
			switch rasterx.PathCommand(c.Path[i]) {
			case rasterx.PathMoveTo, rasterx.PathLineTo:
				n = 2
			case rasterx.PathQuadTo:
				n = 4
			case rasterx.PathCubicTo:
				n = 6
			}
			i++
			for end := min(i+n, len(c.Path)); i+1 < end; i += 2 {
				pts = append(pts, [2]float64{float64(c.Path[i]) / 64, float64(c.Path[i+1]) / 64})
			}
		}
		return pts
	}
	return nil
}

// Slots finds the rect and path elements with the given ids, whatever the
// case of either, and returns them by lower-case id together with the
// image without them: a slot marks a place and is not part of the picture.
func Slots(data []byte, ids ...string) (map[string]Slot, []byte, error) {
	d, err := parse(data)
	if err != nil {
		return nil, nil, err
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[strings.ToLower(id)] = true
	}
	slots := map[string]Slot{}
	var rest []byte
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	styles := []style{{fill: "black"}}
	ctm := []matrix{identity}
	kept, cutDepth, cutFrom := 0, 0, 0 // the slot being cut out ends at cutDepth
	for {
		before := int(dec.InputOffset())
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			st := inherit(styles[len(styles)-1], t.Attr)
			m := ctm[len(ctm)-1].mul(parseTransform(attr(t.Attr, "transform")))
			styles, ctm = append(styles, st), append(ctm, m)
			id := strings.ToLower(attr(t.Attr, "id"))
			pts := outline(t)
			if cutDepth > 0 || !want[id] || len(pts) == 0 {
				continue
			}
			if _, dup := slots[id]; dup {
				continue
			}
			x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
			for _, p := range pts {
				x, y := m.apply(p[0], p[1])
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
			}
			s := Slot{X: (x0 - d.vb[0]) / d.vb[2], Y: (y0 - d.vb[1]) / d.vb[3], W: (x1 - x0) / d.vb[2], H: (y1 - y0) / d.vb[3]}
			if c := parseColor(st.fill); c != nil {
				r, g, b, _ := c.RGBA()
				s.Fill = fmt.Sprintf("%02X%02X%02X", r>>8, g>>8, b>>8)
			}
			slots[id] = s
			cutDepth, cutFrom = len(ctm), before
		case xml.EndElement:
			if cutDepth == len(ctm) {
				rest = append(rest, data[kept:cutFrom]...)
				kept, cutDepth = int(dec.InputOffset()), 0
			}
			styles, ctm = styles[:len(styles)-1], ctm[:len(ctm)-1]
		}
	}
	return slots, append(rest, data[kept:]...), nil
}
