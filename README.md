# soundofcode

Turn your keyboard into a piano. Every keystroke plays a synthesized note and gets recorded — save as MIDI or MP3 when you're done.

## How it works

Keys are mapped to a diatonic C major scale across four rows:

| Row        | Keys                  | Range  |
|------------|-----------------------|--------|
| Number row | `1 2 3 4 5 6 7 8 9 0 - =` | C5 – G6 |
| Top row    | `Q W E R T Y U I O P [ ]` | C4 – G5 |
| Home row   | `A S D F G H J K L ; '`   | C3 – G4 |
| Bottom row | `Z X C V B N M , . /`     | C2 – E3 |
| `Space`    |                       | C2 (bass) |
| `Enter`    |                       | C3     |

Audio is synthesized with additive synthesis (6 harmonics with piano-like inharmonicity) and an ADSR envelope.

## Requirements

### Linux

- evdev support (`/dev/input/event*`)
- ALSA (via [oto](https://github.com/ebitengine/oto))
- Member of the `input` group to read keyboard devices:
  ```sh
  sudo usermod -aG input $USER
  # log out and back in
  ```
- LAME (`libmp3lame`) for MP3 export

### macOS

- CoreAudio (built-in)
- Run in the foreground (daemon mode requires a terminal)
- LAME for MP3 export:
  ```sh
  brew install lame
  ```

On macOS, keyboard input is read from the terminal in raw mode — just type as you would on Linux.

## Install

```sh
go install github.com/tecnologer/soundofcode@latest
```

Or build from source:

```sh
git clone https://github.com/tecnologer/soundofcode
cd soundofcode
go build -o soundofcode .
```

## Usage

```sh
# Play and record (saves to ~/soundofcode-<timestamp>.mid on exit)
soundofcode

# Save as MP3 instead
soundofcode -format mp3

# Choose output path
soundofcode -output ~/my-song.mid

# Run as a background daemon (Linux only)
soundofcode -daemon

# Control a running daemon (Linux only)
soundofcode -ctl pause    # pause recording
soundofcode -ctl resume   # resume recording
soundofcode -ctl stop     # save current recording and start fresh
```

Press `Ctrl+C` or send `SIGTERM` to stop and save.

### Daemon mode (Linux only)

When running with `-daemon`, the process detaches from the terminal. Logs are written to `~/.soundofcode.log`. The PID is stored in `~/.soundofcode.pid` for control commands.

## License

MIT
