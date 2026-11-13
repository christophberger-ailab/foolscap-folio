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
| `folio lint [<topic>]` | check the `.md` and `.qmd` sources (currently: unclosed `:::` fences) |
| `folio web [<path>]` | serve the local sorting, editing and rendering UI |
| `folio upgrade` | install the newest release of folio (alias `folio update`; `--check` only reports, `--yes` does not ask) |

Every command takes `--help`, which lists its flags and arguments.

## In this help

- [Opening projects](projects.md) covers `folio web`.
- [Rendering](render.md) is the web version of `folio render`.

Back to [Contents](index.md).
