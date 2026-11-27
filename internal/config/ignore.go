package config

import (
	"cmp"
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultIgnore is what folio, as Quarto, used to leave out of a project:
// every file and folder whose name starts with "_" or ".". A folio.yml
// that folio writes names it, so that a project without the setting
// leaves out nothing (issue #181).
var DefaultIgnore = []string{"_*", ".*"}

// Ignore says which paths of a project folio leaves out of renders,
// previews and folio web: those that match the project's ignore patterns,
// which follow the rules of .gitignore, and always the renders' output,
// _build and .git. The zero value leaves out _build and .git alone.
type Ignore struct {
	patterns []pattern
	output   string
}

// pattern is one line of a .gitignore: a path, split at "/", to match
// from the project root, a leading "**" for one that matches at any depth.
type pattern struct {
	segs     []string
	neg, dir bool
}

// NewIgnore reads the gitignore patterns lines; output is the directory
// the renders write below, project-relative.
func NewIgnore(lines []string, output string) (Ignore, error) {
	ig := Ignore{output: strings.Trim(path.Clean("/"+output), "/")}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || l[0] == '#' {
			continue
		}
		var p pattern
		if l[0] == '!' {
			p.neg, l = true, l[1:]
		}
		if strings.HasSuffix(l, "/") {
			p.dir, l = true, strings.TrimRight(l, "/")
		}
		if !strings.Contains(l, "/") {
			l = "**/" + l
		}
		p.segs = strings.Split(strings.TrimPrefix(l, "/"), "/")
		for _, s := range p.segs {
			if _, err := path.Match(s, ""); err != nil {
				return Ignore{}, fmt.Errorf("ignore: bad pattern %q", l)
			}
		}
		ig.patterns = append(ig.patterns, p)
	}
	return ig, nil
}

// ReadIgnore is the Ignore of the project at root, read from its
// configuration file alone; it leaves out only what is always left out
// when the file has no valid setting.
func ReadIgnore(root string) Ignore {
	var c struct {
		Ignore []string `yaml:"ignore"`
		Output string   `yaml:"output"`
	}
	if p := Find(root); p != "" {
		if data, err := os.ReadFile(p); err == nil {
			_ = yaml.Unmarshal(data, &c)
		}
	}
	ig, err := NewIgnore(c.Ignore, cmp.Or(c.Output, "_output"))
	if err != nil {
		ig, _ = NewIgnore(nil, cmp.Or(c.Output, "_output"))
	}
	return ig
}

// Match reports whether the path rel, slash-separated and relative to the
// project root, is left out; dir says whether it is a directory. As in
// .gitignore, what lies in a folder that is left out is left out too, and
// the last pattern that matches decides.
func (ig Ignore) Match(rel string, dir bool) bool {
	rel = strings.Trim(path.Clean("/"+rel), "/")
	if rel == "" {
		return false
	}
	if rel == "_build" || strings.HasPrefix(rel, "_build/") ||
		ig.output != "" && (rel == ig.output || strings.HasPrefix(rel, ig.output+"/")) {
		return true
	}
	parts := strings.Split(rel, "/")
	for i := range parts {
		if parts[i] == ".git" || ig.matches(parts[:i+1], dir || i < len(parts)-1) {
			return true
		}
	}
	return false
}

// matches reports whether the patterns leave out the path parts.
func (ig Ignore) matches(parts []string, dir bool) bool {
	out := false
	for _, p := range ig.patterns {
		if (dir || !p.dir) && matchSegs(p.segs, parts) {
			out = !p.neg
		}
	}
	return out
}

// matchSegs reports whether the pattern segments pat match the path
// segments name, "**" any number of them. It takes time in proportion to
// len(pat)*len(name), however many "**" a pattern has: folio.yml may come
// from a repository someone else wrote.
func matchSegs(pat, name []string) bool {
	m := make([]bool, len(name)+1) // m[j]: the segments so far match name[:j]
	m[0] = true
	for _, p := range pat {
		next := make([]bool, len(name)+1)
		for j := range next {
			switch {
			case p == "**":
				next[j] = m[j] || j > 0 && next[j-1]
			case j > 0 && m[j-1]:
				next[j], _ = path.Match(p, name[j-1])
			}
		}
		m = next
	}
	return m[len(name)]
}
