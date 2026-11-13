# Config pages and the settings file

## The Config button

`Config` at the far right of the top bar opens a page listing the configuration areas. Use the `App` button at the top to return. Above the areas, a `Release` box names the folio release you are running and has a `Check for updates` button (see [The settings file](#the-settings-file) for the automatic checks). When a newer release is found, the box says so and the top bar's **Update** button installs it. A development build cannot check. The areas are:

- `Preview: Custom CSS`: the stylesheet for the [preview](preview.md).
- `Copyedit: Editing tasks` and `Copyedit: API connections`: see [copyediting](copyedit.md).
- `External editor`: see [other editors](external-editor.md).
- `Comments: Your name`: the name your [comments](editor.md#comments) and replies are signed with.
- `Project: folio.yml`: the open project's configuration file, as text.
- `folio: config.cue`: the settings file below, as text.

## Project: folio.yml

Edits the open project's `folio.yml` (or its older `qm.yml`) in a text editor. `Save` checks the file the way a render reads it, so an unknown key or an output type folio cannot write is refused with the line it is on, and the file stays as it was. Saving creates `folio.yml` when the project has none. A changed listen address takes effect at the next start of `folio web`.

## folio: config.cue

Edits [the settings file](#the-settings-file) in a text editor. `Save` checks the settings against the schema and puts them in effect at once. API keys are never shown: leave `(stored key, not shown)` in place to keep a key, or type a new key over it. The same rules as for [editing by hand](#editing-by-hand) apply: comments among the settings are not kept.

## Comments: Your name

`folio web` runs on your own computer, so there is no login. The comments and replies you write in the editor are signed with the name you enter here, together with the date. Leave the field empty to sign with the name you are logged in to the computer with. Double quotes, braces, and line breaks are not allowed in the name.

## Preview: Custom CSS

Here you write CSS that overrides the preview's built-in styles. It is loaded document-wide after the app's own stylesheet. To affect only the preview, scope your rules with `.markdown-preview`, for example `.markdown-preview h1 { color: rebeccapurple; }`.

- Press `Save` to store it. Drag the corner of the editor to make it taller or shorter; the height is remembered.
- To add another stylesheet, type a name such as `dark.css` under `Add an alternate stylesheet` and press `Add stylesheet`. With more than one stylesheet, an `Editing:` dropdown switches between them.
- The active stylesheet is chosen with the dropdown below the preview.
- Rules like `.folio.pol:before { content: "..." }` also supply the symbols the [page tree](tree.md) shows for spans.

Stylesheets are files named `<user config dir>/folio/custom-css/*.css`. A default `custom.css` is created the first time `folio web` runs.

## The settings file

Everything `folio web` remembers lives in one file: `<user config dir>/folio/config.cue`. It holds:

- the recently opened projects,
- the render selection and last open page of each project,
- the active preview stylesheet,
- the copyediting tasks and connections (including API keys),
- the external editor command lines,
- the name your comments are signed with (`user: name:`).
- where and how often to look for a new folio release (`update: source:` and `update: check:`, which is `daily`, `weekly` or `never`). When `folio web` finds one, an **Update** button appears at the right end of the top bar; it installs the release, which is used from the next start of `folio web`.

It is readable by you alone because of the keys. The file is created with defaults on the first run. Older JSON settings files are carried over once and renamed to `*.migrated`.

## Editing by hand

You may edit the file yourself.

- It is written in CUE and begins with its own schema, so it says what belongs in it. The app rewrites the schema on every save.
- Fields you leave out take their defaults. Lists and maps left out are empty.
- A misspelled field name or a wrong value is an error, reported with file, line, and column.
- Comments you write among the settings are lost the next time the app saves.

## Errors stop the app

If the file does not check out, `folio web` stops instead of replacing it, because it is the only copy of your connections and selections. Fix the reported line and start again. A save that the app itself could not read back is refused.

Back to [Contents](index.md).
