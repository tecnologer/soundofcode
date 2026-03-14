package keymap

import (
	"math"

	"github.com/tecnologer/soundofcode/internal/evdev"
)

// KeyMap maps evdev key codes to note frequencies in Hz.
type KeyMap struct {
	notes map[uint16]float64
}

// midiToFreq converts a MIDI note number to frequency in Hz.
// A4 = MIDI 69 = 440 Hz.
func midiToFreq(midi int) float64 {
	return 440.0 * math.Pow(2, float64(midi-69)/12.0)
}

// buildRow generates `count` diatonic notes starting from the given MIDI root.
func buildRow(rootMIDI, count int) []float64 {
	// Diatonic C major scale semitone offsets: C D E F G A B
	scale := []int{0, 2, 4, 5, 7, 9, 11}

	notes := make([]float64, 0, count)
	// Find the octave and scale position of rootMIDI (assumed to be a C)
	octave := rootMIDI/12 - 1

	scaleIdx := 0
	for len(notes) < count {
		midi := 12*(octave+1) + scale[scaleIdx]
		notes = append(notes, midiToFreq(midi))

		scaleIdx++
		if scaleIdx >= len(scale) {
			scaleIdx = 0
			octave++
		}
	}

	return notes
}

// New creates a KeyMap using a diatonic C major scale across keyboard rows.
//
// Row layout (higher rows = higher pitch):
//
//	Number row (1-0, -, =):    C5 → G6
//	Top row    (Q-P, [, ]):    C4 → G5
//	Home row   (A-L, ;, '):    C3 → G4
//	Bottom row (Z-M, ,, ., /): C2 → E3
//	Space:                     C2 (bass)
//	Enter:                     C3
func New() *KeyMap {
	rows := []struct {
		keys  []uint16
		rootC int // MIDI note of the C that starts this row
	}{
		{
			keys: []uint16{
				evdev.Key1, evdev.Key2, evdev.Key3, evdev.Key4,
				evdev.Key5, evdev.Key6, evdev.Key7, evdev.Key8,
				evdev.Key9, evdev.Key0, evdev.KeyMinus, evdev.KeyEqual,
			},
			rootC: 72, // C5
		},
		{
			keys: []uint16{
				evdev.KeyQ, evdev.KeyW, evdev.KeyE, evdev.KeyR,
				evdev.KeyT, evdev.KeyY, evdev.KeyU, evdev.KeyI,
				evdev.KeyO, evdev.KeyP, evdev.KeyLeftBrace, evdev.KeyRightBrace,
			},
			rootC: 60, // C4
		},
		{
			keys: []uint16{
				evdev.KeyA, evdev.KeyS, evdev.KeyD, evdev.KeyF,
				evdev.KeyG, evdev.KeyH, evdev.KeyJ, evdev.KeyK,
				evdev.KeyL, evdev.KeySemicolon, evdev.KeyApostrophe,
			},
			rootC: 48, // C3
		},
		{
			keys: []uint16{
				evdev.KeyZ, evdev.KeyX, evdev.KeyC, evdev.KeyV,
				evdev.KeyB, evdev.KeyN, evdev.KeyM, evdev.KeyComma,
				evdev.KeyDot, evdev.KeySlash,
			},
			rootC: 36, // C2
		},
	}

	notes := make(map[uint16]float64)

	for _, row := range rows {
		freqs := buildRow(row.rootC, len(row.keys))
		for idx, key := range row.keys {
			notes[key] = freqs[idx]
		}
	}

	notes[evdev.KeySpace] = midiToFreq(36) // C2 bass
	notes[evdev.KeyEnter] = midiToFreq(48) // C3

	return &KeyMap{notes: notes}
}

// Lookup returns the frequency for the given key code, and whether it exists.
func (k *KeyMap) Lookup(code uint16) (float64, bool) {
	freq, ok := k.notes[code]

	return freq, ok
}
