package project

import (
	"strings"
	"testing"
)

// page builds a page file with the given order and heading level.
func page(order, level int, body string) string {
	return "---\ntitle: Zugriffsebene setzen\norder: " + itoa(order) + "\n---\n" +
		strings.Repeat("#", level) + " Zugriffsebene setzen\n" + body
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// The tree renumbering a page must survive the autosave that follows it:
// the editor still holds the old order, and writing that back would put
// the page where it was dragged from.
func TestRebaseKeepsTheTreesOrder(t *testing.T) {
	base := page(3, 1, "text\n")
	disk := page(1, 1, "text\n")
	edited := page(3, 1, "text the user just typed\n")

	got, ok := Rebase([]byte(edited), []byte(base), []byte(disk))
	if !ok {
		t.Fatal("want the renumbering recognized as the tree's own")
	}
	if want := page(1, 1, "text the user just typed\n"); string(got.Body) != want {
		t.Errorf("body =\n%q\nwant\n%q", got.Body, want)
	}
	if !got.OrderChanged {
		t.Error("OrderChanged = false, want true")
	}
}

// A file nothing touched leaves the edit exactly as the user wrote it.
func TestRebaseUntouchedFilePassesTheEditThrough(t *testing.T) {
	base := page(1, 1, "text\n")
	edited := page(1, 1, "edited\n")

	got, ok := Rebase([]byte(edited), []byte(base), []byte(base))
	if !ok {
		t.Fatal("want an untouched file accepted")
	}
	if string(got.Body) != edited {
		t.Errorf("body = %q, want the edit unchanged", got.Body)
	}
	if got.OrderChanged {
		t.Error("want nothing reported as replayed")
	}
}

// Anything the tree did not write is a conflict: someone else edited the
// page, and there is no safe way to write over them.
func TestRebaseRefusesAnOutsideEdit(t *testing.T) {
	base := page(1, 1, "text\n")
	disk := page(1, 1, "text somebody else wrote\n")
	edited := page(1, 1, "text the user typed\n")

	if _, ok := Rebase([]byte(edited), []byte(base), []byte(disk)); ok {
		t.Error("want an outside edit refused, not merged")
	}
}

// An outside edit alongside the tree's own write is still a conflict: the
// replay has to reproduce the file exactly or not be trusted at all.
func TestRebaseRefusesAnOutsideEditAlongsideARenumber(t *testing.T) {
	base := page(3, 1, "text\n")
	disk := page(1, 1, "text somebody else wrote\n")
	edited := page(3, 1, "text the user typed\n")

	if _, ok := Rebase([]byte(edited), []byte(base), []byte(disk)); ok {
		t.Error("want the outside edit refused despite the renumber")
	}
}

// An order the user typed themselves is their own, and is kept over the
// one the tree wrote — a hand-made edit is never silently dropped.
func TestRebaseKeepsAnOrderTheUserTyped(t *testing.T) {
	base := page(3, 1, "text\n")
	disk := page(1, 1, "text\n")
	edited := page(9, 1, "text\n")

	got, ok := Rebase([]byte(edited), []byte(base), []byte(disk))
	if !ok {
		t.Fatal("want the renumbering recognized")
	}
	if want := page(9, 1, "text\n"); string(got.Body) != want {
		t.Errorf("body =\n%q\nwant the user's own order kept\n%q", got.Body, want)
	}
	if got.OrderChanged {
		t.Error("OrderChanged = true, want false when the user set it")
	}
}

// The tree no longer shifts headings (issue #96), so a heading level that
// changed on disk was changed by someone else and is a conflict.
func TestRebaseRefusesAHeadingShift(t *testing.T) {
	base := page(1, 1, "text\n")
	disk := page(1, 2, "text\n")
	edited := page(1, 1, "edited\n")

	if _, ok := Rebase([]byte(edited), []byte(base), []byte(disk)); ok {
		t.Error("want a heading shift on disk refused as an outside edit")
	}
}

// Applying the replay repeatedly must not compound it: the base moves on
// with every save.
func TestRebaseIsStableAcrossRepeatedSaves(t *testing.T) {
	base := page(3, 1, "text\n")
	disk := page(1, 1, "text\n")

	first, ok := Rebase([]byte(page(3, 1, "one\n")), []byte(base), []byte(disk))
	if !ok {
		t.Fatal("first rebase refused")
	}
	// The editor still holds the old order, and the file now holds what
	// the first save wrote, so the next save rebases from there.
	second, ok := Rebase([]byte(page(3, 1, "two\n")), []byte(page(3, 1, "one\n")), first.Body)
	if !ok {
		t.Fatal("second rebase refused")
	}
	if want := page(1, 1, "two\n"); string(second.Body) != want {
		t.Errorf("body =\n%q\nwant\n%q", second.Body, want)
	}
}
