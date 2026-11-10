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

## The Open field

The Open field sits in the top bar.

- It shows only the last element of the project's path, because the folder name is what tells projects apart. Hover over it to see the full path.
- Type or paste a path and press `Open` to open that path as written.
- The `▾` button next to the field lists the ten projects you opened most recently, newest first, by their full paths. Pick one to fill the field.
- A project you started from the command line counts as opened, so it appears in the list next time.

Opening a project updates everything that belongs to it: the page tree, the panes, the Render panel, the Git panel, and the Editor button.

The list of recent projects is kept in the [settings file](settings.md).

Back to [Contents](index.md).
