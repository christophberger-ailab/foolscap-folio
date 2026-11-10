package web

import "testing"

// A passage is commented in the editor, signed, replied to, removed, and
// highlighted in the preview (spec-web.yaml 3-8). Both happen in the browser; what a Go test can check
// is that the scripts and the stylesheet still carry the pieces that do it.
func TestCommentsInEditorAndPreview(t *testing.T) {
	for file, wants := range map[string][]string{
		"assets/static/editor.js":  {"function addComment(", "function focusComment(", "'Ctrl-Alt-M': addComment", "mountCommentRail();", "function replyToComment(", "'Ctrl-Alt-R': replyToComment", "getAttribute('data-user')", "function commentMeta(instance)", "function removeComment(", "window.confirm(", "comment-remove"},
		"assets/static/preview.js": {"function parseCommentMarker(", "function markComments(", "markComments(el);", "marked.lexer(unblockComments(src))", "'<<+': 'reply'", "function threadReply(", "function commentReply(", "function commentRemoval("},
		"assets/static/app.js":     {"focusComment(Number("},
		"assets/static/app.css":    {"--comment:", ".comment-rail", ".comment-add", ".folio-comment", ".comment-remove"},
	} {
		src, err := assets.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !assetContains(file, string(src), want) {
				t.Errorf("%s does not handle comments, missing %q", file, want)
			}
		}
	}
}
