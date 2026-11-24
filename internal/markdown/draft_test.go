package markdown

import (
	"strings"
	"testing"
)

// A commented passage at the start of a line is Markdown like any other,
// not an HTML block that swallows the line as raw text.
func TestCommentAtLineStartIsMarkdown(t *testing.T) {
	got := render(t, "<!-- << c -->*passage*<!-- >> --> after\n", Options{})
	if !strings.Contains(got, "<p><!-- << c --><em>passage</em>") {
		t.Errorf("passage not parsed:\n%s", got)
	}
	got = render(t, "```\n<!-- << c -->*code*\n```\n", Options{})
	if strings.Contains(got, "​") {
		t.Errorf("code changed:\n%q", got)
	}
}

// Without the draft option, todos stay text and comments stay HTML
// comments, as before.
func TestNoDraftLeavesMarkersAlone(t *testing.T) {
	got := render(t, "Do ==TODO== this <!-- << c -->x<!-- >> -->.\n", Options{})
	if strings.Contains(got, "folio-todo") || strings.Contains(got, "folio-comment") {
		t.Errorf("marked without draft:\n%s", got)
	}
}

func TestDraftTodos(t *testing.T) {
	got := render(t, "Do ==TODO== this, `==TODO==` stays.\n\n<!-- ==TODO== later -->\n\nText <!-- ==TODO== fix -->.\n", Options{Draft: true})
	for _, want := range []string{
		`Do <mark class="folio-todo">TODO</mark> this, <code>==TODO==</code> stays.`,
		`<p><mark class="folio-todo folio-todo-hidden" title="==TODO== later">TODO</mark></p>`,
		`Text <mark class="folio-todo folio-todo-hidden" title="==TODO== fix">TODO</mark>.`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
}

func TestDraftComment(t *testing.T) {
	src := `Text <!-- << {#c1 author="Ann" date="2026-11-01"} Check <this> --> <!-- <<+ {author="Bob"} Agreed -->the *passage*<!-- >> {#c1} --> after.` + "\n"
	got := render(t, src, Options{Draft: true})
	want := `<p>Text <mark class="folio-comment">the <em>passage</em></mark>` +
		`<span class="folio-note" role="note"><span class="folio-note-entry"><span class="folio-note-meta">Ann, 2026-11-01</span> Check &lt;this&gt;</span>` +
		`<span class="folio-note-entry folio-note-reply"><span class="folio-note-meta">Bob</span> Agreed</span></span> after.</p>`
	if !strings.Contains(got, want) {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// A passage across paragraphs is highlighted in each of them; its note
// stands with the first.
func TestDraftCommentAcrossBlocks(t *testing.T) {
	got := render(t, "<!-- << Long -->First\n\nMiddle\n\nLast<!-- >> --> rest\n", Options{Draft: true})
	for _, want := range []string{
		`<mark class="folio-comment">First</mark><span class="folio-note" role="note">`,
		`<p><mark class="folio-comment">Middle</mark></p>`,
		`<p><mark class="folio-comment">Last</mark> rest</p>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	if strings.Count(got, "folio-note-entry") != 1 {
		t.Errorf("note repeated:\n%s", got)
	}
}

// An unclosed comment highlights the rest of its paragraph.
func TestDraftCommentUnclosed(t *testing.T) {
	got := render(t, "One <!-- << Hm -->two three\n\nNext\n", Options{Draft: true})
	if !strings.Contains(got, `One <mark class="folio-comment">two three</mark>`) || strings.Contains(got, `<mark class="folio-comment">Next`) {
		t.Errorf("got\n%s", got)
	}
}
