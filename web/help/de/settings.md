# Konfigurationsseiten und Einstellungsdatei

## Die Schaltfläche Config

`Config` ganz rechts in der oberen Leiste öffnet eine Seite, die die Konfigurationsbereiche auflistet. Mit der Schaltfläche `App` oben kehrst du zurück. Über den Bereichen nennt ein Kasten `Release` die laufende folio-Version und bietet die Schaltfläche `Check for updates` (zu den automatischen Prüfungen siehe [Die Einstellungsdatei](#die-einstellungsdatei)). Gibt es eine neuere Version, sagt der Kasten es und bietet die Schaltfläche **Update to …**, die sie installiert wie die Schaltfläche **Update** in der oberen Leiste. Nach dem Update startet folio web mit der neuen Version neu, und die Seite lädt sich neu. Einem Entwicklungs-Build wird die neueste Version angeboten, welche es auch ist. Die Bereiche sind:

- `Preview: Custom CSS`: das Stylesheet für die [Vorschau](preview.md).
- `Copyedit: Editing tasks` und `Copyedit: API connections`: siehe [Lektorat](copyedit.md).
- `External editor`: siehe [andere Editoren](external-editor.md).
- `Comments: Your name`: der Name, mit dem deine [Kommentare](editor.md#kommentare) und Antworten gezeichnet werden.
- `Project: folio.yml`: die Konfigurationsdatei des geöffneten Projekts, als Text.
- `folio: config.cue`: die unten beschriebene Einstellungsdatei, als Text.

## Project: folio.yml

Bearbeitet die `folio.yml` (oder die ältere `qm.yml`) des geöffneten Projekts in einem Texteditor. `Save` prüft die Datei so, wie ein Rendern sie liest: Ein unbekannter Schlüssel oder ein Ausgabetyp, den folio nicht schreiben kann, wird mit seiner Zeile abgelehnt, und die Datei bleibt, wie sie war. Hat das Projekt keine, legt das Speichern `folio.yml` an. Eine geänderte Adresse, auf der `folio web` lauscht, gilt ab dem nächsten Start.

## folio: config.cue

Bearbeitet [die Einstellungsdatei](#die-einstellungsdatei) in einem Texteditor. `Save` prüft die Einstellungen gegen das Schema und setzt sie sofort in Kraft. API-Schlüssel werden nie angezeigt: Lass `(stored key, not shown)` stehen, um einen Schlüssel zu behalten, oder überschreibe es mit einem neuen. Es gilt dasselbe wie beim [Bearbeiten von Hand](#von-hand-bearbeiten): Kommentare zwischen den Einstellungen bleiben nicht erhalten.

## Comments: Your name

`folio web` läuft auf deinem eigenen Rechner, daher gibt es keine Anmeldung. Die Kommentare und Antworten, die du im Editor schreibst, werden mit dem Namen gezeichnet, den du hier einträgst, zusammen mit dem Datum. Bleibt das Feld leer, wird der Name verwendet, mit dem du am Rechner angemeldet bist. Doppelte Anführungszeichen, geschweifte Klammern und Zeilenumbrüche sind im Namen nicht erlaubt.

## Preview: Custom CSS

Hier schreibst du CSS, das die eingebauten Stile der Vorschau überschreibt. Es wird dokumentweit nach dem eigenen Stylesheet der App geladen. Damit es nur die Vorschau betrifft, begrenze deine Regeln mit `.markdown-preview`, zum Beispiel `.markdown-preview h1 { color: rebeccapurple; }`.

- Drücke `Save`, um es zu speichern. Ziehe an der Ecke des Editors, um ihn höher oder niedriger zu machen; die Höhe wird gemerkt.
- Um ein weiteres Stylesheet hinzuzufügen, tippe unter `Add an alternate stylesheet` einen Namen wie `dark.css` und drücke `Add stylesheet`. Bei mehr als einem Stylesheet wechselt ein Dropdown `Editing:` zwischen ihnen.
- Das aktive Stylesheet wählst du mit dem Dropdown unter der Vorschau.
- Regeln wie `.folio.pol:before { content: "..." }` liefern auch die Symbole, die der [Seitenbaum](tree.md) für Spans zeigt.

Stylesheets sind Dateien mit dem Namen `<user config dir>/folio/custom-css/*.css`. Eine Standarddatei `custom.css` wird beim ersten Start von `folio web` angelegt.

## Die Einstellungsdatei

Alles, was sich `folio web` merkt, steht in einer einzigen Datei: `<user config dir>/folio/config.cue`. Sie enthält:

- die zuletzt geöffneten Projekte,
- die Render-Auswahl und die zuletzt geöffnete Seite jedes Projekts,
- das aktive Vorschau-Stylesheet,
- die Lektorat-Aufgaben und -Verbindungen (einschließlich API-Schlüsseln),
- die Kommandozeilen des externen Editors,
- der Name, mit dem deine Kommentare gezeichnet werden (`user: name:`).
- wo und wie oft nach einer neuen folio-Version gesucht wird (`update: source:` und `update: check:`, das `daily`, `weekly` oder `never` ist). Findet `folio web` eine, erscheint rechts in der oberen Leiste ein **Update**-Knopf; er installiert die Version, die ab dem nächsten Start von `folio web` gilt.
- die Titel von Callouts ohne eigenen Titel, je Sprache (`callouts:`). Die Sprache ist `lang` aus der `folio.yml` des Projekts; `folio render`, `folio preview` und die Vorschau in `folio web` verwenden sie alle. Englisch und Deutsch sind vorbelegt; eine weitere Sprache kommt so hinzu, und ein weggelassener Typ bleibt englisch:

  ```
  callouts: fr: {note: "Remarque", tip: "Astuce", warning: "Avertissement", important: "Important", caution: "Attention"}
  ```

  Ein regionales `lang` wie `fr-CA` nimmt `fr`, solange es keinen eigenen Eintrag hat.

Wegen der Schlüssel ist sie nur für dich lesbar. Die Datei wird beim ersten Start mit Standardwerten angelegt. Ältere JSON-Einstellungsdateien werden einmalig übernommen und in `*.migrated` umbenannt.

## Von Hand bearbeiten

Du darfst die Datei selbst bearbeiten.

- Sie ist in CUE geschrieben und beginnt mit ihrem eigenen Schema, das also sagt, was hineingehört. Die App schreibt das Schema bei jedem Speichern neu.
- Weggelassene Felder nehmen ihre Standardwerte an. Weggelassene Listen und Maps sind leer.
- Ein falsch geschriebener Feldname oder ein falscher Wert ist ein Fehler, der mit Datei, Zeile und Spalte gemeldet wird.
- Kommentare, die du zwischen die Einstellungen schreibst, gehen beim nächsten Speichern der App verloren.

## Fehler halten die App an

Besteht die Datei die Prüfung nicht, hält `folio web` an, statt sie zu ersetzen, denn sie ist die einzige Kopie deiner Verbindungen und Auswahlen. Behebe die gemeldete Zeile und starte erneut. Ein Speichern, das die App selbst nicht wieder einlesen konnte, wird abgelehnt.

Zurück zum [Inhalt](index.md).
