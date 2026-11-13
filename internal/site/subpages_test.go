package site

import (
	"strings"
	"testing"
)

func TestSubpages(t *testing.T) {
	root, out := project(t)
	write(t, root, "dispatcher/index.qmd", "---\ntitle: Dispatcher\n---\n\n# Dispatcher\n\nBefore.\n\n"+
		"::: {.subpages title=\"Themen & mehr\"}\n:::\n\nAfter.\n")
	write(t, root, "dispatcher/a/deep.qmd", "---\ntitle: Deep\n---\n\nText.\n")
	write(t, root, "dispatcher/b.qmd", "---\ntitle: Page <B>\norder: 2\n---\n\n::: subpages\n:::\n\nA leaf.\n")
	write(t, root, "dispatcher/a/index.qmd", "---\ntitle: Page A\norder: 1\n---\n\n::: subpages\n:::\n")
	write(t, root, "index.md", "---\ntitle: Home\n---\n\n::: {.subpages}\n:::\n")
	if err := Build(opts(root, out)); err != nil {
		t.Fatal(err)
	}

	// A folder's index page lists its files and its folders with an index
	// page alike, in sidebar order, at the block's position.
	idx := read(t, out, "dispatcher/index.html")
	has(t, "index", idx,
		`<table class="subpages">`,
		"<thead><tr><th>Themen &amp; mehr</th></tr></thead>",
		`<tr><td><a href="a/index.html">Page A</a></td></tr>`,
		`<tr><td><a href="b.html">Page &lt;B&gt;</a></td></tr>`)
	// Not the pages further down, nor a folder without a page of its own.
	hasNot(t, "index", idx, `>Deep</a></td>`, `>Sub</a></td>`, `>Group</a></td>`)
	before, table, after := strings.Index(idx, "Before."), strings.Index(idx, "<table"), strings.Index(idx, "After.")
	if before > table || table > after {
		t.Errorf("table not at the block's position: %d %d %d", before, table, after)
	}
	if strings.Index(idx, ">Page A</a></td>") > strings.Index(idx, ">Page &lt;B&gt;</a></td>") {
		t.Error("subpages not in sidebar order")
	}

	// Without a title the table has no header; links are relative to the page.
	a := read(t, out, "dispatcher/a/index.html")
	has(t, "a", a, `<table class="subpages">`, `<tr><td><a href="deep.html">Deep</a></td></tr>`)
	hasNot(t, "a", a, "<thead>")

	// A page without subpages shows no table.
	hasNot(t, "b", read(t, out, "dispatcher/b.html"), "<table", "subpages")

	// The home page lists the topics.
	has(t, "home", read(t, out, "index.html"),
		`<tr><td><a href="dispatcher/index.html">Dispatcher</a></td></tr>`,
		`<tr><td><a href="x/index.html">Custom X</a></td></tr>`)
}
