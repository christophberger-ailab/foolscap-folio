package web

import "testing"

// In Vim mode, Ctrl-C with text selected copies it rather than being Vim's
// Esc (spec-web.yaml 3-6-8). The copy happens in the browser; what a Go test
// can check is that editor.js still carries the handler that lets it through.
func TestVimCtrlCCopiesSelection(t *testing.T) {
	file := "assets/static/editor.js"
	src, err := assets.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"function letVimCopy(", "cm.on('keydown', letVimCopy);", "e.codemirrorIgnore = true", "cm.on('copy', leaveVisualAfterCopy);"} {
		if !assetContains(file, string(src), want) {
			t.Errorf("%s does not let Ctrl-C copy in Vim mode, missing %q", file, want)
		}
	}
}
