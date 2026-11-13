package docrender

import "encoding/hex"

// Colors are the colors of a document's text, headings and links as
// RRGGBB. "" keeps folio's own; headings without a color follow the text.
type Colors struct{ Text, Heading, Link string }

type rgb [3]uint8

// palette holds the colors text is drawn in.
type palette struct{ text, heading, link rgb }

// parseRGB reads RRGGBB; anything else is def.
func parseRGB(s string, def rgb) rgb {
	if b, err := hex.DecodeString(s); err == nil && len(b) == 3 {
		return rgb(b)
	}
	return def
}

func newPalette(c Colors) palette {
	text := parseRGB(c.Text, rgb{30, 35, 42})
	return palette{text, parseRGB(c.Heading, text), parseRGB(c.Link, rgb{24, 84, 139})}
}

// of is the color of a run: a link's wins over a heading's.
func (p palette) of(r Run) rgb {
	switch {
	case r.Link != "":
		return p.link
	case r.Heading:
		return p.heading
	}
	return p.text
}
