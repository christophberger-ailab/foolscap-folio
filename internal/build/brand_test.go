package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

const brandYML = `color:
  primary: "#003366"
typography:
  fonts:
    - family: Corporate Sans
      source: file
      files:
        - path: fonts/Sans-Regular.ttf
          weight: 400
        - path: fonts/Sans-Medium.ttf
          weight: medium
        - path: fonts/Sans-Bold.ttf
          weight: bold
        - path: fonts/Sans-Italic.ttf
          style: italic
        - fonts/Sans.woff2
    - family: Corporate Mono
      source: file
      files:
        - fonts/Mono.ttf
    - family: Fancy Display
      source: google
  base: Corporate Sans
  headings:
    family: Corporate Sans
    weight: 500
  monospace: Corporate Mono
`

func brandProject(t *testing.T, yml string) string {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "fonts"), 0o755)
	for name, data := range map[string][]byte{
		"Sans-Regular.ttf": goregular.TTF, "Sans-Medium.ttf": gomedium.TTF, "Sans-Bold.ttf": gobold.TTF,
		"Sans-Italic.ttf": goitalic.TTF, "Sans.woff2": []byte("wOF2"), "Mono.ttf": gomono.TTF,
	} {
		os.WriteFile(filepath.Join(root, "fonts", name), data, 0o644)
	}
	os.WriteFile(filepath.Join(root, BrandFile), []byte(yml), 0o644)
	return root
}

// brandFonts reads the PDF fonts of the brand file at root.
func brandFonts(root string) (map[string]string, []string, error) {
	b, err := loadBrand(root, "")
	if err != nil {
		return nil, nil, err
	}
	return b.fonts()
}

func TestBrandFonts(t *testing.T) {
	root := brandProject(t, brandYML)
	paths, notes, err := brandFonts(root)
	if err != nil {
		t.Fatal(err)
	}
	font := func(name string) string { return filepath.Join(root, "fonts", name) }
	want := map[string]string{
		"regular": font("Sans-Regular.ttf"), "bold": font("Sans-Bold.ttf"),
		"italic": font("Sans-Italic.ttf"), "bolditalic": font("Sans-Italic.ttf"),
		"heading": font("Sans-Medium.ttf"), "mono": font("Mono.ttf"),
	}
	for role, p := range want {
		if paths[role] != p {
			t.Errorf("%s: %q, want %q", role, paths[role], p)
		}
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "Sans.woff2") {
		t.Errorf("notes %q", notes)
	}

	// Families the PDF cannot embed keep the Go fonts, with a note.
	yml := strings.Replace(brandYML, "  base: Corporate Sans\n", "  base: Fancy Display\n", 1)
	yml = strings.Replace(yml, "    family: Corporate Sans\n", "    family: Unknown\n", 1)
	paths, notes, err = brandFonts(brandProject(t, yml))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths["mono"] == "" {
		t.Errorf("paths %v, want mono only", paths)
	}
	if len(notes) != 3 || !strings.Contains(notes[1], "Fancy Display comes from google") ||
		!strings.Contains(notes[2], "Unknown has no TrueType files") {
		t.Errorf("notes %q", notes)
	}

	if paths, notes, err = brandFonts(t.TempDir()); paths != nil || notes != nil || err != nil {
		t.Errorf("no brand file: %v, %v, %v", paths, notes, err)
	}
	for name, yml := range map[string]string{
		"missing file": strings.Replace(brandYML, "Mono.ttf", "Missing.ttf", 1),
		"outside":      strings.Replace(brandYML, "fonts/Mono.ttf", "../Mono.ttf", 1),
		"weight":       strings.Replace(brandYML, "weight: 500", "weight: heavyish", 1),
		"yaml":         "typography: [",
	} {
		if _, _, err := brandFonts(brandProject(t, yml)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

const brandStyleYML = `color:
  palette:
    navy: "#003366"
    paper: "#fffdf8"
  foreground: "#222"
  background: paper
  primary: navy
typography:
  fonts:
    - family: Corporate Sans
      source: file
      files:
        - path: fonts/Sans-Regular.ttf
          weight: 400
        - path: fonts/Sans-Italic.ttf
          style: italic
        - fonts/Sans.woff2
    - family: Fancy Display
      source: google
    - family: Bunny Mono
      source: bunny
  base:
    family: Corporate Sans
    size: 17px
    line-height: 1.5
  headings:
    family: Fancy Display
    weight: 600
    color: primary
  monospace: Bunny Mono
  monospace-inline:
    color: "#aa0000"
    background-color: "#eee"
  link:
    color: "#0055aa"
    decoration: none
`

func TestBrandCSS(t *testing.T) {
	root := brandProject(t, brandStyleYML)
	b, err := loadBrand(root, "")
	if err != nil {
		t.Fatal(err)
	}
	css, files, notes := b.css()
	if len(notes) != 0 {
		t.Errorf("notes %q", notes)
	}
	for _, want := range []string{
		`@import url("https://fonts.googleapis.com/css?family=Fancy+Display:400,400italic,600,600italic,700,700italic&display=swap");`,
		`@import url("https://fonts.bunny.net/css?family=bunny-mono:400,400i,700,700i&display=swap");`,
		`@font-face{font-family:"Corporate Sans";src:url("fonts/1-Sans-Regular.ttf");font-weight:400;font-style:normal}`,
		`@font-face{font-family:"Corporate Sans";src:url("fonts/2-Sans-Italic.ttf");font-weight:400;font-style:italic}`,
		`src:url("fonts/3-Sans.woff2")`,
		`--font:"Corporate Sans",system-ui,sans-serif`, `--mono:"Bunny Mono",ui-monospace,monospace`, `--lh-body:1.5`,
		`html{font-size:17px}`,
		`h1,h2,h3,h4,h5,h6{font-family:"Fancy Display",var(--font);font-weight:600}`,
		`a{text-decoration:none}`,
		`@media (prefers-color-scheme: light){`,
		`--bg:#fffdf8`, `--nav-bg:#fffdf8`, `--fg:#222`, `--accent:#003366`, `--link:#0055aa`,
		`h1,h2,h3,h4,h5,h6{color:#003366}`,
		`:not(pre)>code{color:#aa0000;background-color:#eee}`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("css lacks %s\n%s", want, css)
		}
	}
	if strings.Index(css, "@import") != 0 {
		t.Error("the imports do not come first")
	}
	if len(files) != 3 || files["1-Sans-Regular.ttf"] != filepath.Join(root, "fonts", "Sans-Regular.ttf") {
		t.Errorf("files %v", files)
	}

	// The documents take the colors and the names of the fonts.
	st := b.style()
	want := brandLook{Font: "Corporate Sans", HeadingFont: "Fancy Display", MonoFont: "Bunny Mono",
		Color: "222222", HeadingColor: "003366", LinkColor: "0055AA"}
	if st != want {
		t.Errorf("style %+v, want %+v", st, want)
	}

	// Values that would break out of the stylesheet are left out, with a note.
	bad := strings.Replace(brandStyleYML, `foreground: "#222"`, `foreground: "red;} body{display:none"`, 1)
	bad = strings.Replace(bad, "monospace: Bunny Mono", `monospace: 'x"; } *{color:red'`, 1)
	b, err = loadBrand(brandProject(t, bad), "")
	if err != nil {
		t.Fatal(err)
	}
	css, _, notes = b.css()
	if strings.Contains(css, "display:none") || strings.Contains(css, "*{color") || len(notes) != 2 {
		t.Errorf("unsafe values: notes %q\n%s", notes, css)
	}

	// Without a brand file there is nothing to add.
	var none *brand
	if css, files, notes = none.css(); css != "" || files != nil || notes != nil {
		t.Errorf("no brand: %q %v %v", css, files, notes)
	}
	if none.style() != (brandLook{}) {
		t.Error("no brand: a style")
	}
}

// A project without a brand file of its own takes the user's.
func TestUserBrand(t *testing.T) {
	user := brandProject(t, brandYML)
	userFile := filepath.Join(user, BrandFile)
	b, err := loadBrand(t.TempDir(), userFile)
	if err != nil || b == nil {
		t.Fatalf("user brand: %v, %v", b, err)
	}
	// Its font files are found beside it, not in the project.
	paths, _, err := b.fonts()
	if err != nil || paths["regular"] != filepath.Join(user, "fonts", "Sans-Regular.ttf") {
		t.Errorf("fonts %v, %v", paths, err)
	}
	// The project's own file wins.
	own := brandProject(t, "color:\n  primary: \"#112233\"\n")
	if b, err = loadBrand(own, userFile); err != nil || b.Color.Primary != "#112233" {
		t.Errorf("project brand: %+v, %v", b, err)
	}
	// Neither: no brand.
	if b, err = loadBrand(t.TempDir(), filepath.Join(t.TempDir(), BrandFile)); b != nil || err != nil {
		t.Errorf("no brand: %v, %v", b, err)
	}
	if b, err = loadBrand(t.TempDir(), ""); b != nil || err != nil {
		t.Errorf("no brand: %v, %v", b, err)
	}
}
