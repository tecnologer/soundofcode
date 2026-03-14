package evdev

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/tecnologer/soundofcode/utils/closer"
	"golang.org/x/sys/unix"
)

// InputEvent mirrors the kernel's struct input_event (24 bytes on amd64).
type InputEvent struct {
	Sec   int64
	Usec  int64
	Type  uint16
	Code  uint16
	Value int32
}

const inputEventSize = 24

// eviocgbit returns the ioctl number for EVIOCGBIT(evType, size).
// _IOC(IOC_READ=2, 'E'=0x45, 0x20+evType, size)
func eviocgbit(evType, size uint) uintptr {
	return (2 << 30) | (0x45 << 8) | uintptr(0x20+evType) | (uintptr(size) << 16)
}

// isKeyboard checks whether the device supports key events by testing
// its EvKey capability bitmask for the presence of KeyA (code 30).
func isKeyboard(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}

	defer closer.Close(file)

	var keyBits [96]byte // covers key codes 0–767

	req := eviocgbit(1, uint(len(keyBits))) // EvKey = 1

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, file.Fd(), req, uintptr(unsafe.Pointer(&keyBits[0])))
	if errno != 0 {
		return false
	}

	// Check bit for KeyA (30): byte index = 30/8 = 3, bit = 30%8 = 6
	return keyBits[KeyA/8]&(1<<(KeyA%8)) != 0
}

// FindKeyboards returns paths to all keyboard input event devices.
func FindKeyboards() ([]string, error) {
	matches, err := filepath.Glob("/dev/input/event*")
	if err != nil {
		return nil, fmt.Errorf("glob /dev/input/event*: %w", err)
	}

	var keyboards []string

	for _, path := range matches {
		if isKeyboard(path) {
			keyboards = append(keyboards, path)
		}
	}

	return keyboards, nil
}
