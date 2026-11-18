package project

import "bytes"

// Rebasing an edit onto the tree's own writes
//
// The web editor holds a page's text from the moment it opened it, and
// autosaves that whole text back. The tree, meanwhile, writes to the same
// files behind the editor's back: every move and every create renumbers a
// sibling group's `order:`. An autosave posted after one of those has an
// `order:` from before it, and writing it out as it stands puts the move
// back the way it was.
//
// That write is the only one to reckon with, and it is mechanical: SetOrder
// on the frontmatter. (The tree used to shift a moved page's headings too;
// since issue #96 heading levels are the author's alone.) So an edit is not
// merged line by line but replayed: the order the tree wrote is checked
// against the file, and then applied to what the user typed.
//
// The replay is checked by reconstruction rather than by reading the
// difference: the order is applied to the base and the result compared to
// the file, byte for byte. Anything else that touched the page — an
// external editor, another tool — is a conflict rather than something to
// be papered over. That makes the failure mode "refuse and say so", never
// "write the wrong thing".

// Rebased is an edit replayed onto the tree's writes: the text to save,
// and whether the replay had to put the tree's order back, which the
// caller reports to the user — the editor still shows the text from
// before, and only a reload brings it in line.
type Rebased struct {
	// Body is the text to write.
	Body []byte
	// OrderChanged reports whether the tree renumbered the page.
	OrderChanged bool
}

// Rebase replays the tree's own writes onto an edit. base is the text the
// editor started from, disk the file as it now stands, and edited what the
// user is saving.
//
// It reports false when the difference between base and disk is not the
// tree's doing — someone else wrote the file — in which case there is
// nothing safe to write and the caller must not.
func Rebase(edited, base, disk []byte) (Rebased, bool) {
	// The common case: nothing moved underneath, so the edit stands as it
	// is. This is also the answer whenever the editor is already current.
	if bytes.Equal(base, disk) {
		return Rebased{Body: edited}, true
	}
	order := ParseFrontmatter(disk).Order
	if order == nil || !bytes.Equal(SetOrder(base, *order), disk) {
		return Rebased{}, false
	}
	// The order the tree wrote is put back only where the user left the
	// field alone. Someone who typed a new order into the frontmatter
	// meant it, and an edit the user made by hand is never discarded; the
	// next move renumbers the group anyway.
	if !sameOrder(ParseFrontmatter(edited).Order, ParseFrontmatter(base).Order) {
		return Rebased{Body: edited}, true
	}
	return Rebased{Body: SetOrder(edited, *order), OrderChanged: true}, true
}

// sameOrder compares two order fields, either of which may be absent.
func sameOrder(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
