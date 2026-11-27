# Der Seitenbaum

Die linke Seite des Fensters zeigt die `.md`-Seiten (und älteren `.qmd`-Seiten) des Projekts als Baum, in der Reihenfolge der Website und nach Ordnern verschachtelt. Der Ordner `assets` auf oberster Ebene des Projekts und Ordner, deren Name mit `_` oder `.` beginnt, fehlen samt ihrem Inhalt. Klicke auf einen Titel, um die Seite im [Editor](editor.md) zu öffnen.

## Titel

Der Titel einer Seite stammt in dieser Reihenfolge aus:

- dem `title:` in ihrem Frontmatter,
- dem Text ihrer ersten Markdown-Überschrift,
- einem aus dem Dateinamen gebildeten Titel.

Der Attributblock einer Überschrift, etwa `{.unnumbered}`, wird aus dem Titel weggelassen. Ein Quarto-Span wie `[Polizei-]{.pol}` wird als das Symbol angezeigt, das seine Klasse im aktiven [eigenen Vorschau-Stylesheet](preview.md) trägt, gefolgt vom Text des Spans. Spans, deren Klasse kein Symbol hat, werden so angezeigt, wie sie geschrieben sind.

## Nummerierung

Die `index.md` im Projektstamm ist die Startseite. Sie steht zuoberst, ohne Nummer, und ist die einzige Seite, die kein `order:` braucht. Themenseiten (die Ordner der obersten Ebene) werden nicht nummeriert; ein Thema ohne `order:` zeigt aber weiterhin ⚠. Abschnitte werden je Thema mit 1, 2, … nummeriert, Kapitel mit 1.1, 1.2 und so weiter tiefer im Baum.

Jede Quelldatei soll mit einer h1 beginnen. In einem gedruckten Dokument (DOCX, PDF) bestimmt die Stelle einer Seite im Baum, wie ihre Überschriften gesetzt werden, und eine Überschrift, die tiefer als h6 läge, wird zu hervorgehobenem Text. Eine Seite, deren Überschriften auf ihrer Ebene als h7 oder tiefer erschienen, bekommt einen hellgrauen Zeilenhintergrund. Ein Tooltip nennt, welche Überschrift zu tief ist. Die Seiten darüber sind ebenfalls grau, mit einem Tooltip, dass eine Seite darunter zu tiefe Überschriften hat.

## Entwürfe

Eine Seite mit `draft: true` im Frontmatter wird ausgegraut. Sie gehört zum Projekt, aber nicht zu dem, was ein Rendering veröffentlicht. Du kannst sie weiterhin öffnen, verschieben und durchsuchen.

## Statusmarken

Eine Seite kann nach ihrem Titel Statusmarken tragen:

- 🆕 neu: `status: new` im Frontmatter. Neue Seiten beginnen damit.
- 🗑️ veraltet: `status: deprecated`, `obsolete` oder `old`.
- 🚧 zu erledigen: der Text `==TODO==` irgendwo in der Seite, auch in einem HTML-Kommentar.
- 👀 zu prüfen: `review:` im Frontmatter, mit dem, was geprüft werden soll. Der Tooltip der Markierung lautet „Review: “ und dann dieser Text. `review: true` markiert die Seite, ohne zu sagen, was zu prüfen ist.
- 💬 kommentiert: die Seite enthält mindestens einen Kommentar (siehe die Hilfe zum Editor). Eine Kommentarmarke in Code zählt nicht.
- 🔖 mit Lesezeichen: `bookmarked: true`, das der Button 🔖 des Editors setzt (siehe [Eine Seite bearbeiten](editor.md)).

```yaml
---
title: Meine Seite
status: new
review: die Zahlen in Abschnitt 2
---
```

Jede Marke erscheint einmal pro Seite, egal wie viele `==TODO==` die Seite enthält.

In einem Git-Arbeitsbaum zeigt eine Seite, die seit dem letzten Commit geändert wurde, einen Stern (`*`) nach ihrem Titel.

## Seiten ausblenden

Eine Seite mit `hide: true` im Frontmatter erscheint nicht im Baum. Standard ist `false`. Eine ausgeblendete Abschnittsseite blendet ihren ganzen Zweig aus. Das Ausblenden betrifft nur den Baum: Die Seite wird weiterhin gerendert, und die [Suche](search.md) findet sie, sodass du sie dort öffnen und die Einstellung wieder entfernen kannst.

```yaml
---
title: Meine Seite
hide: true
---
```

Seiten aus früheren Versionen führen diese Schlüssel unter `folio:` oder `qm:`. Sie werden weiterhin gelesen; ein Schlüssel auf oberster Ebene gewinnt.

## Umordnen und Verschieben

Ziehe eine Seite am Griff `⠿`.

- Legst du sie zwischen ihren Geschwisterseiten ab, wird das `order`-Frontmatter der betroffenen Seiten neu geschrieben.
- Legst du sie unter einem anderen Elternelement ab, wird zusätzlich ihre Datei auf der Platte umbenannt. Die Markdown-Überschriftenebenen der Seite bleiben unverändert: Du bestimmst sie selbst.
- Wird die geöffnete Seite verschoben, folgt ihr der Editor zum neuen Pfad. Dein ungespeicherter Text bleibt unberührt, und Autosave schreibt weiter in die richtige Datei.

## Seiten anlegen

- Tippe oben den Namen der Seite und drücke dann `＋ Page`.
- Die Seite wird als `name/index.md` gespeichert. Der Ordnername ist der Name in Kleinbuchstaben, mit Bindestrichen statt Leerzeichen: Aus „Kartenansicht neu“ wird `kartenansicht-neu/index.md`.
- Eine neue Seite hat kein `title` im Frontmatter, nur ihr `order` und `status: new`. Sie beginnt mit einer Überschrift der Ebene 1 in einem `::: explanation`-Block, unabhängig von ihrer Stelle im Baum. Die Überschrift ist der Name so, wie du ihn getippt hast (`# Kartenansicht neu`), und sie ist der Titel der Seite.
- Die Schaltfläche `＋` in einer Zeile fügt unter dieser Seite eine Unterseite hinzu. Du wirst nach dem Namen gefragt.

## Seiten löschen

Die Schaltfläche `🗑` in einer Zeile fragt nach Bestätigung und verschiebt die Seite dann in den Papierkorb.

## Auf- und Zuklappen

Klicke auf den kleinen Schalter in einer Zeile, um einen Zweig zu- oder aufzuklappen. Die Schaltflächen `⊞` und `⊟` unter dem Baum klappen alle Zweige auf oder zu.

## Änderungen von außen

Der Baum prüft das Projekt auf Änderungen außerhalb des Browsers und aktualisiert sich selbst, wenn sich etwas geändert hat.

Zurück zum [Inhalt](index.md).
