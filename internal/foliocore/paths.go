package foliocore

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ProjectPath joins a project-relative path from a profile onto docRoot and
// refuses to leave the project.
func ProjectPath(docRoot, rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("%q must be a project-relative path", rel)
	}
	abs := filepath.Join(docRoot, clean)
	inside, err := filepath.Rel(docRoot, abs)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q points outside the project", rel)
	}
	return abs, nil
}
