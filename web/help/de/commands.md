# Die folio-Kommandozeile im Überblick

`folio web` ist einer von mehreren Befehlen. Sie arbeiten alle am selben Projekt.

```
folio <object> <subcommand> [flags] [args]
```

## Globales Flag

| Flag              | Umgebungsvariable | Standard | Bedeutung             |
| ----------------- | ----------------- | -------- | --------------------- |
| `--project`, `-p` | `QM_PROJECT`      | `.`      | Pfad zum Dokumentbaum |

## Befehle

| Befehl                                     | Was er tut                                                                                                                |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------- |
| `folio render [<topic>…]`                  | rendert die Matrix aus Thema × Format × Zielgruppe des Projekts                                                           |
| `folio flatten <topic>`                    | schreibt das Markdown, das ein Dokument-Rendering des Themas einliest, nach `_build/<topic>.md`, ohne zu rendern          |
| `folio migrate`                            | schreibt `folio.yml` aus den alten `_quarto*.yml`-Dateien des Projekts (`--dry-run` gibt sie aus, `--force` überschreibt) |
| `folio lint [<topic>]`                     | prüft die `.md`- und `.qmd`-Quellen (derzeit: nicht geschlossene `:::`-Fences)                                                       |
| `folio chapters insert <folder>`           | gibt einer neuen Seite ihr `order:` und nummeriert die Geschwisterseiten neu                                              |
| `folio chapters move <folder> <old> <new>` | verschiebt ein Kapitel an eine andere `order`-Position                                                                    |
| `folio chapters remove <folder> <order>`   | entfernt ein Kapitel nach Rückfrage                                                                                       |
| `folio web [<path>]`                       | startet die lokale Oberfläche zum Sortieren, Bearbeiten und Rendern                                                       |

Jeder Befehl kennt `--help`, das seine Flags und Argumente auflistet.

## In dieser Hilfe

- [Projekte öffnen](projects.md) behandelt `folio web`.
- [Rendern](render.md) ist die Web-Variante von `folio render`.

Zurück zum [Inhalt](index.md).
