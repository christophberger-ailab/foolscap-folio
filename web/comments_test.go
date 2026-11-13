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

// Comments are edited and replied to in the Comments pane (spec-web.yaml
// 14-8-2 to 14-8-5). As above, a Go test can only check that the pieces
// are there.
func TestCommentsEditableInPane(t *testing.T) {
	for file, wants := range map[string][]string{
		"assets/static/editor.js": {"function commentMarkerAt(", "function commentLine(", "function editComment(", "function writeReply(", "marker.text !== was"},
		"assets/static/app.js":    {"function startCommentEdit(", "function saveCommentEdit(", "author === commentUser()", "comments-edit", "editComment(", "writeReply(", "evt.key === 'Escape'"},
		"assets/static/app.css":   {".comments-form", ".comments-edit"},
		"help/en/panes.md":        {"`Edit`", "`Save`"},
	} {
		src, err := assets.ReadFile(file)
		if err != nil {
			src, err = helpFS.ReadFile(file)
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !assetContains(file, string(src), want) {
				t.Errorf("%s does not edit comments in the pane, missing %q", file, want)
			}
		}
	}
}
