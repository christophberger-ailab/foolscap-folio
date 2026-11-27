# Opening projects

A project is a document tree of Markdown pages, optionally with a `folio.yml`. `folio web` shows one project at a time.

## Starting folio web

```sh
folio web                    # serve the project given by --project (default: the current directory)
folio web ../handbook        # serve that project instead
folio web --addr :9000       # listen elsewhere
folio web --browser          # and open the app in your browser
```

- A path after `web` names the project to open. It takes precedence over `--project`.
- Without a path, the project is the one given by `--project` (or `-p`), or the current directory.
- The command keeps running until you interrupt it, and prints the address it serves on.
- `--browser` opens that address in your default browser. The Windows Start menu entry starts `folio web --browser`, with its console window minimized; closing that window stops `folio web`.
- A folder without a `folio.yml`, such as the one the Start menu starts in, opens no project. Pick one with `Open`.

## The address and port

- By default `folio web` listens on `localhost:8199`.
- A project can name its own address in the `web` section of its `folio.yml`: `baseurl` is the host name or IP address, `port` the port. For example `web: {baseurl: localhost, port: 8200}`. It is read from the project `folio web` is started on.
- If that port is taken, usually by another `folio web`, the next port up is tried, and so on until a free one is found. The address actually used is printed.
- An address you give with `--addr` wins over `folio.yml` and is used exactly as given. If it is taken, that is an error.

## Opening a project

The top bar shows the open project's title, then `Open`, then `↻ Reload`.

- The title is the `title` set in the project's `folio.yml`. Without one, it is the project's folder name. Hover over it to see the full path.
- `Open` shows a folder dialog. It starts in the folder you last opened a project from, with that project selected. The first time, it starts in the current directory.
- Folders that are projects (they hold a `folio.yml`) are shown with 📘 and in bold.
- Click a folder to select it. Double-click it, or press `Enter`, to go into it; on a project, that opens it. `↑` or `Backspace` goes up one folder.
- The dialog's `Open` button opens the selected folder, or the folder shown if none is selected.
- Type or paste a path into the field at the top and press `Enter` to go there.
- The Recent list on the left holds the ten projects you opened most recently, newest first. One click opens one. A project you started from the command line counts as opened, so it appears in the list next time.
- `Esc` or `Cancel` closes the dialog.
- A folder without `folio.yml` opens with an offer in the editor area: `Start a new project here` writes a `folio.yml`, a `_brand.yml` and, in an empty folder, a home page, as `folio init` does; in an empty folder, `Create a demo project here` writes the demo project, as `folio init --demo` does. Either opens the new project.
- `↻ Reload` reads the open page and the tree from disk again.

Opening a project updates everything that belongs to it: the page tree, the panes, among them the Render and Git panes, and the Editor button.

The list of recent projects is kept in the [settings file](settings.md).

## Opening a project from a repository

You can start from a Git repository instead of a folder on disk.

1. Press `Open`, then `From a repository…`.
2. Paste the repository's address, as the hosting site shows it: `https://github.com/owner/book.git` or `git@github.com:owner/book.git`.
3. If the book is not on the default branch, enter its `Branch`. If it is not at the top of the repository, as when it lives beside code, enter its `Folder of the book in the repository`, such as `docs`.
4. Press `Clone and open`.

The clone goes into folio's own data folder, so you need not choose one; open `Where the clone goes` to clone somewhere else. The clone goes into a new folder there, named after the repository. Cloning the same repository again opens the clone already there.

If the book's folder has a `folio.yml`, it opens right away. If not, the dialog shows it so you can pick the subfolder that holds the book.

If git cannot reach the host while your browser can, the network most likely wants a proxy that git is not told about. On Windows, folio then clones once more through the proxy Windows uses, and the clone keeps it for pull and push. If that fails too, or on other systems, the form says what to check and the command that tells git about a proxy.

For a private repository on another host, such as Azure DevOps or GitLab, paste a personal access token into `Access token`. folio sends it to that host only and keeps it for push and sync.

From then on it is an ordinary project in a folder of its own: commit, push, and sync in the [Git pane](git.md), or in a terminal.

### Signing in to GitHub

A public repository clones without signing in. For a private one, and to push your changes back, git needs to know who you are. If you have not set up git credentials, press `Sign in to GitHub` in the form:

1. folio shows a short code and a link to `github.com/login/device`.
2. Open the link, enter the code, and approve folio.
3. The form says whom you are signed in as.

folio keeps the sign-in for the next start, in a file next to the [settings file](settings.md) that only you can read, and sends it to github.com only. `Sign out` forgets it. To withdraw folio's access on GitHub's side too, revoke it under Settings → Applications on github.com.

## Updating folio

`folio web` looks for a new release once a day. When it finds one, an `Update` button appears at the right end of the top bar. It updates folio, restarts `folio web` at the same address with the same project open, and reloads the page. `What's new` then shows the release notes.

Back to [Contents](index.md).
