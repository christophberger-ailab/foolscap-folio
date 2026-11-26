package markdown

import (
	"html"
	"strings"

	"github.com/yuin/goldmark/ast"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// FigureCaption reports whether a paragraph is a figure -- one image and
// nothing else but blank text, the way Pandoc reads an implicit figure --
// and returns its caption: the image's alt text, after its label ("Figure
// 3: ") when NumberFigures gave it one. An image without alt text has no
// caption, so its paragraph is no figure.
func FigureCaption(n ast.Node, src []byte) (string, bool) {
	img := figureImage(n, src)
	if img == nil {
		return "", false
	}
	caption := strings.TrimSpace(PlainText(img, src))
	if caption == "" {
		return "", false
	}
	if l := FigureLabel(img); l != "" {
		caption = l + ": " + caption
	}
	return caption, true
}

// figureImage returns the image of a paragraph that holds one image and
// nothing else but blank text, nil for any other node.
func figureImage(n ast.Node, src []byte) *ast.Image {
	if _, ok := n.(*ast.Paragraph); !ok {
		return nil
	}
	var img *ast.Image
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		switch ch := ch.(type) {
		case *ast.Image:
			if img != nil {
				return nil
			}
			img = ch
		case *ast.Text:
			if strings.TrimSpace(string(ch.Segment.Value(src))) != "" {
				return nil
			}
		default:
			return nil
		}
	}
	return img
}

// renderParagraph writes a figure paragraph as a figure with its caption
// below the image, and any other paragraph as goldmark does.
func renderParagraph(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	caption, figure := FigureCaption(n, src)
	switch {
	case figure && entering:
		w.WriteString("<figure>")
	case figure:
		w.WriteString("<figcaption>" + html.EscapeString(caption) + "</figcaption></figure>\n")
	case entering && n.Attributes() != nil:
		w.WriteString("<p")
		ghtml.RenderAttributes(w, n, ghtml.ParagraphAttributeFilter)
		w.WriteByte('>')
	case entering:
		w.WriteString("<p>")
	default:
		w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}
