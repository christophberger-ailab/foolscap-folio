package foliocore

import "strings"

// SplitFrontMatter separates a leading YAML front-matter block from the body.
//
// A front-matter block must start on the very first line with `---` and end
// with a line of `---` or `...`. Returns the raw YAML (without the fences),
// the remaining body, and whether a block was found.
func SplitFrontMatter(src string) (yamlText, body string, found bool) {
	// Tolerate a UTF-8 BOM and leading blank lines before the opening fence.
	src = strings.TrimPrefix(src, "\uFEFF")

	lines := strings.Split(src, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start >= len(lines) || strings.TrimRight(lines[start], " \t\r") != "---" {
		return "", src, false
	}

	for i := start + 1; i < len(lines); i++ {
		trimmed := strings.TrimRight(lines[i], " \t\r")
		if trimmed == "---" || trimmed == "..." {
			yamlText = strings.Join(lines[start+1:i], "\n")
			body = strings.Join(lines[i+1:], "\n")
			return yamlText, body, true
		}
	}

	// Unterminated block: treat the whole file as body rather than guessing.
	return "", src, false
}
