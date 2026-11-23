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
- Ein Projekt kann im Abschnitt `web` seiner `folio.yml` eine eigene Adresse angeben: `baseurl` ist der Hostname oder die IP-Adresse, `port` der Port. Zum Beispiel `web: {baseurl: localhost, port: 8200}`. Gelesen wird sie aus dem Projekt, mit dem `folio web` gestartet wird.
- Ist dieser Port belegt, meist durch ein anderes `folio web`, wird der nächsthöhere probiert und so weiter, bis ein freier gefunden ist. Die tatsächlich verwendete Adresse wird ausgegeben.
- Eine Adresse, die du mit `--addr` angibst, hat Vorrang vor der `folio.yml` und wird genau so verwendet. Ist sie belegt, ist das ein Fehler.

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

Beim Öffnen eines Projekts wird alles aktualisiert, was dazugehört: der Seitenbaum, die Bereiche, darunter Render und Git, und die Schaltfläche Editor.

Die Liste der zuletzt geöffneten Projekte steht in der [Einstellungsdatei](settings.md).

## Ein Projekt aus einem Repository öffnen

Du kannst auch mit einem Git-Repository beginnen statt mit einem Ordner auf der Platte.

1. Drücke `Open`, dann `From a repository…`.
2. Füge die Adresse des Repositorys ein, wie die Hosting-Seite sie zeigt: `https://github.com/owner/book.git` oder `git@github.com:owner/book.git`.
3. Prüfe den Ordner, in den geklont wird. Vorgegeben ist der Ordner, den der Dialog zeigte. Der Klon landet darin in einem neuen Ordner, der nach dem Repository benannt ist.
4. Drücke `Clone and open`.

Hat das Repository oben eine `folio.yml`, wird es sofort geöffnet. Sonst zeigt der Dialog den geklonten Ordner, damit du den Unterordner mit dem Buch auswählen kannst.

Danach ist es ein gewöhnliches Projekt in einem eigenen Ordner: Committen, Pushen und Synchronisieren gehen im [Git-Bereich](git.md) oder im Terminal.

### Bei GitHub anmelden

Ein öffentliches Repository lässt sich ohne Anmeldung klonen. Für ein privates und zum Zurückpushen muss git wissen, wer du bist. Hast du keine Git-Zugangsdaten eingerichtet, drücke im Formular `Sign in to GitHub`:

1. folio zeigt einen kurzen Code und einen Link zu `github.com/login/device`.
2. Öffne den Link, gib den Code ein und erlaube folio den Zugriff.
3. Das Formular zeigt, als wer du angemeldet bist.

folio merkt sich die Anmeldung für den nächsten Start, in einer Datei neben der [Einstellungsdatei](settings.md), die nur du lesen kannst, und schickt sie nur an github.com. `Sign out` vergisst sie. Um folio den Zugriff auch bei GitHub zu entziehen, widerrufe ihn auf github.com unter Settings → Applications.

Zurück zum [Inhalt](index.md).
