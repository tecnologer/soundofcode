//go:build darwin

package keyboard

import (
	"context"
	"fmt"
	"log"
	"os"
	"syscall"

	"github.com/tecnologer/soundofcode/internal/evdev"
	"golang.org/x/term"
)

// charToCode maps terminal byte values to evdev-compatible key codes.
var charToCode = map[byte]uint16{
	'1': evdev.Key1, '2': evdev.Key2, '3': evdev.Key3, '4': evdev.Key4,
	'5': evdev.Key5, '6': evdev.Key6, '7': evdev.Key7, '8': evdev.Key8,
	'9': evdev.Key9, '0': evdev.Key0, '-': evdev.KeyMinus, '=': evdev.KeyEqual,
	'q': evdev.KeyQ, 'w': evdev.KeyW, 'e': evdev.KeyE, 'r': evdev.KeyR,
	't': evdev.KeyT, 'y': evdev.KeyY, 'u': evdev.KeyU, 'i': evdev.KeyI,
	'o': evdev.KeyO, 'p': evdev.KeyP, '[': evdev.KeyLeftBrace, ']': evdev.KeyRightBrace,
	'\r': evdev.KeyEnter, '\n': evdev.KeyEnter,
	'a': evdev.KeyA, 's': evdev.KeyS, 'd': evdev.KeyD, 'f': evdev.KeyF,
	'g': evdev.KeyG, 'h': evdev.KeyH, 'j': evdev.KeyJ, 'k': evdev.KeyK,
	'l': evdev.KeyL, ';': evdev.KeySemicolon, '\'': evdev.KeyApostrophe,
	'z': evdev.KeyZ, 'x': evdev.KeyX, 'c': evdev.KeyC, 'v': evdev.KeyV,
	'b': evdev.KeyB, 'n': evdev.KeyN, 'm': evdev.KeyM, ',': evdev.KeyComma,
	'.': evdev.KeyDot, '/': evdev.KeySlash,
	' ': evdev.KeySpace,
}

// StartListening puts the terminal in raw mode and reads key codes from stdin into keyCh.
func StartListening(ctx context.Context, keyCh chan<- uint16) error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("stdin is not a terminal — daemon mode is not supported on macOS")
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("raw terminal: %w", err)
	}

	// Restore terminal when context is cancelled.
	go func() {
		<-ctx.Done()
		_ = term.Restore(fd, oldState)
	}()

	log.Println("soundofcode: listening on stdin (press Ctrl+C to stop)")

	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				return
			}

			b := buf[0]

			// Ctrl+C in raw mode — send SIGINT so the signal handler in main fires.
			if b == 3 {
				_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
				return
			}

			if code, ok := charToCode[b]; ok {
				select {
				case <-ctx.Done():
					return
				case keyCh <- code:
				default:
				}
			}
		}
	}()

	return nil
}
