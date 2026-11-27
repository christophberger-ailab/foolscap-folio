# The page tree

The left side of the window shows the project's `.md` (and older `.qmd`) pages as a tree, in the order of the website, nested by folder. The `assets` folder at the top of the project, and the folders and pages that `ignore` in `folio.yml` names (such as `_*/` for every folder starting with `_`), are left out, along with everything inside them. Click a title to open the page in the [editor](editor.md).

## Titles

A page's title is taken from, in this order:

- the `title:` in its frontmatter,
- the text of its first Markdown heading,
- a title made from its file name.

A heading's own attribute block, such as `{.unnumbered}`, is dropped from the title. A Quarto span such as `[Polizei-]{.pol}` is shown as the symbol its class carries in the active [custom preview stylesheet](preview.md), followed by the span's text. Spans whose class has no symbol are shown as written.

## Numbering

The root `index.md` is the home page. It is listed first, without a number, and it is the only page that needs no `order:`. Topic pages (the top-level folders) are not numbered, though a topic without an `order:` still shows ⚠. Sections are numbered 1, 2, … within each topic, and chapters 1.1, 1.2, and so on deeper down.

Each source file is expected to start with an h1. In a printed document (DOCX, PDF) a page's place in the tree decides how its headings are set, and a heading that would end up deeper than h6 becomes emphasized text. A page whose headings would render as h7 or deeper at its level gets a light grey row. Hover over it to see which heading is too deep. The pages above it are greyed as well, with a tooltip saying that a page below has headings that are too deep.

## Drafts

A page with `draft: true` in its frontmatter is greyed out. It is part of the project but not part of what a render publishes. You can still open, move, and search it.

## Status marks

A page can carry status marks after its title:

- 🆕 new: `status: new` in the frontmatter. New pages start with it.
- 🗑️ deprecated: `status: deprecated`, `obsolete`, or `old`.
- 🚧 to do: the text `==TODO==` anywhere in the page, even inside an HTML comment.
- 👀 to review: `review:` in the frontmatter, set to what should be reviewed. The mark's tooltip reads "Review: " followed by that text. `review: true` marks the page without saying what to review.
- 💬 commented: the page holds at least one comment (see the editor's help). A comment marker inside code does not count.
- 🔖 bookmarked: `bookmarked: true`, which the editor's 🔖 button sets (see [Editing a page](editor.md)).

```yaml
---
title: My page
status: new
review: the figures in section 2
---
```

Each mark is shown once per page, however many `==TODO==` the page holds.

In a Git working tree, a page changed since the last commit shows an asterisk (`*`) after its title.

## Hiding pages

A page with `hide: true` in its frontmatter is left out of the tree. The default is `false`. A hidden section page hides its whole branch. Hiding only affects the tree: the page is still rendered, and the [search](search.md) still finds it, so you can open it there and remove the setting again.

```yaml
---
title: My page
hide: true
---
```

Pages written for earlier versions keep these keys under `folio:` or `qm:`. They are still read; a key at the top wins.

## Reordering and moving

Drag a page by its `⠿` handle.

- Dropping it among its siblings rewrites the `order` frontmatter of the affected pages.
- Dropping it under another parent also renames its file on disk. Its Markdown heading levels are left as they are: you control them.
- If the page you have open moves, the editor follows it to the new path. Your unsaved text stays untouched, and autosave keeps writing to the right file.

## Creating pages

- Type the page's name at the top, then press `＋ Page`.
- The page is saved as `name/index.md`. Its folder name is the name in lower case, with dashes for spaces: "Map view" becomes `map-view/index.md`.
- A new page has no `title` in its frontmatter, only its `order` and `status: new`. It starts with a level-1 heading inside an `::: explanation` div, whatever its place in the tree. The heading is the name as you typed it (`# Map view`), and it is the page's title.
- The `＋` button on a row adds a child page under that page. You are asked for the name.

## Deleting pages

The `🗑` button on a row asks for confirmation and then moves the page to the trash.

## Expanding and collapsing

Click the small toggle on a row to collapse or expand a branch. The `⊞` and `⊟` buttons below the tree expand or collapse every branch.

## Outside changes

The tree checks the project for changes made outside the browser and refreshes itself when something changed.

Back to [Contents](index.md).
