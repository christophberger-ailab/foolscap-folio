# The folio command line in brief

`folio web` is one of several commands. They all work on the same project.

```
folio <object> <subcommand> [flags] [args]
```

## Global flag

| flag | environment variable | default | meaning |
| --- | --- | --- | --- |
| `--project`, `-p` | `QM_PROJECT` | `.` | path to the document tree |

## Commands

| command | what it does |
| --- | --- |
| `folio render [<topic>…]` | render the project's topic × format × audience matrix (`--draft` renders a draft for review) |
| `folio init` | write a starter `folio.yml` and `_brand.yml` into a Markdown tree, or a sample project into an empty folder |
| `folio migrate` | write `folio.yml` from the project's old `_quarto*.yml` files (`--dry-run` prints it, `--force` overwrites) |
| `folio lint [<topic>]` | check the `.md` and `.qmd` sources: unclosed `:::` fences and cross-references that cannot be linked |
| `folio web [<path>]` | serve the local sorting, editing and rendering UI (`--browser` opens it in the browser) |
| `folio preview [<path>]` | serve the website and update it live as pages change (`--draft` as with `folio render`) |
| `folio upgrade` | install the newest release of folio (alias `folio update`; `--check` only reports, `--yes` does not ask) |

Every command takes `--help`, which lists its flags and arguments.

## In this help

- [Opening projects](projects.md) covers `folio web`.
- [Rendering](render.md) is the web version of `folio render`.

Back to [Contents](index.md).
