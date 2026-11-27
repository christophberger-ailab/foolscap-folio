# The Markdown preview

The preview shows the page you are editing, updated as you type. Bring it to the front with its `Preview` tab (see [the panes](panes.md)).

The preview is an approximation. It is drawn in the browser from your Markdown and is not a render, so use [Rendering](render.md) for the real result.

## What it shows

- A ```` ```{mermaid} ```` block is drawn as its diagram, as a render draws it; a plain ```` ```mermaid ```` block stays code.
- Quarto fenced divs (`::: name`) and bracketed spans (`[text]{.name}`) get their class, so your custom CSS can style them.
- `{{name}}` and `{{< var name >}}` show their values, from `folio.yml` (for the page's topic) and `_variables.yml`, as a render resolves them.
- A cross-reference such as `[](#install)` shows the target's number and name, as a render does (see the [Quickstart](quickstart.md)). Click it to go to the target: the preview scrolls to it, or opens the page it is on.
- When a format in `folio.yml` sets `numbered_headings` or `numbered_figures`, headings and figure captions show their numbers, as the website does. A heading or figure you have not saved yet gets its number once the page is saved.
- A heading's attribute block, such as `{.unnumbered}`, is dropped, because it directs the render and is not part of what the heading says. Headings inside code blocks are left alone.
- A page written for one target group is previewed inside that group's div. This applies when the file name, or a folder above it, ends in `_FW` or `_POL` (the deepest name wins). The stylesheet then tints and marks the whole page, as it does a `::: fw` or `::: pol` block. The frontmatter stays outside the div.

## TODO markers

A `==TODO==` is shown as a filled pink `TODO` marker, in the same color the editor highlights it with. A TODO inside an HTML comment does not render, so the preview shows an outlined marker where the comment stands instead; hover it to read the comment.

## Images

Images are loaded from your project.

- A path starting with `/`, like `/assets/images/x.png`, is relative to the project root.
- Any other path is relative to the folder of the page.
- Only image files inside the open project are served.
- An image alone in its paragraph (blank line before and after) shows its alt text as a caption underneath. An image inside running text, or without alt text, gets no caption.
- Hover over an image to see its path as written in the page. If the image has a Markdown title, the title is shown first.
- Click an image to view it at full size over the app. Click again or press Escape to close it.
- Double-click an image to open it in your [image editor](external-editor.md). When you save in that program, the preview picks up the new version within a couple of seconds.
- `↻ Reload` shows images as they are on disk, even if you replaced one under the same name.

## Comments

A passage commented in the [editor](editor.md) is highlighted, also when it runs over several paragraphs. Hover over it to read the comment, who wrote it and when, and the replies to it; click it to put the editor's cursor at the start of the comment, ready to edit it. A comment written inside code is shown as code.

## Scrolling

The editor and the preview scroll together: moving either one brings the other to the same part of the page. The preview also stays with the editor while you type.

## Back and Forward

Below the preview, `←` and `→` go back and forward through the pages you opened and the cross-references you followed, as a browser does. Going back returns to the place in the page where you left it. Each button's tooltip names the page it goes to.

## Previous and Next

Below the preview, `◀ Previous` and `Next ▶` open the page before or after the current one, in the order of the [page tree](tree.md). Each button's tooltip names the page. At either end of the project the button is disabled.

## Custom CSS

The preview is styled by the project's stylesheets in `assets/styles/folio`. If the project has more than one, a dropdown below the preview switches between them. The choice is remembered and the preview reloads to apply it. See [Config pages and the settings file](settings.md) for how to write the stylesheets.

Back to [Contents](index.md).
