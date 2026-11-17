package mermaid

import (
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/svgimage"
)

const flow = "flowchart LR\n  A[Mermaid] --> B[SVG]\n"

func TestRender(t *testing.T) {
	svg, err := Render(flow+"  accTitle: Pipeline\n", "d1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(svg, "<svg") || !strings.Contains(svg, "Mermaid") {
		t.Errorf("not the diagram:\n%s", svg)
	}
	if !strings.Contains(svg, `id="d1-title"`) {
		t.Errorf("ids do not carry the prefix:\n%s", svg)
	}
}

func TestRenderError(t *testing.T) {
	if _, err := Render("flowchart LR\n  A -->", "d1"); err == nil {
		t.Error("broken source rendered")
	}
}

// Static SVG has no CSS variables left, which resvg cannot resolve, and
// gives the root its font size, which resvg does not take from the font
// shorthand.
func TestStatic(t *testing.T) {
	svg, err := Render(flow, "d1")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Static(svg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s, "var(") {
		t.Errorf("variables left:\n%s", s)
	}
	if !strings.HasPrefix(s, `<svg font-size="16" `) {
		t.Errorf("root has no font size:\n%.200s", s)
	}
	if !strings.Contains(s, "#1f1f1f") {
		t.Errorf("default colors not filled in:\n%s", s)
	}
	if _, err := svgimage.PNG([]byte(s), 1); err != nil {
		t.Errorf("resvg cannot draw it: %v", err)
	}
}

func TestResolve(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`<svg style="--a:#fff;--b:var(--a)"><rect fill="var(--b)"/></svg>`, `<svg style="--a:#fff;--b:#fff"><rect fill="#fff"/></svg>`},
		{`<svg><rect fill="var(--x,red)"/></svg>`, `<svg><rect fill="red"/></svg>`},
		{`<svg><rect fill="var(--x, var(--y, blue))"/></svg>`, `<svg><rect fill="blue"/></svg>`},
		{`<svg style="--a:var(--a)"><rect fill="var(--a,green)"/></svg>`, `<svg style="--a:"><rect fill=""/></svg>`},
		{`<svg><text>var(--x,red)</text></svg>`, `<svg><text>var(--x,red)</text></svg>`},
	} {
		got, err := resolve(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
		} else if got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.in, got, c.want)
		}
	}
}

// Variables that refer to each other many times over cannot blow the SVG
// up.
func TestResolveBounded(t *testing.T) {
	style := "--a:" + strings.Repeat("x", 32)
	for c := 'b'; c <= 'k'; c++ {
		style += ";--" + string(c) + ":" + strings.Repeat("var(--"+string(c-1)+")", 4)
	}
	if _, err := resolve(`<svg style="` + style + `"><rect fill="var(--k)"/></svg>`); err == nil {
		t.Error("expanded without bound")
	}
}
