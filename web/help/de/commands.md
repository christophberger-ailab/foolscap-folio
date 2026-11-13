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
| `folio lint [<topic>]`                     | prüft die `.md`- und `.qmd`-Quellen (derzeit: nicht geschlossene `:::`-Fences)                                                       |
| `folio web [<path>]`                       | startet die lokale Oberfläche zum Sortieren, Bearbeiten und Rendern                                                       |
| `folio upgrade` | installiert die neueste Version von folio (Alias `folio update`; `--check` meldet nur, `--yes` fragt nicht) |

Jeder Befehl kennt `--help`, das seine Flags und Argumente auflistet.

## In dieser Hilfe

- [Projekte öffnen](projects.md) behandelt `folio web`.
- [Rendern](render.md) ist die Web-Variante von `folio render`.

Zurück zum [Inhalt](index.md).
