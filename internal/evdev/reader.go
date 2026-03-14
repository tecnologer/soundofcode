//go:build linux

package evdev

import (
	"context"
	"log"
	"os"
	"unsafe"

	"github.com/tecnologer/soundofcode/utils/closer"
)

// ReadKeys reads key-press events from the device at path and sends key codes to keyCh.
// Blocks until an error occurs or the context is cancelled.
func ReadKeys(ctx context.Context, path string, keyCh chan<- uint16) {
	file, err := os.Open(path)
	if err != nil {
		log.Printf("evdev: open %s: %v", path, err)
		return
	}

	// Close the fd when context is cancelled to unblock file.Read.
	go func() {
		<-ctx.Done()

		closer.Close(file)
	}()

	buf := make([]byte, inputEventSize)
	for {
		n, err := file.Read(buf)
		if err != nil {
			return // context cancelled or device removed
		}

		if n != inputEventSize {
			continue
		}

		ev := (*InputEvent)(unsafe.Pointer(&buf[0]))
		if ev.Type == EvKey && ev.Value == KeyDown {
			select {
			case keyCh <- ev.Code:
			default: // drop if channel full
			}
		}
	}
}
