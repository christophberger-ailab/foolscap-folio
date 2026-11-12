package docrender

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
)

// imageLines lays out runs as one paragraph and returns each line, with
// inline images shown as [WxH], and the height the paragraph took.
func imageLines(t *testing.T, runs []Run, width float64) ([]string, float64) {
	t.Helper()
	w := testWriter(t)
	var out []string
	w.trace = func(line []piece) {
		var s strings.Builder
		for _, p := range line {
			if p.Image != "" {
				fmt.Fprintf(&s, "[%gx%g]", p.width, p.height)
				continue
			}
			s.WriteString(p.Text)
		}
		out = append(out, strings.TrimSpace(s.String()))
	}
	start := w.y
	if err := w.paragraph(runs, 11, margin, width); err != nil {
		t.Fatal(err)
	}
	return out, w.y - start
}

func TestInlineImageFlowsWithText(t *testing.T) {
	img := coverPNG(t) // 200x100
	runs := []Run{{Text: "Press "}, {Image: img, ImageHeight: "12pt"}, {Text: " to save."}}
	got, h := imageLines(t, runs, pageW-2*margin)
	if len(got) != 1 || got[0] != "Press [24x12] to save." {
		t.Fatalf("lines %q", got)
	}
	// 12pt is above the text's ascent, so the line grows by the difference.
	if want := 12 + 11*1.35 - 11*testWriter(t).fonts["regular"].ascent + 7; math.Abs(h-want) > 1e-9 {
		t.Errorf("height %v, want %v", h, want)
	}
}

func TestSmallInlineImageKeepsLineHeight(t *testing.T) {
	runs := []Run{{Text: "a "}, {Image: coverPNG(t), ImageWidth: "8px"}, {Text: " b"}}
	got, h := imageLines(t, runs, pageW-2*margin)
	if len(got) != 1 || got[0] != "a [6x3] b" {
		t.Fatalf("lines %q", got)
	}
	if want := 11*1.35 + 7; math.Abs(h-want) > 1e-9 {
		t.Errorf("height %v, want %v", h, want)
	}
}

func TestInlineImageWrapsLikeAWord(t *testing.T) {
	img := coverPNG(t)
	runs := []Run{{Text: "lead "}, {Image: img, ImageWidth: "40pt"}, {Text: "."}}
	got, _ := imageLines(t, runs, width(t, "lead ")+20)
	// The image and the period touching it move to the next line together.
	if len(got) != 2 || got[0] != "lead" || got[1] != "[40x20]." {
		t.Fatalf("lines %q", got)
	}
}

func TestSoleImageStaysABlock(t *testing.T) {
	w := testWriter(t)
	var traced int
	w.trace = func([]piece) { traced++ }
	start := w.y
	if err := w.paragraph([]Run{{Image: coverPNG(t), ImageHeight: "50pt"}}, 11, margin, pageW-2*margin); err != nil {
		t.Fatal(err)
	}
	if traced != 0 {
		t.Errorf("sole image laid out as a text line")
	}
	if want := 50.0 + 10 + 7; math.Abs(w.y-start-want) > 1e-9 {
		t.Errorf("height %v, want %v", w.y-start, want)
	}
}

// A Windows drive letter is not a URL scheme: the callout icons Quarto ships
// are found at C:/ProgramData/.../warning.png on a Windows agent.
func TestResourceDrivePath(t *testing.T) {
	for _, src := range []string{
		"C:/ProgramData/Chocolatey/lib/quarto/tools/share/formats/docx/warning.png",
		"d:/a/1/s/doc/Training/assets/callout-icons/caution.png",
		`C:\Program Files\Quarto\share\formats\docx\note.png`,
	} {
		got, err := resource("/project", src)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		if want := filepath.FromSlash(src); got != want {
			t.Errorf("%s: got %s, want %s", src, got, want)
		}
	}
}

func TestResourceKeepsRejectingURLs(t *testing.T) {
	for _, src := range []string{"https://example.com/a.png", "http://example.com/a.png", "data:image/png;base64,AAAA"} {
		if _, err := resource("/project", src); err == nil || !strings.Contains(err.Error(), "requires local image") {
			t.Errorf("%s: err %v, want \"requires local image\"", src, err)
		}
	}
}

func TestResourceRelativePaths(t *testing.T) {
	root := filepath.FromSlash("/project")
	for src, want := range map[string]string{
		"assets/a.png":     filepath.Join(root, "assets", "a.png"),
		"/assets/a.png":    filepath.Join(root, "assets", "a.png"),
		"my%20image.png":   filepath.Join(root, "my image.png"),
		"sub/dir/b%2B.png": filepath.Join(root, "sub", "dir", "b+.png"),
	} {
		got, err := resource(root, src)
		if err != nil || got != want {
			t.Errorf("%s: got %s, %v; want %s", src, got, err, want)
		}
	}
}

func TestFitImageDimensions(t *testing.T) {
	w := testWriter(t)
	img := coverPNG(t) // 200x100
	for _, c := range []struct {
		width, height string
		w, h          float64
	}{
		{"", "", 100, 50}, // 0.75pt per pixel, shrunk to the width
		{"100pt", "", 100, 50},
		{"", "30pt", 60, 30},
		{"100pt", "30pt", 100, 30},
		{"80", "", 60, 30},
		{"50%", "", 50, 25},
		{"", "2em", 22, 11},
		{"400pt", "", 100, 50}, // shrinks to fit, keeping its proportions
	} {
		_, iw, ih, err := w.fitImage(Run{Image: img, ImageWidth: c.width, ImageHeight: c.height}, 5.5, 100)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(iw-c.w) > 1e-9 || math.Abs(ih-c.h) > 1e-9 {
			t.Errorf("width %q height %q: %gx%g, want %gx%g", c.width, c.height, iw, ih, c.w, c.h)
		}
	}
}

func TestImageHeightAttribute(t *testing.T) {
	runs, err := inlines([]Node{{T: "Image", C: []byte(`[["",[],[["width","2cm"],["height","1cm"]]],[],["a.png",""]]`)}}, Run{})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ImageWidth != "2cm" || runs[0].ImageHeight != "1cm" {
		t.Fatalf("runs %+v", runs)
	}
}

// An SVG image is drawn, at its own size.
func TestFitImageSVG(t *testing.T) {
	w := testWriter(t)
	w.root = t.TempDir()
	os.WriteFile(filepath.Join(w.root, "a.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"><rect width="80" height="40" fill="red"/></svg>`), 0o644)
	key, iw, ih, err := w.fitImage(Run{Image: "/a.svg"}, 11, 400)
	if err != nil {
		t.Fatal(err)
	}
	if iw != 60 || ih != 30 {
		t.Errorf("size %gx%g, want 60x30", iw, ih)
	}
	if err := w.drawImage(key, 0, 0, iw, ih); err != nil {
		t.Fatal(err)
	}
}

// A missing image does not stop the PDF: the project's placeholder stands
// in for it, and one warning names it, however often it is measured.
func TestFitImageMissingShowsPlaceholder(t *testing.T) {
	w := testWriter(t)
	w.root = t.TempDir()
	var warned []string
	w.warn = func(f string, a ...any) { warned = append(warned, fmt.Sprintf(f, a...)) }
	for range 2 {
		key, iw, ih, err := w.fitImage(Run{Image: "/nope.png"}, 11, 400)
		if err != nil {
			t.Fatal(err)
		}
		if iw != 240 || ih != 135 { // the built-in placeholder, 320x180 px
			t.Errorf("size %gx%g, want 240x135", iw, ih)
		}
		if err := w.drawImage(key, 0, 0, iw, ih); err != nil {
			t.Fatal(err)
		}
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "nope.png") {
		t.Errorf("warnings %q, want one naming the image", warned)
	}
}

// An image alone in its paragraph has its alt text as a caption below it.
func TestSoleImageCaption(t *testing.T) {
	w := testWriter(t)
	var lines []string
	w.trace = func(line []piece) {
		var s strings.Builder
		for _, p := range line {
			s.WriteString(p.Text)
			if !p.Italic {
				t.Errorf("caption piece %q not italic", p.Text)
			}
		}
		lines = append(lines, strings.TrimSpace(s.String()))
	}
	start := w.y
	if err := w.paragraph([]Run{{Image: coverPNG(t), ImageHeight: "50pt", Caption: "A fine view"}}, 11, margin, pageW-2*margin); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != "A fine view" {
		t.Fatalf("caption lines %q", lines)
	}
	if w.y-start <= 50+10+7 {
		t.Errorf("height %v leaves no room for the caption", w.y-start)
	}
}

func TestMarkdownImageCaption(t *testing.T) {
	d := markdown.Parse([]byte("![A *fine* view](a.png)\n\nSee ![icon](i.png) here.\n"))
	bb, err := FromMarkdown(d)
	if err != nil {
		t.Fatal(err)
	}
	if got := bb[0].Runs[0].Caption; got != "A fine view" {
		t.Errorf("sole image caption %q", got)
	}
	if got := bb[1].Runs[1].Caption; got != "" {
		t.Errorf("inline image caption %q", got)
	}
}
