# Config pages and the settings file

## The Config button

`Config` at the far right of the top bar opens a page listing the configuration areas. Use the `App` button at the top to return. Above the areas, a `Release` box names the folio release you are running and has a `Check for updates` button (see [The settings file](#the-settings-file) for the automatic checks). When a newer release is found, the box says so and offers an **Update to …** button, which installs it just like the top bar's **Update** button. Below the button, the release notes of every release the update brings are listed, newest first, including the ones in between when you skipped some. After an update, folio web restarts itself with the new release and the page reloads; a **What's new in …** button in the top bar then leads back here to the notes of everything the update brought. A development build is offered the newest release, whatever it is. The areas are:

- `Configuration files`: the open project's `folio.yml`, its `_brand.yml` and its preview stylesheets, and folio's own `config.cue`, as text, each on a tab of its own.
- `Copyedit: Editing tasks` and `Copyedit: API connections`: see [copyediting](copyedit.md).
- `External editor`: see [other editors](external-editor.md).
- `Comments: Your name`: the name your [comments](editor.md#comments) and replies are signed with.

## Configuration files

One page holds every configuration file, a tab each: `folio.yml`, `_brand.yml`, `Preview CSS` and `config.cue`. Each tab edits its file as a whole.

### folio.yml

Edits the open project's `folio.yml` (or its older `qm.yml`) in a text editor. `Save` checks the file the way a render reads it, so an unknown key or an output type folio cannot write is refused with the line it is on, and the file stays as it was. Saving creates `folio.yml` when the project has none. A changed listen address takes effect at the next start of `folio web`.

### _brand.yml

Edits the brand file in a text editor: the project's own `_brand.yml`, or, if it has none, yours beside the settings file, which styles every project without one. With neither, saving creates the project's. `Save` checks the file the way a render reads it, so an unknown `meta: fonts:` entry or an unreadable size is refused, and the file stays as it was. The `meta:` fields `coverpage` and the font names may use `{name}` variables from `folio.yml`, such as `{audience-template}`.

### Preview CSS

The preview stylesheets belong to the project: they are the `.css` and `.scss` files in its `assets/styles/folio` folder, since what they mark are the project's own topics and audiences. A stylesheet is loaded document-wide after the app's own. To affect only the preview, scope your rules with `.markdown-preview`, for example `.markdown-preview h1 { color: rebeccapurple; }`.

- Press `Save` to store it. Drag the corner of the editor to make it taller or shorter; the height is remembered.
- To add another stylesheet, type a name such as `pol.css` under `Add a stylesheet` and press `Add stylesheet`. With more than one stylesheet, an `Editing:` dropdown switches between them.
- The active stylesheet is chosen with the dropdown below the preview.
- Rules like `.folio.pol:before { content: "..." }` also supply the symbols the [page tree](tree.md) shows for spans.
- A project without stylesheets is previewed in a built-in one, which the tab shows; saving it puts it into the project as `custom.css`.
- A website of every topic links `all.css` (or `all.scss`) from the same folder, so the published site can mark the blocks as the preview does.

### config.cue

Edits [the settings file](#the-settings-file) in a text editor. `Save` checks the settings against the schema and puts them in effect at once. API keys are never shown: leave `(stored key, not shown)` in place to keep a key, or type a new key over it. The same rules as for [editing by hand](#editing-by-hand) apply: comments among the settings are not kept.

## Comments: Your name

`folio web` runs on your own computer, so there is no login. The comments and replies you write in the editor are signed with the name you enter here, together with the date. Leave the field empty to sign with the name you are logged in to the computer with. Double quotes, braces, and line breaks are not allowed in the name.

## The settings file

Everything `folio web` remembers lives in one file: `<user config dir>/folio/config.cue`. It holds:

- the recently opened projects,
- the render selection and last open page of each project,
- the active preview stylesheet,
- the copyediting tasks and connections (including API keys),
- the external editor command lines,
- the name your comments are signed with (`user: name:`).
- where and how often to look for a new folio release (`update: source:` and `update: check:`, which is `hourly`, `daily`, `weekly` or `never`). When `folio web` finds one, an **Update** button appears at the right end of the top bar; it installs the release, which is used from the next start of `folio web`.
- the titles of callouts that have no title of their own, per language (`callouts:`). The language is the project's `lang` in `folio.yml`; `folio render`, `folio preview` and the preview in `folio web` all use them. English and German come filled in; add a language like this, and leave out any type you want in English:

  ```
  callouts: fr: {note: "Remarque", tip: "Astuce", warning: "Avertissement", important: "Important", caution: "Attention"}
  ```

  A regional `lang` such as `fr-CA` uses `fr` unless it has an entry of its own.

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
