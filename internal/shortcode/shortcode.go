// Package shortcode resolves the Quarto shortcodes folio's content uses, on
// the text, before the Markdown is parsed.
package shortcode

import (
	"fmt"
	"maps"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/christophberger-ailab/foolscap-folio/internal/foliocore"
)

// The shortcodes:
//
//	{{< include path/file.md >}}    the file's text, in place
//	{{< pagebreak >}}               a page break (see the page-break marker below)
//	{{< var name >}}                a profile variable or a `_variables.yml` value
//	{{name}}                        the same, outside inline code spans
//
// An include path starting with '/' is project-absolute; any other path
// is relative to the folder of the file holding the include. Text passed to
// Expand has no file of its own, so its relative paths start at the project
// root — the page loader makes a page's own includes project-absolute
// before its text gets here.
//
// Includes and page breaks are whole-line shortcodes, which is how the
// content writes them; `var` may sit anywhere in a line. Text inside fenced
// code blocks is left alone. Any other shortcode is reported once and left
// in the text, where it shows up as literal braces.

// pagebreakDiv is the marker a page-break shortcode becomes: an empty div
// that every writer turns into its own page break.
const pagebreakDiv = "\n\n::: {.folio-pagebreak}\n:::\n\n"

// MaxIncludeDepth stops an include that includes itself, however indirectly.
const MaxIncludeDepth = 8

var (
	// shortcodeLine matches a line that holds one shortcode and nothing else.
	shortcodeLine = regexp.MustCompile(`^\s*\{\{<\s*([A-Za-z][\w-]*)\s*(.*?)\s*>\}\}\s*$`)
	// shortcodeAny finds shortcodes anywhere in a line, for the report.
	shortcodeAny = regexp.MustCompile(`\{\{<\s*([A-Za-z][\w-]*)`)
	// escapedShortcode is Quarto's way of writing a shortcode literally.
	escapedShortcode = regexp.MustCompile(`\{\{\{<(.*?)>\}\}\}`)
)

// Expander resolves shortcodes in text. It is configured once per render
// and collects what it found for the log.
type Expander struct {
	root string
	vars map[string]string
	warn func(format string, args ...any)

	warned   map[string]bool
	includes int
}

// New returns an Expander for the project at root. Includes are read
// relative to root; `var` resolves against vars; warn receives the
// problems that do not stop a render.
func New(root string, vars map[string]string, warn func(string, ...any)) *Expander {
	if warn == nil {
		warn = func(string, ...any) {}
	}
	return &Expander{root: root, vars: vars, warn: warn, warned: map[string]bool{}}
}

// Expand returns text with its shortcodes resolved.
func (s *Expander) Expand(text string) (string, error) {
	return s.expand(text, ".", 0, nil)
}

// expand resolves the shortcodes of text that lives in dir, a
// project-relative, slash-separated folder.
func (s *Expander) expand(text, dir string, depth int, stack []string) (string, error) {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	var fence codeFence
	for _, line := range lines {
		if fence.step(line) {
			out = append(out, line)
			continue
		}
		if m := shortcodeLine.FindStringSubmatch(line); m != nil {
			replaced, handled, err := s.wholeLine(m[1], m[2], dir, depth, stack)
			if err != nil {
				return "", err
			}
			if handled {
				out = append(out, replaced)
				continue
			}
		}
		out = append(out, s.inline(line))
	}
	return strings.Join(out, "\n"), nil
}

// wholeLine resolves a line that is one shortcode. handled is false for a
// shortcode it does not know; the caller then treats the line as text.
func (s *Expander) wholeLine(name, arg, dir string, depth int, stack []string) (string, bool, error) {
	switch name {
	case "pagebreak":
		return pagebreakDiv, true, nil
	case "include":
		text, err := s.include(arg, dir, depth, stack)
		return text, true, err
	}
	return "", false, nil
}

func (s *Expander) include(arg, dir string, depth int, stack []string) (string, error) {
	path := strings.Trim(strings.TrimSpace(arg), `"'`)
	if path == "" {
		return "", fmt.Errorf("{{< include >}} names no file")
	}
	rel := ResolveInclude(dir, path)
	abs, err := foliocore.ProjectPath(s.root, rel)
	if err != nil {
		return "", fmt.Errorf("{{< include %s >}}: %w", path, err)
	}
	for _, seen := range stack {
		if seen == abs {
			return "", fmt.Errorf("{{< include %s >}} includes itself", path)
		}
	}
	if depth >= MaxIncludeDepth {
		return "", fmt.Errorf("{{< include %s >}}: includes are nested more than %d deep", path, MaxIncludeDepth)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("{{< include %s >}}: %w", path, err)
	}
	s.includes++
	// Like Quarto, drop the included file's own front matter.
	_, body, _ := foliocore.SplitFrontMatter(string(data))
	text, err := s.expand(strings.TrimRight(body, "\n"), pathpkg.Dir(rel), depth+1, append(stack, abs))
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return text, nil
}

// ResolveInclude returns the project-relative path an include path names
// when it is written in a file in dir (project-relative, slash-separated).
// A path starting with '/' is project-absolute. The result may start with
// "../" when the path leaves the project.
func ResolveInclude(dir, path string) string {
	if strings.HasPrefix(path, "/") {
		return pathpkg.Clean(strings.TrimPrefix(path, "/"))
	}
	return pathpkg.Join(dir, path)
}

// inline resolves the shortcodes that may sit in the middle of a line.
func (s *Expander) inline(line string) string {
	if !strings.Contains(line, "{{") {
		return line
	}
	line = escapedShortcode.ReplaceAllString(line, "{{<$1>}}")
	line = varRef.ReplaceAllStringFunc(line, func(m string) string {
		name := varRef.FindStringSubmatch(m)[1]
		if v, ok := s.vars[name]; ok {
			return v
		}
		s.reportOnce("var "+name, "undefined variable %q in {{< var %s >}}; it is left as written", name, name)
		return m
	})
	line = outsideCode(line, func(text string) string {
		return nameRef.ReplaceAllStringFunc(text, func(m string) string {
			name := nameRef.FindStringSubmatch(m)[1]
			if v, ok := s.vars[name]; ok {
				return v
			}
			s.reportOnce("var "+name, "undefined variable %q in {{%s}}; it is left as written", name, name)
			return m
		})
	})
	for _, m := range shortcodeAny.FindAllStringSubmatch(line, -1) {
		if m[1] == "var" || m[1] == "include" || m[1] == "pagebreak" {
			continue
		}
		s.reportOnce(m[1], "the {{< %s >}} shortcode is not supported; it stays in the text as written", m[1])
	}
	return line
}

func (s *Expander) reportOnce(key, format string, args ...any) {
	if s.warned[key] {
		return
	}
	s.warned[key] = true
	s.warn(format, args...)
}

// varRef matches a `{{< var name >}}` shortcode, nameRef the short form
// `{{name}}`.
var (
	varRef  = regexp.MustCompile(`\{\{<\s*var\s+([^\s>]+)\s*>\}\}`)
	nameRef = regexp.MustCompile(`\{\{\s*([A-Za-z][\w.-]*)\s*\}\}`)
)

// outsideCode returns line with f applied to the text between its inline
// code spans, which stay as written. A span opens with a run of backticks
// and closes with the next run of the same length; a run without one is
// text.
func outsideCode(line string, f func(string) string) string {
	var b strings.Builder
	text := 0 // start of the text not yet written
	for i := 0; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		n := 1
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		end := closingRun(line[i+n:], n)
		if end < 0 {
			i += n
			continue
		}
		end += i + n + n
		b.WriteString(f(line[text:i]))
		b.WriteString(line[i:end])
		i, text = end, end
	}
	b.WriteString(f(line[text:]))
	return b.String()
}

// closingRun returns the index in s of the first run of exactly n
// backticks, or -1.
func closingRun(s string, n int) int {
	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] == '`' {
			j++
		}
		if j-i == n {
			return i
		}
		i = j
	}
	return -1
}

// codeFence tracks whether a line belongs to a fenced code block.
type codeFence struct {
	open  bool
	char  byte
	width int
}

// step feeds one line and reports whether it is part of a fenced block: its
// fences and everything between them.
func (f *codeFence) step(line string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) >= 4 && !f.open {
		return false // an indented code block is not a fence
	}
	if !f.open {
		c := byte(0)
		if strings.HasPrefix(trimmed, "```") {
			c = '`'
		} else if strings.HasPrefix(trimmed, "~~~") {
			c = '~'
		}
		if c == 0 {
			return false
		}
		n := 0
		for n < len(trimmed) && trimmed[n] == c {
			n++
		}
		f.open, f.char, f.width = true, c, n
		return true
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == f.char {
		n++
	}
	if n >= f.width && strings.TrimSpace(trimmed[n:]) == "" {
		f.open = false
	}
	return true
}

// Includes is the number of files included.
func (s *Expander) Includes() int { return s.includes }

// Variables is what `{{< var >}}` resolves against: vars, overlaid with
// the project's `_variables.yml`, whose nested keys are addressed with
// dots (`metadata.version`), overlaid in turn with each of over. Its date
// is the date they give, else today, in their date_format (see
// FormatDate) and the language lang. A `_variables.yml` that is not YAML
// is an error; the variables are returned without it.
func Variables(root, lang string, vars map[string]string, over ...map[string]string) (map[string]string, error) {
	out := maps.Clone(vars)
	if out == nil {
		out = map[string]string{}
	}
	var err error
	for _, name := range []string{"_variables.yml", "_variables.yaml"} {
		data, rerr := os.ReadFile(filepath.Join(root, name))
		if rerr != nil {
			continue
		}
		var doc map[string]any
		if err = yaml.Unmarshal(data, &doc); err != nil {
			err = fmt.Errorf("%s: %w", name, err)
		}
		Flatten(doc, out)
		break
	}
	for _, m := range over {
		maps.Copy(out, m)
	}
	out["date"] = date(out, lang)
	return out, err
}

// Flatten adds the scalars of doc to out, nested keys joined with dots.
func Flatten(doc map[string]any, out map[string]string) { flatten("", doc, out) }

func flatten(prefix string, v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			flatten(p, t[k], out)
		}
	case nil:
	case time.Time: // an unquoted YAML date
		if prefix != "" {
			out[prefix] = t.Format(time.DateOnly)
		}
	default:
		if prefix != "" {
			out[prefix] = fmt.Sprint(t)
		}
	}
}
