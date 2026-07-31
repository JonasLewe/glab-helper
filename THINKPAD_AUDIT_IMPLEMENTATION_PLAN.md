# glab-helper: Arbeitsanweisung zur Umsetzung des Qualitätsaudits

Stand: 31.07.2026
Audit-Basis: Branch `refactor/split-glab-helper`, Commit `5c74cb3`

## Ziel

Diese Arbeitsanweisung beschreibt, wie die Befunde des Softwarequalitätsaudits
auf einem Linux-ThinkPad sicher und nachvollziehbar umgesetzt werden sollen.

Das unmittelbare Ziel ist nicht ein vollständiger Rewrite, sondern ein
verlässlicher P0-Sicherheitsstand:

- API- und Pagination-Fehler führen zum Abbruch statt zu falschen Plänen.
- `--dry-run` garantiert, dass keine schreibende Operation ausgeführt wird.
- Fehlende Jira-Daten entfernen keine bestehenden GitLab-Zuordnungen.
- Bestätigungen erfolgen vor allen Änderungen.
- Fehlgeschlagene oder teilweise erfolgreiche Abläufe liefern einen Exit-Code
  ungleich null.

Bis P0 vollständig umgesetzt und getestet ist:

> Bulk-Sync und `--dev` nicht gegen produktive GitLab-Projekte ausführen.

## 1. ThinkPad vorbereiten

### Unterstützte Werkzeuge

Benötigt werden:

- Git
- Zsh 5 oder neuer
- Bash
- GitLab CLI `glab`
- `fzf`
- `jq`
- `curl`
- ShellCheck

Unter Arch Linux:

```bash
sudo pacman -S --needed git zsh bash glab fzf jq curl shellcheck
```

Unter Debian oder Ubuntu:

```bash
sudo apt update
sudo apt install git zsh bash fzf jq curl shellcheck
```

`glab` gegebenenfalls nach der offiziellen GitLab-Anleitung installieren, falls
es in der verwendeten Distribution nicht oder nur stark veraltet verfügbar ist.

Versionen dokumentieren:

```bash
zsh --version
bash --version
git --version
glab --version
fzf --version
jq --version
curl --version
shellcheck --version
```

### Repository vorbereiten

Falls noch kein Clone vorhanden ist:

```bash
git clone git@github.com:JonasLewe/glab-helper.git
cd glab-helper
```

Danach:

```bash
git status --short --branch
git remote -v
git fetch --all --prune
git switch refactor/split-glab-helper
git status --short --branch
```

Wenn lokale Änderungen angezeigt werden, diese nicht überschreiben. Vor der
weiteren Arbeit committen oder mit einem eindeutig benannten Stash sichern.
Kein `git reset --hard` verwenden.

Für die P0-Arbeiten einen eigenen Branch anlegen:

```bash
git switch -c fix/p0-sync-safety
```

## 2. Ausgangszustand validieren

Vor jeder Änderung:

```bash
./tests/run.sh
zsh -n src/glab-helper src/lib/*.zsh src/flows/*.zsh
bash -n install.sh tests/run.sh
shellcheck install.sh
```

Erwarteter Ausgangszustand: alle vorhandenen 26 Checks bestehen.

Zusätzlich ein ausschließlich dafür vorgesehenes GitLab-Testprojekt festlegen.
Es muss entbehrliche Issues und Milestones enthalten dürfen. Projekt-ID,
Namespace und GitLab-Host in den Testnotizen dokumentieren.

## 3. P0 – vor dem nächsten produktiven Sync

Die P0-Pakete in der angegebenen Reihenfolge bearbeiten. Nach jedem Paket
Syntaxchecks und Tests ausführen und einen kleinen, fachlich eindeutigen Commit
erstellen.

### P0.1 – GitLab-Reads müssen „fail closed“ arbeiten

Betroffene Dateien:

- `src/lib/common.zsh`
- `src/lib/gitlab_api.zsh`
- `src/flows/sync_stories.zsh`
- `src/flows/create_issue.zsh`
- `src/flows/work_issue.zsh`

Aufgaben:

1. `safe_json` nicht mehr dazu verwenden, fehlgeschlagene Requests in `[]`
   umzuwandeln.
2. Rückgabestatus von `glab api` immer prüfen.
3. JSON anschließend mit `jq -e 'type == "array"'` beziehungsweise einem
   passenden Schema validieren.
4. Für GitLab-Listen bevorzugt `glab api --paginate` verwenden, statt Seiten
   selbst anhand der Elementanzahl zu erraten.
5. Jeder Fetcher muss bei Request-, Pagination- oder Schemafehlern ungleich null
   zurückgeben.
6. Jeder Aufrufer muss diesen Fehler behandeln und vor Preview oder Apply
   abbrechen.
7. Fehlermeldungen dürfen Token nicht enthalten, sollen aber Ressource,
   Operation und Ursache nennen.

Abnahmetests:

- Fehler auf Seite 1: Abbruch und keine Mutation.
- Fehler auf einer späteren Seite: Abbruch statt Verwendung der Teilmenge.
- HTML oder ungültiges JSON als Antwort: Abbruch.
- HTTP 401, 403, 429 und 500: eindeutiger Fehler und Exit-Code ungleich null.
- Snapshot-Export meldet bei einem fehlgeschlagenen Read niemals Erfolg.

### P0.2 – Jira-Pagination korrigieren

Betroffene Datei:

- `src/lib/jira_api.zsh`

Aktuelles Problem: Der Offset wird immer um die angeforderten 100 erhöht.
Jira darf serverseitig beispielsweise nur 50 Einträge liefern. Dadurch wird die
Seite ab Offset 50 übersprungen.

Aufgaben:

1. `startAt`, die tatsächliche Länge von `.issues`, optional `maxResults`,
   optional `total` und gegebenenfalls `isLast` aus der Response auswerten.
2. Den nächsten Offset aus dem gelieferten `startAt` plus der tatsächlichen
   Anzahl gelieferter Issues berechnen.
3. `total` nicht als garantiert vorhanden behandeln.
4. Eine leere Seite sicher beenden; wenn sie einem bekannten `total`
   widerspricht, als unvollständigen Read behandeln.
5. Schutz gegen Endlosschleifen einbauen: Der nächste Offset muss größer als
   der vorherige sein.
6. Das Response-Schema vollständig validieren.

Abnahmetests:

- Angefordert 100, geliefert 50, insgesamt 150:
  Es müssen die Offsets `0`, `50` und `100` gelesen werden.
- `total` fehlt: Alle Seiten werden trotzdem vollständig gelesen.
- Eine Zwischenseite ist leer oder ungültig: Der Sync bricht ab.
- Jira verändert `total` zwischen zwei Requests: kein Skip und keine
  Endlosschleife.

### P0.3 – Epic-Ausfall darf keine Milestones entfernen

Betroffene Datei:

- `src/flows/sync_stories.zsh`

Aufgaben:

1. Explizit speichern, ob der Epic-Read vollständig erfolgreich war.
2. Bei fehlgeschlagenem Epic-Read keinerlei Milestone-Create-, Update-,
   Remove- oder Reassignment-Plan erzeugen.
3. Bestehende Milestones auf synchronisierten Issues unverändert lassen.
4. Im Preview deutlich anzeigen, dass Milestone-Änderungen wegen fehlender
   Quelldaten ausgesetzt wurden.
5. Optional den gesamten Story-Sync abbrechen. Das ist sicherer als ein
   Teil-Sync und daher für P0 die bevorzugte Variante.

Abnahmetest:

- Ein bestehendes Issue besitzt einen Milestone, der Jira-Story-Read gelingt,
  der Epic-Read scheitert: Es darf kein `milestone_id=0`, kein Milestone-Update
  und keine Milestone-Erstellung aufgerufen werden.

### P0.4 – `--dry-run` zu einer echten Write-Barriere machen

Betroffene Dateien:

- `src/glab-helper`
- alle Flow-Dateien
- `tests/run.sh`

Aufgaben:

1. `--dry-run` muss den gesamten Prozess in einen schreibgeschützten Modus
   versetzen.
2. Im Dry-run dürfen keine GitLab-POST-, PUT-, PATCH- oder DELETE-Aufrufe,
   `glab issue create/update/close`, `glab label create` oder schreibende
   Git-Kommandos ausgeführt werden.
3. Im Dry-run-Menü keine parallel auswählbare echte Sync-Aktion anzeigen.
4. Einen zentralen Guard wie `require_writes_allowed` vor jeder Mutation
   verwenden. Nicht nur auf die Menüführung vertrauen.
5. Unbekannte CLI-Argumente mit Usage und Exit-Code ungleich null ablehnen.
6. `--help` und später `--version` müssen ohne Netzwerkzugriff funktionieren.

Abnahmetests:

- Alle externen Kommandos in einem Dry-run protokollieren und bestätigen, dass
  kein schreibender Aufruf enthalten ist.
- Derselbe Test muss auch bei `--dev --dry-run` bestehen.
- Ein Tippfehler wie `--dry-rnu` darf das normale Menü nicht starten.

### P0.5 – Reset aus dem normalen Produktworkflow entfernen

Betroffene Datei:

- `src/glab-helper`

Bevorzugte Lösung:

1. Reset vollständig aus dem normalen TUI entfernen.
2. Falls er für Integrationstests benötigt wird, in ein separates
   `tests/reset-test-project.sh` verschieben.
3. Das Testskript nur ausführen, wenn alle folgenden Werte exakt passen:
   - erwarteter GitLab-Host,
   - explizit allowlistete Projekt-ID,
   - erwarteter Namespace,
   - zusätzliche Umgebungsvariable wie
     `GLAB_HELPER_ALLOW_DESTRUCTIVE_TEST_RESET=yes`,
   - Eingabe des vollständigen Projektpfads.
4. Vor dem Löschen einen verifizierten Export verlangen.
5. Fehler einzeln zählen und bei jedem Fehlschlag ungleich null zurückgeben.

Ein einfacher `--dev`-Schalter darf niemals genügen, um alle Issues und
Milestones eines beliebigen Projekts zu löschen.

### P0.6 – Mutationen erst nach Bestätigung ausführen

Betroffene Dateien:

- `src/flows/create_issue.zsh`
- `src/flows/sync_stories.zsh`
- `src/flows/sync_epics.zsh`

Aufgaben:

1. Vor der Bestätigung ausschließlich lesen und einen Plan aufbauen.
2. Labels und Milestones im Jira-Create-Flow erst nach der finalen
   Issue-Bestätigung erstellen oder aktualisieren.
3. Nach einem Abbruch darf kein externer Zustand verändert worden sein.
4. Abhängigkeiten in dieser Reihenfolge anwenden und jeweils verifizieren:
   Milestone, Labels, Issue, optional Status.
5. Wenn eine benötigte Abhängigkeit fehlschlägt, das abhängige Issue nicht
   unvollständig erstellen.

Abnahmetest:

- Jira-Issue auswählen, Editor beenden und bei der finalen Bestätigung
  abbrechen: Es darf kein Label, Milestone oder Issue verändert worden sein.

### P0.7 – Retries und Exit-Codes korrigieren

Betroffene Dateien:

- `src/lib/common.zsh`
- `src/flows/sync_epics.zsh`
- `src/flows/sync_stories.zsh`
- `src/glab-helper`

Aufgaben:

1. Automatische Retries zunächst nur für sichere Reads und idempotente Updates
   zulassen.
2. Exponentielles Backoff mit Obergrenze und optionalem Jitter verwenden.
3. Bei HTTP 429 `Retry-After` respektieren.
4. POST nach einem unklaren Ergebnis nicht blind wiederholen.
5. Vor einem POST-Retry anhand eines stabilen Jira-Markers prüfen, ob das Objekt
   inzwischen existiert.
6. Teilfehler im Ergebnisbericht ausgeben und mit Exit-Code ungleich null
   beenden.
7. Im Dispatcher den Rückgabestatus der Flow-Funktion übernehmen; kein
   bedingungsloses `exit 0` nach Sync oder Snapshot.

Abnahmetests:

- Server erstellt ein Issue, die Clientantwort schlägt fehl: kein zweites Issue.
- Ein von zehn Issues schlägt fehl: Zusammenfassung zeigt `1 failed`, Prozess
  endet ungleich null.
- Snapshot-Fehler wird vom Hauptprozess weitergegeben.

## 4. P1 – Datenmodell, Secrets und Wartbarkeit

P1 erst beginnen, wenn sämtliche P0-Abnahmetests grün sind.

### P1.1 – Stabile Sync-Identität und Konflikterkennung

1. Jira-Keys nicht ausschließlich aus sichtbaren Titeln ermitteln.
2. Einen versionierten Marker verwenden, beispielsweise in der Beschreibung:

   ```html
   <!-- glab-helper:v1 source=jira key=PROJ-123 -->
   ```

3. Vorhandene titelbasierte Zuordnungen über einen expliziten
   Migrations-/Adoptionsschritt übernehmen.
4. Mehrere GitLab-Issues für denselben Jira-Key als Konflikt melden und nicht
   willkürlich das erste aktualisieren.
5. Milestones nur über Jira-Marker übernehmen. Ein gleichnamiger manueller
   Milestone darf nicht automatisch überschrieben werden.
6. Aktive und geschlossene Milestones bei der Zuordnung berücksichtigen.

### P1.2 – Feld-Ownership definieren

Für jedes Feld dokumentieren und implementieren:

| Feld | Empfohlene Regel |
|---|---|
| Titel | Jira ist Quelle, lokaler Präfix/Marker bleibt stabil |
| Beschreibung | Entweder Jira-only oder getrennte Jira-/GitLab-Sektionen |
| Jira-Labels | Jira ist Quelle; entfernte Jira-Labels werden entfernt |
| Manuelle GitLab-Labels | bleiben erhalten |
| Status | konfigurierbare monotone Abbildung |
| Milestone | nur ändern, wenn Epic-Daten vollständig und eindeutig sind |
| Assignee | manuell in GitLab, sofern nicht ausdrücklich konfiguriert |

Mit einem gespeicherten Hash des zuletzt synchronisierten Quellzustands können
gleichzeitige manuelle Änderungen als Konflikt erkannt werden.

### P1.3 – Jira-Schema konfigurierbar machen

Nicht mehr hart codieren:

- `customfield_10000`
- `Story`
- `Epic`
- englische Statusnamen
- Prioritätsdarstellung
- gewünschte OR-/AND-Semantik der Board-Labels

Eine versionierte Konfigurationsdatei vorsehen, zum Beispiel:

```text
~/.config/glab-helper/config.toml
```

Projektbezogene Werte dürfen zusätzlich aus einer Datei im Repository gelesen
werden, aber niemals Secrets enthalten.

Custom-Field-IDs sollen über Jira-Metadaten ermittelt oder ausdrücklich
konfiguriert und beim Start validiert werden.

### P1.4 – Secrets härten

1. Jira-PAT nicht länger als gemeinsam abrufbare GitLab-CI-Variable auf
   Entwicklerrechner verteilen.
2. Für interaktive Nutzung einen individuellen Jira-PAT im OS-Keychain oder
   Linux Secret Service speichern.
3. Für zentralen Sync einen dedizierten Jira-Service-Account mit minimalen
   Leserechten verwenden.
4. Nur HTTPS erlauben und Jira-Hosts allowlisten.
5. Token nicht als sichtbares Prozessargument an `curl` übergeben.
6. Token-Ablauf, Rotation und Widerruf dokumentieren.
7. Die in der README verlangten GitLab-Scopes auf tatsächlich notwendige
   Berechtigungen reduzieren.

### P1.5 – Snapshot semantisch und technisch verbessern

1. Snapshot nicht als „Backup“ bezeichnen, solange kein getesteter Restore
   existiert.
2. Außerhalb des Git-Repositories speichern, zum Beispiel unter:

   ```text
   ~/.local/state/glab-helper/snapshots/
   ```

3. `umask 077` verwenden.
4. Schema-Version, Tool-Version, Projekt-ID, Host, Zeitstempel und Checksummen
   in ein Manifest schreiben.
5. Atomar über temporäre Dateien schreiben.
6. Namenskollisionen bei zwei Exports in derselben Sekunde verhindern.
7. Einen Validierungsbefehl implementieren.
8. Entweder einen getesteten Restore ergänzen oder klar als Diagnoseexport
   dokumentieren.

### P1.6 – Work-on-Issue-Flow stabilisieren

1. `glab issue list --all` oder eine vollständige API-Pagination verwenden.
2. API-Fehler nicht als „No open issues“ behandeln.
3. Die aktuelle Issue-Repräsentation nach jeder Mutation neu laden.
4. Endlosschleife durch `exec "$0"` bei wiederholtem Read-Fehler entfernen.
5. Entfernen aller Labels als explizite Aktion ermöglichen.
6. Den GitLab-Remote dynamisch ermitteln und `origin` nicht fest voraussetzen.
7. Branch-Namen mit `git check-ref-format --branch` validieren.

## 5. Tests und CI

### Teststruktur

Die vorhandenen Smoke-Tests behalten, aber gemeinsame Stubs und Fixtures
extrahieren. Ergänzen:

- Unit-Tests für `jira_to_markdown`, `slugify`, CSV-/Label-Logik und
  Status-Mapping.
- Contract-Tests für Jira- und GitLab-Response-Schemas.
- Failure-Path-Tests für sämtliche P0-Fälle.
- Einen Integrationstest gegen ein ausschließlich dafür vorgesehenes
  GitLab-Testprojekt.
- Wiederholungstest: Ein zweiter identischer Sync muss ein No-op sein.
- Parallelitätstest: Zwei Planer dürfen nicht unkontrolliert dieselben Objekte
  erzeugen.

`tests/run.sh` darf keine gemeinsam verwendete feste Datei wie
`/tmp/glab-helper-test.out` benutzen. Pro Lauf ein eigenes `mktemp`-Verzeichnis
verwenden.

### CI

Zunächst entscheiden, ob GitHub oder GitLab die kanonische Hosting- und
CI-Plattform ist. Danach genau dort eine verbindliche Pipeline einrichten.

Die Pipeline muss mindestens ausführen:

```bash
zsh -n src/glab-helper src/lib/*.zsh src/flows/*.zsh
bash -n install.sh tests/run.sh
shellcheck install.sh
./tests/run.sh
```

Empfohlene Matrix:

- aktuelle Arch-/Rolling-Umgebung,
- aktuelle Ubuntu-LTS-Umgebung,
- mindestens unterstützte Zsh- und fzf-Version,
- aktuelle und festgelegte minimale `glab`-Version.

Außerdem ergänzen:

- `.gitignore`, insbesondere für Snapshots und lokale Konfiguration,
- `LICENSE` mit tatsächlichem MIT-Text,
- `CHANGELOG.md`,
- `SECURITY.md`,
- maschinenlesbare Tool-Version und `--version`,
- dokumentierte Mindestversion für `glab`.

## 6. P2 – Entscheidung über die langfristige Architektur

Nach P0 und P1 anhand der tatsächlichen Nutzung entscheiden.

### Option A: Zsh beibehalten

Geeignet, wenn:

- wenige Nutzer beteiligt sind,
- alle Abläufe interaktiv bleiben,
- nur ein Jira-/GitLab-Schema unterstützt wird,
- kein zentraler oder zeitgesteuerter Sync benötigt wird.

Vorteil: geringer Migrationsaufwand.
Nachteil: globale Shell-Zustände, Fehlerbehandlung und komplexe Datenmodelle
bleiben wartungsintensiv.

### Option B: Go-Sync-Engine mit dünner TUI

Empfohlene Zieloption, sobald das Tool teamweit, automatisiert oder für mehrere
Projekte verwendet wird.

Schichten:

```text
CLI/TUI
  -> Planner mit reinen, typisierten Datenmodellen
  -> Jira- und GitLab-Adapter
  -> Executor mit Lock, Journal und Verifikation
```

Der Planner soll einen serialisierbaren Plan erzeugen:

```text
glab-helper sync plan --output plan.json
glab-helper sync apply --plan plan.json
```

Vor Apply werden Projekt-ID, Quellstand und Plan-Hash erneut validiert. Das
lokale Branch- und Issue-TUI kann zunächst in Zsh bleiben und später denselben
Go-Kern aufrufen.

### Option C: Zentraler Sync in CI oder als Service

Geeignet für:

- geplante Synchronisation,
- zentrale Secrets,
- genau einen aktiven Writer,
- Audit-Logs und geschützte Freigaben.

Empfohlener Workflow:

1. Plan-Job läuft read-only.
2. Plan wird als Artefakt gespeichert.
3. Mensch prüft den Plan.
4. Geschützter manueller Apply-Job führt genau diesen Plan aus.
5. Ergebnis und Verifikation werden als Artefakt gespeichert.

Die lokale CLI bleibt für Branches und interaktive Arbeit zuständig.

### Option D: Jira nicht mehr nach GitLab spiegeln

Vor einem größeren Rewrite fachlich prüfen, ob doppelte Issues und Milestones
wirklich benötigt werden.

Wenn Jira alleinige Arbeitsquelle bleiben kann:

- native GitLab-Jira-Integration nutzen,
- Jira-Key in Branches, Commits und Merge Requests verwenden,
- GitLab-Issues nicht mehr spiegeln.

Dies beseitigt den größten Teil der Konflikt- und Ownership-Probleme, bildet
aber keine Jira-Epics als GitLab-Milestones nach.

## 7. Sichere Einführung

Nach Abschluss von P0:

1. Alle lokalen und CI-Tests ausführen.
2. Dry-run gegen das disposable Testprojekt ausführen.
3. Das Kommando-Log auf Schreiboperationen prüfen.
4. Einen einzelnen Jira-Vorgang synchronisieren.
5. GitLab-Issue, Labels, Milestone, Beschreibung und Status manuell prüfen.
6. Denselben Sync wiederholen; Ergebnis muss ein No-op sein.
7. Einen simulierten Jira- und GitLab-Ausfall testen.
8. Einen kleinen Batch ausführen und verifizieren.
9. Erst danach einen produktiven Pilot mit wenigen Issues freigeben.
10. Produktive Aktivierung dokumentieren und eine verantwortliche Person für
    Rollback und Token-Widerruf benennen.

## 8. Definition of Done

P0 gilt nur dann als abgeschlossen, wenn:

- [ ] Kein fehlgeschlagener Read wird als leere erfolgreiche Antwort behandelt.
- [ ] Jira-Pagination überspringt bei serverseitigen Limits keine Seite.
- [ ] Epic-Ausfälle verändern keine Milestone-Zuordnungen.
- [ ] `--dry-run` führt garantiert keine Mutation aus.
- [ ] Reset ist aus dem normalen Produktworkflow entfernt oder streng
      allowlistet.
- [ ] Ein Abbruch vor Bestätigung hinterlässt keinerlei externe Änderungen.
- [ ] POSTs werden nach unklarem Ausgang nicht blind wiederholt.
- [ ] Teilfehler und Snapshot-Fehler liefern Exit-Code ungleich null.
- [ ] Alle neuen Fehlerpfadtests bestehen.
- [ ] Der zweite identische Sync ist ein No-op.
- [ ] Die Änderungen wurden ausschließlich gegen ein Testprojekt validiert,
      bevor ein produktiver Pilot beginnt.

## Referenzen

- GitLab CLI Pagination:
  <https://docs.gitlab.com/cli/api/>
- GitLab REST-Pagination:
  <https://docs.gitlab.com/api/rest/>
- GitLab Rate Limits:
  <https://docs.gitlab.com/administration/settings/user_and_ip_rate_limits/>
- GitLab Project Variables API:
  <https://docs.gitlab.com/api/project_level_variables/>
- Jira Data Center REST und Pagination:
  <https://developer.atlassian.com/server/jira/platform/about-the-jira-server-rest-apis/>
- Jira Custom Fields:
  <https://developer.atlassian.com/server/jira/platform/jira-rest-api-examples/>
- GitLab-Jira-Integration:
  <https://docs.gitlab.com/integration/jira/configure/>
