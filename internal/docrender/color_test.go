package docrender

import "testing"

func TestBrandColors(t *testing.T) {
	p := newPalette(Colors{Text: "222222", Heading: "003366", Link: "0055AA"})
	for name, c := range map[string]struct {
		run  Run
		want rgb
	}{
		"text":         {Run{Text: "a"}, rgb{0x22, 0x22, 0x22}},
		"heading":      {Run{Text: "a", Heading: true}, rgb{0x00, 0x33, 0x66}},
		"link":         {Run{Text: "a", Link: "#x"}, rgb{0x00, 0x55, 0xAA}},
		"heading link": {Run{Text: "a", Heading: true, Link: "#x"}, rgb{0x00, 0x55, 0xAA}},
	} {
		if got := p.of(c.run); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	// Without a brand, and for a color that is none, folio's own stay;
	// headings follow the text.
	p = newPalette(Colors{Text: "nope", Link: "12345"})
	if p.of(Run{}) != (rgb{30, 35, 42}) || p.of(Run{Heading: true}) != (rgb{30, 35, 42}) || p.of(Run{Link: "x"}) != (rgb{24, 84, 139}) {
		t.Errorf("defaults: %+v", p)
	}
	if p = newPalette(Colors{Text: "101010"}); p.of(Run{Heading: true}) != (rgb{0x10, 0x10, 0x10}) {
		t.Errorf("heading does not follow the text: %+v", p)
	}
}
