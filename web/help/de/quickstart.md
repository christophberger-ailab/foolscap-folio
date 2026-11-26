# Schnellstart

Der kürzeste Weg zu jedem ersten Schritt. Die übrigen Themen haben die Einzelheiten.

## folio installieren

- **Windows:** `folio_<version>_windows_setup.exe` aus dem neuesten Release auf GitHub herunterladen und ausführen. Es braucht keine Administratorrechte und legt **Foolscap Folio** im Startmenü an.
- **macOS und Linux:** `brew install christophberger-ailab/tap/folio`.
- **Windows mit Scoop:** `scoop bucket add folio https://github.com/christophberger-ailab/scoop-bucket`, danach `scoop install folio`.
- **Von Hand:** das Programm für dein System aus einem Release entpacken und `folio` in den `PATH` legen.

`folio upgrade` bringt folio später auf den neuesten Stand oder verweist auf den Paketmanager, der es installiert hat.

## Ein Projekt öffnen

```sh
folio web pfad/zum/projekt
```

Oder `folio web` irgendwo starten und in der oberen Leiste `Open` drücken: den Ordner des Projekts wählen oder eines aus der Liste Recent. `From a repository…` im selben Dialog klont ein Git-Repository und öffnet es. Siehe [Projekte öffnen](projects.md).

## Ein neues Projekt anlegen

```sh
mkdir handbuch
cd handbuch
folio init
folio web
```

In einem leeren Ordner schreibt `folio init` ein Beispielprojekt als Ausgangspunkt: eine Startseite, ein Thema `handbook` mit Abschnitten und Kapiteln, eine `folio.yml` und eine `_brand.yml`. In einem Ordner mit Markdown-Seiten schreibt es nur `folio.yml` und `_brand.yml`, mit jedem Inhaltsordner als Thema. Jede Seite beginnt mit Front Matter:

```markdown
---
title: Installation
order: 2
---

# Installation
```

## Die Markdown-Erweiterungen von folio

### Blöcke und Spans

Ein umzäunter Block oder ein Span in eckigen Klammern mit einer Klasse bleibt nur in den Formaten und Zielgruppen erhalten, die die Klasse zeigen:

```markdown
::: pol
Nur für die Zielgruppe `pol`.
:::

Die Feuerwehr [(nur Feuerwehr)]{.fw} trifft zuerst ein.

- Punkt 1
[- Punkt 2 nur für POL]{.pol}
- Punkt 3
```

Eine Klasse, die kein Format und keine Zielgruppe in `folio.yml` unter `show:` aufführt, ist eine gewöhnliche Klasse zum Gestalten.

### Callouts

```markdown
::: {.callout-tip title="Oft speichern"}
`folio web` speichert beim Tippen.
:::
```

Die Typen sind `note`, `tip`, `warning`, `important` und `caution`. `collapse="true"` klappt den Callout unter seinen Titel zusammen, `appearance="simple"` lässt die Schattierung weg, `icon=false` das Symbol.

### Querverweise

Ein Link auf eine Überschrift oder eine Abbildung über ihren Schlüssel setzt Nummer und Namen des Ziels ein:

```markdown
## Installation {#install}

Siehe [](#install), [](#install){.name}, [](#install){.number} oder [diesen Abschnitt](#install).
```

Eine Überschrift ohne eigenen Bezeichner hat ihren Text als Schlüssel, kleingeschrieben, Leerzeichen als Bindestriche, Sonderzeichen weggelassen: `## Die (nicht so ferne) Zukunft` ist `#die-nicht-so-ferne-zukunft`. Umlaute bleiben, `## Einführung` ist `#einführung`, aber auch `#einfuehrung` und `#einfuhrung` finden sie. Für eine andere Seite steht deren `id:` (oder ihr Pfad) davor: `[](#install:overview)`.

### Bildgröße

```markdown
![Die Karte](images/karte.png){width=50%}
![](images/logo.svg){height=2cm}
```

Größen nehmen `cm`, `mm`, `in`, `pt`, `px` (die Vorgabe), `em` oder `%`. Mit Breite oder Höhe allein folgt das andere dem Seitenverhältnis. Ein Bild mit Alt-Text allein in seinem Absatz ist eine Abbildung, mit dem Alt-Text als Bildunterschrift.

### Definitionslisten

```markdown
Callout
: Ein Kasten, der auf einen Hinweis, einen Tipp oder eine Warnung aufmerksam macht.

Span
: Ein Text in eckigen Klammern mit Attributen, `[Text]{.klasse}`.
```

Eine Definition kann mehrere Absätze haben; diese werden um vier Leerzeichen eingerückt.
