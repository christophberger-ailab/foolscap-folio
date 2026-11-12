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
| `folio render [<topic>…]` | render the project's topic × format × audience matrix |
| `folio flatten <topic>` | write the Markdown a document render of one topic parses, to `_build/<topic>.md`, without rendering |
| `folio migrate` | write `folio.yml` from the project's old `_quarto*.yml` files (`--dry-run` prints it, `--force` overwrites) |
| `folio lint [<topic>]` | check the `.md` and `.qmd` sources (currently: unclosed `:::` fences) |
| `folio chapters insert <folder>` | give a new page its `order:`, renumbering its siblings |
| `folio chapters move <folder> <old> <new>` | move a chapter to another order position |
| `folio chapters remove <folder> <order>` | remove a chapter, after confirmation |
| `folio web [<path>]` | serve the local sorting, editing and rendering UI |

Every command takes `--help`, which lists its flags and arguments.

## In this help

- [Opening projects](projects.md) covers `folio web`.
- [Rendering](render.md) is the web version of `folio render`.

Back to [Contents](index.md).
