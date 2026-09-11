# Simulation der GPU-Erkennung

Ausgeführt mit dem echten Shrike-Engine-, GPU-Detektor-, Watch- und
Doctor-Rendering-Code. Nur Prozessliste, GPU-Messwerte und Zeitstempel sind
künstlich. Es wurde keine GPU-Last erzeugt, kein Prozess signalisiert und keine
macOS-Benachrichtigung versendet. Die Benachrichtigungen sind Vorschauen der
echten Watch-Entscheidung. Simulierte 95 Sekunden laufen ohne echte Wartezeit.

## Ablauf und gemessenes Ergebnis

| Simulierte Zeit | GPU | Ergebnis | Neue Benachrichtigung |
|---|---:|---|---|
| 0 s | 12 % | Unauffällig | Nein |
| 5 s | 99 % | Medium: hohe Momentaufnahme | Nein |
| 10 s | 15 % | Spitze vorbei; Bestätigung zurückgesetzt | Nein |
| 15–40 s | 97–99 % | Medium: erneute hohe Messwerte, noch keine 30 s erreicht | Nein |
| 45 s | 99 % | High: sieben hohe Messwerte über 30 s; Systemwarnung und Prozesskandidat | Ja, eine zusammengefasste Meldung |
| 50 s | 98 % | Beide Befunde bestehen weiter | Nein, bereits gemeldet |
| 55 s | 99 % | Prozesszähler fehlen; Systemwarnung bleibt | Nein, bereits gemeldet |
| 60 s | 12 % | Erholung; Befunde verschwinden | Nein |
| 65–80 s | 99 % | Neuer Überlastfall, zunächst Medium | Nein |
| 95 s | 99 % | Erneut High nach 30 s; Systemwarnung und Kandidat | Ja |

Ein zweiter Durchlauf beginnt ganz ohne Prozesszähler: Nach drei hohen
Messwerten über 30 Sekunden entsteht trotzdem eine High-Systemwarnung mit
Benachrichtigung. Es wird kein Verursacher erfunden.

## Tatsächlich erzeugte Doctor-Zeilen bei 45 Sekunden

```text
Simulated GPU       99.0% GPU · system High
  GPU 99.0%; high in 7 observations over 30s; cause not established

Simulated Renderer  PID 4242 · GPU time +5000 ticks / 5.0s High
  GPU-active candidate: +5000 GPU-time ticks in 5.0s; may be legitimate work
```

Der fiktive Renderer hat in der Prozessliste nur 0,1 % CPU-Auslastung. Er wird
wegen seiner GPU-Aktivität als Kandidat erkannt. Die 5000 GPU-Ticks sind frei
gewählte Simulationswerte, keine Prozent- oder Nanosekundenangabe.

Benachrichtigungsvorschau bei bekannter Zuordnung:

```text
Shrike: 2 new issues
🎮 Simulated GPU, 🎮 Simulated Renderer
```

Benachrichtigungsvorschau ohne Prozesszähler:

```text
Shrike: High gpu
🎮 Simulated GPU — GPU 99.0%; high in 3 observations over 30s; cause not established
```

## Wiederholen

Im Projektverzeichnis ausführen:

```sh
go test ./internal/detectors -run '^TestGPUSimulation$' -v -count=1
```

Die Simulation prüft Befundanzahl, Schweregrad, Prozesszuordnung,
Benachrichtigungen und die echte Doctor-Ausgabe mit Assertions. Beide Szenarien
sind erfolgreich durchgelaufen. Dies prüft die Reaktion auf Messdaten; es
reproduziert keinen echten GPU-/Treiber-Hänger.
