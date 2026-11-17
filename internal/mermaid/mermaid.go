// Package mermaid draws Mermaid diagrams as SVG with pele-go, and makes
// that SVG drawable by resvg for the writers that embed it as a picture.
package mermaid

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"

	pele "github.com/Broderick-Westrope/pele-go"
	_ "github.com/Broderick-Westrope/pele-go/all" // every diagram type
	"github.com/Broderick-Westrope/pele-go/fontmeasure"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
)

// measurer measures labels in the Go fonts, which resvg draws them in, so
// they fit their boxes in the PDF and DOCX; browsers' sans-serif fonts are
// narrower, if anything.
var measurer = sync.OnceValues(func() (pele.TextMeasurer, error) {
	return fontmeasure.New(goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF,
		fontmeasure.WithMono(gomono.TTF, gomonobold.TTF, gomonoitalic.TTF, gomonobolditalic.TTF))
})

// Render draws Mermaid source as SVG. The ids in it begin with idPrefix,
// so that several diagrams can share a page. Images in the diagram are
// dropped: a browser showing it would fetch them.
func Render(src, idPrefix string) (string, error) {
	m, err := measurer()
	if err != nil {
		return "", err
	}
	res, err := pele.Render(context.Background(), src, pele.Options{
		Measurer: m,
		IDPrefix: pele.Ptr(idPrefix),
		Images:   pele.Ptr(false),
	})
	return res.SVG, err
}

var rootFontSize = regexp.MustCompile(`^<svg\b[^>]*?\sstyle="[^"]*\bfont:\s*(\d+(?:\.\d+)?)px`)

// Static turns SVG from Render into SVG that resvg draws as a browser
// would: the CSS variables filled in, and the root's font size, which
// resvg does not read from the font shorthand, given as an attribute.
func Static(svg string) (string, error) {
	s, err := resolve(svg)
	if err != nil {
		return "", err
	}
	size := "16"
	if m := rootFontSize.FindStringSubmatch(svg); m != nil {
		size = m[1]
	}
	return strings.Replace(s, "<svg ", `<svg font-size="`+size+`" `, 1), nil
}

var (
	tag       = regexp.MustCompile(`<[^<>]*>`)
	attrValue = regexp.MustCompile(`"[^"]*"`)
	rootStyle = regexp.MustCompile(`^<svg\b[^>]*?\sstyle="([^"]*)"`)

	errTooLarge = errors.New("mermaid: CSS variables expand too large")
	errTooDeep  = errors.New("mermaid: CSS variables nest too deep")
)

// resolve substitutes every var() in the attribute values of svg with
// the custom properties declared in the root element's style, where Pele
// declares them, or the fallback. Text is left alone: it is the diagram's
// labels. A variable that refers back to itself falls back as if it were
// not declared.
func resolve(svg string) (string, error) {
	r := resolver{defs: map[string]string{}, done: map[string]string{}, busy: map[string]bool{},
		limit: max(4*len(svg), 1<<20)}
	if m := rootStyle.FindStringSubmatch(svg); m != nil {
		for decl := range strings.SplitSeq(m[1], ";") {
			name, value, ok := strings.Cut(decl, ":")
			if name = strings.TrimSpace(name); ok && strings.HasPrefix(name, "--") {
				r.defs[name] = strings.TrimSpace(value)
			}
		}
	}
	var err error
	out := tag.ReplaceAllStringFunc(svg, func(t string) string {
		return attrValue.ReplaceAllStringFunc(t, func(v string) string {
			s, e := r.expand(v)
			if err == nil {
				err = e
			}
			return s
		})
	})
	if err == nil && len(out) > r.limit {
		err = errTooLarge
	}
	return out, err
}

type resolver struct {
	defs, done map[string]string
	busy       map[string]bool
	limit      int
}

func (r *resolver) expand(s string) (string, error) {
	var b strings.Builder
	for {
		before, after, found := strings.Cut(s, "var(")
		body, rest, closed := matchParen(after)
		if !found || !closed {
			b.WriteString(s)
			return b.String(), nil
		}
		b.WriteString(before)
		name, fallback, _ := strings.Cut(body, ",")
		v, err := r.lookup(strings.TrimSpace(name), strings.TrimSpace(fallback))
		if err != nil {
			return "", err
		}
		if b.WriteString(v); b.Len() > r.limit {
			return "", errTooLarge
		}
		s = rest
	}
}

func (r *resolver) lookup(name, fallback string) (string, error) {
	if v, ok := r.done[name]; ok {
		return v, nil
	}
	def, ok := r.defs[name]
	if !ok || r.busy[name] {
		return r.expand(fallback)
	}
	if len(r.busy) >= 32 {
		return "", errTooDeep
	}
	r.busy[name] = true
	v, err := r.expand(def)
	delete(r.busy, name)
	r.done[name] = v
	return v, err
}

// matchParen splits s at the parenthesis that closes one already open.
func matchParen(s string) (body, rest string, ok bool) {
	depth := 1
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			if depth--; depth == 0 {
				return s[:i], s[i+1:], true
			}
		}
	}
	return "", "", false
}
