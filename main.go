package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tecnologer/soundofcode/internal/audio"
	"github.com/tecnologer/soundofcode/internal/evdev"
	"github.com/tecnologer/soundofcode/internal/keymap"
	"github.com/tecnologer/soundofcode/internal/song"
)

func main() {
	daemon := flag.Bool("daemon", false, "run as a background process")
	output := flag.String("output", "", "output file path (default: ~/soundofcode-<timestamp>.<format>)")
	format := flag.String("format", "mid", "output format: mid or mp3")
	ctl := flag.String("ctl", "", "send control command to running daemon: pause, resume, stop")

	flag.Parse()

	if *format != "mid" && *format != "mp3" {
		log.Fatalf("soundofcode: invalid format %q — must be mid or mp3", *format)
	}

	if *ctl != "" {
		err := control(*ctl)
		if err != nil {
			log.Fatal(err)
		}

		return
	}

	if *daemon {
		err := daemonize()
		if err != nil {
			log.Fatal(err)
		}

		return
	}

	err := run(*output, *format)
	if err != nil {
		log.Fatal(err)
	}
}

func run(outputPath, format string) error {
	err := writePID()
	if err != nil {
		log.Printf("soundofcode: could not write PID file: %v", err)
	}

	defer removePID()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, err := audio.NewEngine()
	if err != nil {
		return fmt.Errorf("audio engine: %w", err)
	}

	defer engine.Close()

	keyboards, err := evdev.FindKeyboards()
	if err != nil {
		return fmt.Errorf("find keyboards: %w", err)
	}

	if len(keyboards) == 0 {
		return fmt.Errorf("no keyboard devices found — add yourself to the 'input' group:\n  sudo usermod -aG input $USER\nthen log out and back in")
	}

	log.Printf("soundofcode: listening on %d keyboard device(s)", len(keyboards))

	return runLoop(ctx, cancel, engine, keyboards, outputPath, format)
}

// runLoop runs the main event loop after setup.
func runLoop(
	ctx context.Context,
	cancel context.CancelFunc,
	engine *audio.Engine,
	keyboards []string,
	outputPath, format string,
) error {
	keyCh := make(chan uint16, 64)
	for _, dev := range keyboards {
		go evdev.ReadKeys(ctx, dev, keyCh)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2)

	rec := song.New()
	keys := keymap.New()
	paused := false

	for {
		select {
		case <-ctx.Done():
			return saveSong(rec, outputPath, format)

		case sig := <-sigCh:
			cancel = handleSignal(sig, cancel, rec, format, &paused, &rec)

		case code := <-keyCh:
			if !paused {
				if freq, ok := keys.Lookup(code); ok {
					engine.PlayNote(freq)
					rec.Record(freq)
				}
			}
		}
	}
}

// handleSignal processes a received OS signal and returns the (possibly new) cancel func.
func handleSignal(
	sig os.Signal,
	cancel context.CancelFunc,
	rec *song.Song,
	format string,
	paused *bool,
	recPtr **song.Song,
) context.CancelFunc {
	switch sig {
	case syscall.SIGUSR1: // pause / resume toggle
		*paused = !*paused
		if *paused {
			log.Println("soundofcode: recording paused")
		} else {
			log.Println("soundofcode: recording resumed")
		}

	case syscall.SIGUSR2: // save current recording and start fresh
		err := saveSong(rec, "", format)
		if err != nil {
			log.Printf("soundofcode: save error: %v", err)
		}

		*recPtr = song.New()
		*paused = false

		log.Println("soundofcode: new recording started")

	default: // SIGINT, SIGTERM — shut down
		fmt.Println()
		log.Println("soundofcode: shutting down")
		cancel()
	}

	return cancel
}

func saveSong(rec *song.Song, outputPath, format string) error {
	if rec.Len() == 0 {
		log.Printf("soundofcode: no notes recorded, skipping %s export", format)

		return nil
	}

	if outputPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}

		if home == "/root" {
			home = os.Getenv("$HOME")
		}

		ts := time.Now().Format("2006-01-02-150405")
		outputPath = filepath.Join(home, fmt.Sprintf("soundofcode-%s.%s", ts, format))
	}

	switch format {
	case "mp3":
		err := rec.SaveMP3(outputPath)
		if err != nil {
			return fmt.Errorf("save MP3: %w", err)
		}
	default:
		err := rec.SaveMIDI(outputPath)
		if err != nil {
			return fmt.Errorf("save MIDI: %w", err)
		}
	}

	log.Printf("soundofcode: saved %d notes → %q", rec.Len(), outputPath) //nolint:gosec

	return nil
}

// control sends a signal to the running daemon.
// pause/resume toggle via SIGUSR1; stop saves and resets via SIGUSR2.
func control(cmd string) error {
	pid, err := readPID()
	if err != nil {
		return err
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}

	switch cmd {
	case "pause", "resume":
		return fmt.Errorf("signal pause/resume: %w", proc.Signal(syscall.SIGUSR1))
	case "stop":
		return fmt.Errorf("signal stop: %w", proc.Signal(syscall.SIGUSR2))
	default:
		return fmt.Errorf("unknown control command %q — use pause, resume, or stop", cmd)
	}
}

func pidFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	return filepath.Join(home, ".soundofcode.pid")
}

func writePID() error {
	return fmt.Errorf("write PID: %w", os.WriteFile(pidFilePath(), []byte(strconv.Itoa(os.Getpid())), 0o644))
}

func removePID() {
	_ = os.Remove(pidFilePath())
}

func readPID() (int, error) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0, fmt.Errorf("daemon not running (no PID file found): %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID file: %w", err)
	}

	return pid, nil
}

// daemonize re-executes the binary without --daemon, detached from the terminal.
// Stderr is redirected to ~/.soundofcode.log so errors are visible.
func daemonize() error {
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open devnull: %w", err)
	}

	logFile, err := os.OpenFile(logFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "soundofcode: could not open log file: %v\n", err)

		logFile = devNull
	}

	// Filter out the --daemon / -daemon flag to avoid infinite loop.
	args := make([]string, 0, len(os.Args))
	args = append(args, os.Args[0])

	for _, a := range os.Args[1:] {
		if a != "--daemon" && a != "-daemon" {
			args = append(args, a)
		}
	}

	proc, err := os.StartProcess(os.Args[0], args, &os.ProcAttr{ //nolint:gosec
		Files: []*os.File{devNull, devNull, logFile},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	})
	if err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	fmt.Printf("soundofcode: daemon started (pid %d), logs → %s\n", proc.Pid, logFilePath())

	return fmt.Errorf("release daemon process: %w", proc.Release())
}

func logFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	return filepath.Join(home, ".soundofcode.log")
}
