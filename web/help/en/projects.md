# Opening projects

A project is a document tree of Markdown pages, optionally with a `folio.yml`. `folio web` shows one project at a time.

## Starting folio web

```sh
folio web                    # serve the project given by --project (default: the current directory)
folio web ../handbook        # serve that project instead
folio web --addr :9000       # listen elsewhere
```

- A path after `web` names the project to open. It takes precedence over `--project`.
- Without a path, the project is the one given by `--project` (or `-p`), or the current directory.
- The command keeps running until you interrupt it, and prints the address it serves on.

## The address and port

- By default `folio web` listens on `localhost:8199`.
- If that port is taken, usually by another `folio web`, the next port up is tried, and so on until a free one is found. The address actually used is printed.
- An address you give with `--addr` is used exactly as given. If it is taken, that is an error.

## Opening a project

The top bar shows the open project's title, then `Open`, then `↻ Reload`.

- The title is the `title` set in the project's `qm.yml`. Without one, it is the project's folder name. Hover over it to see the full path.
- `Open` shows a folder dialog. It starts in the folder you last opened a project from, with that project selected. The first time, it starts in the current directory.
- Folders that are projects (they hold a `qm.yml`) are shown with 📘 and in bold.
- Click a folder to select it. Double-click it, or press `Enter`, to go into it; on a project, that opens it. `↑` or `Backspace` goes up one folder.
- The dialog's `Open` button opens the selected folder, or the folder shown if none is selected.
- Type or paste a path into the field at the top and press `Enter` to go there.
- The Recent list on the left holds the ten projects you opened most recently, newest first. One click opens one. A project you started from the command line counts as opened, so it appears in the list next time.
- `Esc` or `Cancel` closes the dialog.
- `↻ Reload` reads the open page and the tree from disk again.

Opening a project updates everything that belongs to it: the page tree, the panes, the Render panel, the Git panel, and the Editor button.

The list of recent projects is kept in the [settings file](settings.md).

Back to [Contents](index.md).
