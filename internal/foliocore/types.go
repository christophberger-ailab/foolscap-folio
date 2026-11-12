// Package foliocore contains small helpers shared by the folio commands:
// reading front matter, scanning a folder's pages, and resolving
// project-relative paths.
package foliocore

import (
	"path/filepath"
	"regexp"
)

// FileEntry holds metadata for a single .qmd/.md file.
type FileEntry struct {
	RelPath string // relative to doc root, forward-slash separated
	Order   *int
}

// Frontmatter is the YAML front matter we read from .qmd/.md files.
type Frontmatter struct {
	Order *int   `yaml:"order"`
	Title string `yaml:"title"`
}

// DefaultExcludePattern matches basenames starting with `_` or `.` (FILES.3).
const DefaultExcludePattern = `^[._]`

// ShouldExcludeChapter returns true when the file's basename matches the
// exclusion regex. A nil pattern disables filtering.
func ShouldExcludeChapter(relPath string, pattern *regexp.Regexp) bool {
	if pattern == nil {
		return false
	}
	base := filepath.Base(filepath.ToSlash(relPath))
	return pattern.MatchString(base)
}
