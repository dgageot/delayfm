# ᗺ delay.fm

A terminal radio player with adjustable playback delay.

Search for stations, stream them live, and shift playback up to 30 seconds into the past — useful for syncing audio with a delayed video broadcast.

Vibe-coded on a soccer game night to sync French radio commentary with a foreign TV broadcast.

## Features

- **Station search** — search by name and country code using the [Radio Browser](https://www.radio-browser.info/) API (defaults to FR, changeable in the UI).
- **Adjustable delay** — shift playback from live to -30s in 1-second steps using a ring buffer.
- **Persistent state** — your last search query is restored on next launch.
- **TUI** — full-screen terminal interface built with [Bubble Tea](https://charm.land/bubbletea).

## Install

```sh
go install delayfm@latest
```

Requires Go 1.25+.

## Usage

```sh
delayfm
delayfm --delay 10s
```

| Flag      | Description                          | Default |
|-----------|--------------------------------------|---------|
| `--delay` | Initial playback delay (e.g. `10s`)  | `5s`    |

### Keys

| Key            | Action                 |
|----------------|------------------------|
| `↑` / `k`      | Previous station       |
| `↓` / `j`      | Next station           |
| `Enter`        | Search / play station  |
| `Tab`          | Toggle search / list   |
| `←` / `-`      | Decrease delay (1s)    |
| `→` / `+`      | Increase delay (1s)    |
| `0`–`9`        | Jump delay to 0s–9s    |
| `s`            | Stop playback          |
| `q` / `Ctrl+C` | Quit                   |
