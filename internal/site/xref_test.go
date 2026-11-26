package site

import (
	"path/filepath"
	"testing"
)

// References link across the pages of the site, and the numbers come from
// the topic's book: chapters through the topic, headings below them, and
// the figures through the topic.
func TestCrossReferences(t *testing.T) {
	root, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	write(t, root, "d/index.md", "---\ntitle: D\n---\n\n# D\n")
	write(t, root, "d/part/index.md", "---\ntitle: Part\norder: 1\n---\n\n# Part\n")
	write(t, root, "d/part/one.md", "---\ntitle: One\norder: 1\n---\n\n# One\n\n## Overview\n\n![A Map](/m.png)\n\n"+
		"See [](#two:overview), [](#a-map), [this](#overview) and [](#the-map){.number}.\n")
	write(t, root, "d/part/two.md", "---\ntitle: Two\norder: 2\nid: two\n---\n\n# Two\n\n## Overview\n\n![The Map](/m.png)\n\nBack to [](#one).\n")
	write(t, root, "m.png", "PNG")
	o := Options{Root: root, OutDir: out, Topics: []Topic{{Name: "d", Folder: "d"}},
		NumberedHeadings: true, FigureLabel: "Figure"}
	if err := Build(o); err != nil {
		t.Fatal(err)
	}
	one := read(t, out, "d/part/one.html")
	has(t, "one", one, `<h2 id="overview">1.1 Overview</h2>`, "<figcaption>Figure 1: A Map</figcaption>",
		`See <a href="two.html#overview">2.1, Overview</a>, <a href="#a-map">Figure 1, A Map</a>, <a href="#overview">this</a> and <a href="two.html#the-map">Figure 2</a>.`)
	two := read(t, out, "d/part/two.html")
	has(t, "two", two, "<figcaption>Figure 2: The Map</figcaption>", `Back to <a href="one.html#bm-d-part-one">1, One</a>.`)
}
