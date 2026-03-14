//go:build linux

package keyboard

import (
	"context"
	"fmt"
	"log"

	"github.com/tecnologer/soundofcode/internal/evdev"
)

// StartListening finds all keyboard devices and starts reading key codes into keyCh.
func StartListening(ctx context.Context, keyCh chan<- uint16) error {
	keyboards, err := evdev.FindKeyboards()
	if err != nil {
		return fmt.Errorf("find keyboards: %w", err)
	}

	if len(keyboards) == 0 {
		return fmt.Errorf("no keyboard devices found — add yourself to the 'input' group:\n  sudo usermod -aG input $USER\nthen log out and back in")
	}

	log.Printf("soundofcode: listening on %d keyboard device(s)", len(keyboards))

	for _, dev := range keyboards {
		go evdev.ReadKeys(ctx, dev, keyCh)
	}

	return nil
}
