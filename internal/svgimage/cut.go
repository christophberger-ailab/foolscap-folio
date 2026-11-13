package svgimage

import (
	"bytes"
	"encoding/xml"
	"image/color"
	"io"
	"sort"
	"strings"
)

// Box is a rectangle on an image, in fractions of its width and height.
type Box struct{ X, Y, W, H float64 }

// Slot is an element cut out of an SVG image: where it was drawn, and its
// fill and text anchor (as inherited), for what takes its place.
type Slot struct {
	Box
	// Fill is nil when the element's fill is none or unreadable.
	Fill   color.Color
	Anchor string
}

// element is an element of an SVG text: its byte range, its tokens'
// range and its parent's index (-1 for the root).
type element struct {
	name             string
	start, end       int
	first, last      int
	parent           int
	id, anchor, fill string
}

// Cut removes the elements whose id is one of ids, ignoring case, from an
// SVG image. It returns the image without them and, by the id as given,
// the slot of each. An element that draws nothing, transparent or not,
// has no slot, as has an id that no element carries.
func Cut(data []byte, ids ...string) ([]byte, map[string]Slot, error) {
	if _, err := parse(data); err != nil {
		return nil, nil, err
	}
	var els []element
	var toks []xml.Token
	open := []int{}
	styles := []style{{fill: "black", anchor: "start"}}
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
			st := inherit(styles[len(styles)-1], t.Attr)
			styles = append(styles, st)
			els = append(els, element{name: t.Name.Local, start: before, first: len(toks) - 1, parent: parent,
				id: attr(t.Attr, "id"), anchor: st.anchor, fill: st.fill})
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
		box, ok := locate(data, els, toks, i)
		if ok {
			slots[id] = Slot{Box: box, Fill: parseColor(e.fill), Anchor: e.anchor}
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
func locate(data []byte, els []element, toks []xml.Token, i int) (Box, bool) {
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
		return Box{}, false
	}
	scale := 1000 / max(d.w, d.h)
	img, err := Rasterize(apply(data, edits), scale)
	if err != nil {
		return Box{}, false
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
		return Box{}, false
	}
	w, h := float64(b.Dx()), float64(b.Dy())
	return Box{float64(x0) / w, float64(y0) / h, float64(x1-x0) / w, float64(y1-y0) / h}, true
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
