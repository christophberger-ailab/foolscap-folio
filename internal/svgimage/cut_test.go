package svgimage

import (
	"bytes"
	"image/color"
	"math"
	"os"
	"strings"
	"testing"
)

const coverSVG = `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="200" height="100">
  <rect width="200" height="100" fill="#eeeeee"/>
  <g transform="translate(10,0)" fill="#ff0000">
    <rect x="0" y="0" width="5" height="5"/>
    <path id="Title" inkscape:label="Title" style="opacity:0" d="M20 10 h60 v20 h-60 z"/>
  </g>
  <text id="subtitle" x="100" y="80" font-size="10" text-anchor="middle" fill="#0000ff">Sub</text>
</svg>`

func TestCut(t *testing.T) {
	rest, slots, err := Cut([]byte(coverSVG), "title", "subtitle", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rest, []byte(`id="Title"`)) || bytes.Contains(rest, []byte(">Sub<")) {
		t.Errorf("placeholders left in:\n%s", rest)
	}
	if !bytes.Contains(rest, []byte(`<rect x="0" y="0" width="5" height="5"/>`)) {
		t.Errorf("other elements removed:\n%s", rest)
	}
	if _, err := Rasterize(rest, 1); err != nil {
		t.Errorf("rest: %v", err)
	}
	if _, ok := slots["missing"]; ok || len(slots) != 2 {
		t.Fatalf("slots %v", slots)
	}
	near := func(got, want float64) bool { return math.Abs(got-want) < 0.015 }
	// The path is invisible itself; its geometry counts, transform applied.
	ti := slots["title"]
	if !near(ti.X, 30.0/200) || !near(ti.Y, 0.1) || !near(ti.W, 60.0/200) || !near(ti.H, 0.2) {
		t.Errorf("title box %+v", ti.Box)
	}
	if ti.Fill != (color.RGBA{0xff, 0, 0, 0xff}) || ti.Anchor != "start" {
		t.Errorf("title fill %v, anchor %q", ti.Fill, ti.Anchor)
	}
	sub := slots["subtitle"]
	if sub.Anchor != "middle" || sub.Fill != (color.RGBA{0, 0, 0xff, 0xff}) {
		t.Errorf("subtitle %+v", sub)
	}
	if sub.Y < 0.65 || sub.Y+sub.H > 0.85 || !near(sub.X+sub.W/2, 0.5) {
		t.Errorf("subtitle box %+v", sub.Box)
	}
}

// An element that draws nothing has no box, and counts as missing.
func TestCutEmptyElement(t *testing.T) {
	svg := strings.Replace(coverSVG, ` d="M20 10 h60 v20 h-60 z"`, ``, 1)
	_, slots, err := Cut([]byte(svg), "title")
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 0 {
		t.Errorf("slots %v", slots)
	}
	if _, _, err := Cut([]byte("<html/>"), "title"); err == nil {
		t.Error("not an SVG: no error")
	}
}

// The sample template of issue #66: text placeholders styled by CSS
// classes and placed by a transform.
func TestCutTextPlaceholders(t *testing.T) {
	data, err := os.ReadFile("../../testdata/cover/sample.svg")
	if err != nil {
		t.Fatal(err)
	}
	rest, slots, err := Cut(data, "title", "subtitle")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rest, []byte("Lorem")) || bytes.Contains(rest, []byte("Dolor")) {
		t.Errorf("sample texts left in:\n%s", rest)
	}
	near := func(got, want float64) bool { return math.Abs(got-want) < 0.0005 }
	const w, h = 595.276, 841.89
	gold := color.RGBA{0xfa, 0xbb, 0x55, 0xff}
	for id, want := range map[string][2]float64{"title": {91.154 + 36, 36}, "subtitle": {249.805 + 15.5, 15.5}} {
		s, ok := slots[id]
		if !ok {
			t.Fatalf("no %s slot", id)
		}
		if !s.Text || !near(s.X, 172.123/w) || !near(s.Baseline, want[0]/h) || !near(s.Size, want[1]/h) {
			t.Errorf("%s: %+v", id, s)
		}
		if s.Fill != gold || s.Family != "Source Sans 3" || s.Anchor != "start" {
			t.Errorf("%s: fill %v, family %q, anchor %q", id, s.Fill, s.Family, s.Anchor)
		}
	}
	// The rest draws its class-styled shapes.
	img, err := Rasterize(rest, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(350, 600).RGBA(); r>>8 != 0xfa || g>>8 != 0xbb || b>>8 != 0x55 {
		t.Errorf("box not drawn: %x %x %x", r>>8, g>>8, b>>8)
	}
}
