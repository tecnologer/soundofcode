//go:build darwin

package keyboard

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <stdint.h>

// Defined in keyboard_darwin_tap.c
extern int  createEventTap(void);
extern void runEventTap(void);
extern void stopEventTap(void);
*/
import "C"

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"syscall"

	"github.com/tecnologer/soundofcode/internal/evdev"
	"golang.org/x/term"
)

// macKeyToEvdev maps macOS virtual key codes (kVK_ANSI_*) to evdev key codes.
// Values from Carbon/HIToolbox/Events.h.
var macKeyToEvdev = map[uint16]uint16{
	0x00: evdev.KeyA,         // kVK_ANSI_A
	0x01: evdev.KeyS,         // kVK_ANSI_S
	0x02: evdev.KeyD,         // kVK_ANSI_D
	0x03: evdev.KeyF,         // kVK_ANSI_F
	0x04: evdev.KeyH,         // kVK_ANSI_H
	0x05: evdev.KeyG,         // kVK_ANSI_G
	0x06: evdev.KeyZ,         // kVK_ANSI_Z
	0x07: evdev.KeyX,         // kVK_ANSI_X
	0x08: evdev.KeyC,         // kVK_ANSI_C
	0x09: evdev.KeyV,         // kVK_ANSI_V
	0x0B: evdev.KeyB,         // kVK_ANSI_B
	0x0C: evdev.KeyQ,         // kVK_ANSI_Q
	0x0D: evdev.KeyW,         // kVK_ANSI_W
	0x0E: evdev.KeyE,         // kVK_ANSI_E
	0x0F: evdev.KeyR,         // kVK_ANSI_R
	0x10: evdev.KeyY,         // kVK_ANSI_Y
	0x11: evdev.KeyT,         // kVK_ANSI_T
	0x12: evdev.Key1,         // kVK_ANSI_1
	0x13: evdev.Key2,         // kVK_ANSI_2
	0x14: evdev.Key3,         // kVK_ANSI_3
	0x15: evdev.Key4,         // kVK_ANSI_4
	0x16: evdev.Key6,         // kVK_ANSI_6
	0x17: evdev.Key5,         // kVK_ANSI_5
	0x18: evdev.KeyEqual,     // kVK_ANSI_Equal
	0x19: evdev.Key9,         // kVK_ANSI_9
	0x1A: evdev.Key7,         // kVK_ANSI_7
	0x1B: evdev.KeyMinus,     // kVK_ANSI_Minus
	0x1C: evdev.Key8,         // kVK_ANSI_8
	0x1D: evdev.Key0,         // kVK_ANSI_0
	0x1E: evdev.KeyRightBrace, // kVK_ANSI_RightBracket
	0x1F: evdev.KeyO,         // kVK_ANSI_O
	0x20: evdev.KeyU,         // kVK_ANSI_U
	0x21: evdev.KeyLeftBrace,  // kVK_ANSI_LeftBracket
	0x22: evdev.KeyI,         // kVK_ANSI_I
	0x23: evdev.KeyP,         // kVK_ANSI_P
	0x24: evdev.KeyEnter,     // kVK_Return
	0x25: evdev.KeyL,         // kVK_ANSI_L
	0x26: evdev.KeyJ,         // kVK_ANSI_J
	0x27: evdev.KeyApostrophe, // kVK_ANSI_Quote
	0x28: evdev.KeyK,         // kVK_ANSI_K
	0x29: evdev.KeySemicolon,  // kVK_ANSI_Semicolon
	0x2B: evdev.KeyComma,     // kVK_ANSI_Comma
	0x2C: evdev.KeySlash,     // kVK_ANSI_Slash
	0x2D: evdev.KeyN,         // kVK_ANSI_N
	0x2E: evdev.KeyM,         // kVK_ANSI_M
	0x2F: evdev.KeyDot,       // kVK_ANSI_Period
	0x31: evdev.KeySpace,     // kVK_Space
}

var (
	globalMu    sync.Mutex
	globalKeyCh chan<- uint16
)

// goKeyCallback is called from C (keyboard_darwin_tap.c) for each key-down event.
//
//export goKeyCallback
func goKeyCallback(keyCode C.uint16_t) {
	globalMu.Lock()
	ch := globalKeyCh
	globalMu.Unlock()

	if ch == nil {
		return
	}

	code, ok := macKeyToEvdev[uint16(keyCode)]
	if !ok {
		return
	}

	select {
	case ch <- code:
	default:
	}
}

// StartListening tries global CGEventTap first; falls back to stdin raw mode
// if Input Monitoring permission has not been granted.
func StartListening(ctx context.Context, keyCh chan<- uint16) error {
	globalMu.Lock()
	globalKeyCh = keyCh
	globalMu.Unlock()

	if C.createEventTap() != 0 {
		go C.runEventTap()
		go func() {
			<-ctx.Done()
			C.stopEventTap()
		}()
		log.Println("soundofcode: listening globally via CGEventTap (works without terminal focus)")
		return nil
	}

	log.Println("soundofcode: CGEventTap unavailable — grant Input Monitoring access to your terminal in System Settings > Privacy & Security > Input Monitoring for focus-independent capture; falling back to terminal stdin")
	return startStdin(ctx, keyCh)
}

// charToCode maps terminal byte values to evdev-compatible key codes (stdin fallback).
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

func startStdin(ctx context.Context, keyCh chan<- uint16) error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("stdin is not a terminal — daemon mode is not supported on macOS without Input Monitoring permission")
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("raw terminal: %w", err)
	}

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

			if b == 3 { // Ctrl+C in raw mode
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
