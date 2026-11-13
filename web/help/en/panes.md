# The panes beside the editor

Beside the editor stands a dock with six panes, each behind a tab:

- `Preview` shows the [Markdown preview](preview.md).
- `Copyedit` shows the [copyediting tool](copyedit.md).
- `Comments` lists the comments of the page (see below).
- `Render` [renders the project](render.md).
- `Git` shows [the changes since the last commit](git.md).
- `Help` shows this help.

## Tabs and groups

Panes are gathered in groups. A group shows one pane at a time: click a tab to bring its pane to the front.

Drag a tab to rearrange the panes, the way editor tabs move in VS Code:

- Drop it among the tabs of a group to move it there, at the place you drop it. This also reorders the tabs of a group.
- Drop it near the left or right edge of a group to put it in a group of its own beside that group.
- Drop it near the top or bottom edge of a group to put it above or below that group.

While you drag, the part of the group the pane would take is shaded. A group whose last tab is moved away disappears.

## Closing and opening panes

Tabs have no close button, so a click on a tab never closes its pane by accident. Close a pane from the `Panes ▾` menu, or with a middle click on its tab. When the last pane is closed, the editor takes the whole width.

The `Panes ▾` menu in the top bar lists every pane, with a tick beside the open ones. Pick a closed pane to open it, an open one to close it. `Reset layout` puts every pane back in one group.

## The comments pane

The comments pane lists the [comments](editor.md) of the page in the editor, in the order they stand, each with who wrote it and when, its text, the passage it is about, and its replies. It follows the editor as you type.

- Click a comment or a reply, anywhere but on its buttons, to put the cursor at its text in the editor.
- `Edit`, shown on the comments and replies signed with [your name](settings.md), turns the text into a field right in the pane, so you do not have to write among the markers in the page. `Save` or `Enter` writes it into the page; `Cancel` or `Escape` leaves the comment as it was.
- `Reply` opens the same field for a new reply. It is written after the last reply, signed with your name and the date, when you save it; a reply left empty is not written.
- A comment is kept on one line: line breaks you type become spaces.
- While a field is open, the list does not follow the editor. If the comment was changed in the editor meanwhile, your text is not saved and you are told so.
- `Ctrl-Z` in the editor takes a saved edit or reply back.
- `Remove` removes the comment and its replies after asking; the commented text stays.

## The help pane

The help pane shows these pages. Links between help pages open in the pane itself; a link to a web site opens in a new browser tab. Above the page you find:

- `◀ Back` returns to the help page you read before.
- `Contents` brings you to the table of contents.
- The dropdown jumps to any help topic.
- The language menu switches the help between English and German. The help starts in your system's language when it has that language, and in English otherwise; once you pick one, it keeps your choice.

The pane remembers the page you were reading, also across sessions. It is there even when no page is open, so you can read the help before you open a project or select a page.

## Resizing

- The divider between the editor and the dock changes the width of both.
- The line between two groups moves room from one to the other.

The tree on the far left has a divider of its own as well.

## Remembered

Which panes are open, how they are grouped and arranged, which tab is in front, and the sizes are remembered across page switches, reloads, and sessions.

Back to [Contents](index.md).
