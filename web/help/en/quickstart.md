# Quickstart

The shortest way to each first step. The other topics have the details.

## Install folio

- **Windows:** download `folio_<version>_windows_setup.exe` from the latest release on GitHub and run it. It needs no administrator rights and adds **Foolscap Folio** to the Start menu.
- **macOS and Linux:** `brew install christophberger-ailab/tap/folio`.
- **Windows with Scoop:** `scoop bucket add folio https://github.com/christophberger-ailab/scoop-bucket`, then `scoop install folio`.
- **By hand:** unpack the binary for your system from a release and put `folio` on your `PATH`.

`folio upgrade` updates folio later, or points you to the package manager that installed it.

## Open a project

```sh
folio web path/to/project
```

Or start `folio web` anywhere and press `Open` in the top bar: pick the project's folder, or one from the Recent list. `From a repository…` in the same dialog clones a Git repository and opens it. See [Opening projects](projects.md).

## Create a new project

```sh
mkdir handbook
cd handbook
folio init
folio web
```

In an empty folder, `folio init` writes a sample project to start from: a home page, a `handbook` topic with sections and chapters, a `folio.yml` and a `_brand.yml`. In a folder of Markdown pages, it writes only `folio.yml` and `_brand.yml`, with every content folder as a topic. Each page starts with front matter:

```markdown
---
title: Installing
order: 2
---

# Installing
```

## Folio's Markdown extensions

### Blocks and spans

A fenced block or a bracketed span with a class is kept only by the formats and audiences that show the class:

```markdown
::: pol
Only for the `pol` audience.
:::

Fire engines [(Feuerwehr only)]{.fw} arrive first.

- Item 1
[- Item 2 is only for POL]{.pol}
- Item 3
```

A class that no format or audience lists under `show:` in `folio.yml` is a plain styling class.

### Callouts

```markdown
::: {.callout-tip title="Save often"}
`folio web` saves as you type.
:::
```

The types are `note`, `tip`, `warning`, `important` and `caution`. `collapse="true"` folds the callout under its title, `appearance="simple"` drops the shading, `icon=false` the icon.

### Cross-references

A link to a heading or a figure by its key fills in the target's number and name:

```markdown
## Installing it {#install}

See [](#install), [](#install){.name}, [](#install){.number} or [this section](#install).
```

A heading without an identifier of its own has its text as the key, in lower case, with hyphens for spaces and the symbols dropped: `## The (not so far) future` is `#the-not-so-far-future`. Umlauts stay, `## Einführung` is `#einführung`, but `#einfuehrung` and `#einfuhrung` find it too. To point to another page, put its `id:` (or its path) first: `[](#install:overview)`.

### Image size

```markdown
![The map](images/map.png){width=50%}
![](images/logo.svg){height=2cm}
```

Sizes take `cm`, `mm`, `in`, `pt`, `px` (the default), `em` or `%`. With one of width and height, the other follows the aspect ratio. An image with alt text alone in its paragraph is a figure, captioned with the alt text.

### Definition lists

```markdown
Callout
: A box that draws attention to a note, a tip or a warning.

Span
: A bracketed piece of text with attributes, `[text]{.class}`.
```

A definition may have several paragraphs; indent them by four spaces.
