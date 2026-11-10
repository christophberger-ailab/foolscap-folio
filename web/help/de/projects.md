# Projekte öffnen

Ein Projekt ist ein Dokumentbaum aus Markdown-Seiten, optional mit einer `folio.yml`. `folio web` zeigt jeweils ein Projekt.

## folio web starten

```sh
folio web                    # serve the project given by --project (default: the current directory)
folio web ../handbook        # serve that project instead
folio web --addr :9000       # listen elsewhere
```

- Ein Pfad nach `web` benennt das zu öffnende Projekt. Er hat Vorrang vor `--project`.
- Ohne Pfad ist das Projekt das, das `--project` (oder `-p`) angibt, sonst das aktuelle Verzeichnis.
- Der Befehl läuft, bis du ihn unterbrichst, und gibt die Adresse aus, unter der er erreichbar ist.

## Adresse und Port

- Standardmäßig lauscht `folio web` auf `localhost:8199`.
- Um dauerhaft woanders zu lauschen, setze `baseurl` (Hostname oder IP-Adresse) und `port` im Abschnitt `web` der [Einstellungsdatei](settings.md), zum Beispiel `web: {baseurl: "localhost", port: 8200}`. Sie werden beim Start von `folio web` gelesen.
- Ist dieser Port belegt, meist durch ein anderes `folio web`, wird der nächsthöhere probiert und so weiter, bis ein freier gefunden ist. Die tatsächlich verwendete Adresse wird ausgegeben.
- Eine Adresse, die du mit `--addr` angibst, hat Vorrang vor den Einstellungen und wird genau so verwendet. Ist sie belegt, ist das ein Fehler.

## Ein Projekt öffnen

Die obere Leiste zeigt den Titel des offenen Projekts, dann `Open`, dann `↻ Reload`.

- Der Titel ist der `title` aus der `folio.yml` des Projekts. Ohne ihn ist es der Ordnername des Projekts. Fahre mit der Maus darüber, um den vollen Pfad zu sehen.
- `Open` zeigt einen Ordnerdialog. Er beginnt in dem Ordner, aus dem du zuletzt ein Projekt geöffnet hast, mit diesem Projekt ausgewählt. Beim ersten Mal beginnt er im aktuellen Verzeichnis.
- Ordner, die Projekte sind (sie enthalten eine `folio.yml`), sind mit 📘 und fett dargestellt.
- Ein Klick wählt einen Ordner aus. Ein Doppelklick oder `Enter` wechselt hinein; bei einem Projekt öffnet es das Projekt. `↑` oder `Backspace` geht einen Ordner nach oben.
- Die Schaltfläche `Open` im Dialog öffnet den ausgewählten Ordner, oder den angezeigten, wenn keiner ausgewählt ist.
- Tippe oder füge einen Pfad in das Feld oben ein und drücke `Enter`, um dorthin zu wechseln.
- Die Liste Recent links enthält die zehn zuletzt geöffneten Projekte, das neueste zuerst. Ein Klick öffnet eines. Ein Projekt, das du über die Kommandozeile gestartet hast, gilt als geöffnet und erscheint beim nächsten Mal in der Liste.
- `Esc` oder `Cancel` schließt den Dialog.
- `↻ Reload` liest die offene Seite und den Baum neu von der Platte.

Beim Öffnen eines Projekts wird alles aktualisiert, was dazugehört: der Seitenbaum, die Bereiche, das Render-Panel, das Git-Panel und die Schaltfläche Editor.

Die Liste der zuletzt geöffneten Projekte steht in der [Einstellungsdatei](settings.md).

Zurück zum [Inhalt](index.md).
