package song

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/tecnologer/soundofcode/internal/audio"
	"github.com/viert/go-lame"
)

const (
	ticksPerBeat      = 480
	bpm               = 120
	tempo             = 60_000_000 / bpm // microseconds per beat
	noteDurationTicks = ticksPerBeat * 7 // ≈ 3.5s at 120 BPM, matches piano release envelope
)

type noteEvent struct {
	at   time.Duration
	midi uint8
}

// Song records note events and can serialize them to a MIDI file.
type Song struct {
	mu     sync.Mutex
	start  time.Time
	events []noteEvent
}

func New() *Song {
	return &Song{start: time.Now()}
}

// Record adds a keystroke note at the current time.
func (s *Song) Record(freq float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, noteEvent{
		at:   time.Since(s.start),
		midi: freqToMIDI(freq),
	})
}

// Len returns the number of recorded notes.
func (s *Song) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.events)
}

// SaveMIDI writes the recorded notes to a standard MIDI file (format 0).
func (s *Song) SaveMIDI(path string) error {
	s.mu.Lock()
	events := make([]noteEvent, len(s.events))
	copy(events, s.events)
	s.mu.Unlock()

	if len(events) == 0 {
		return fmt.Errorf("no notes recorded")
	}

	outFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create MIDI file: %w", err)
	}

	defer outFile.Close()

	track := buildTrack(events)

	// Header chunk: MThd
	_, _ = outFile.WriteString("MThd")

	writeBinary(outFile, binary.BigEndian, uint32(6))
	writeBinary(outFile, binary.BigEndian, uint16(0)) // format 0: single track
	writeBinary(outFile, binary.BigEndian, uint16(1)) // 1 track
	writeBinary(outFile, binary.BigEndian, uint16(ticksPerBeat))

	// Track chunk: MTrk
	writeFile(outFile, []byte("MTrk"))
	writeBinary(outFile, binary.BigEndian, uint32(len(track))) //nolint:gosec
	writeFile(outFile, track)

	return nil
}

func writeBinary(w io.Writer, order binary.ByteOrder, data any) {
	err := binary.Write(w, order, data)
	if err != nil {
		log.Printf("soundofcode: binary write error: %v", err)
	}
}

func writeFile(file *os.File, data []byte) {
	_, err := file.Write(data)
	if err != nil {
		log.Printf("soundofcode: file write error: %v", err)
	}
}

// freqToMIDI converts a frequency in Hz to the nearest MIDI note number.
func freqToMIDI(freq float64) uint8 {
	midiNote := int(math.Round(69 + 12*math.Log2(freq/440.0)))
	if midiNote < 0 {
		return 0
	}

	if midiNote > 127 {
		return 127
	}

	return uint8(midiNote) // midiNote is bounded to [0,127] by the checks above
}

type midiEv struct {
	tick uint32
	data []byte
}

func durationToTicks(d time.Duration) uint32 {
	ticksPerSecond := float64(ticksPerBeat) * bpm / 60.0
	return uint32(d.Seconds() * ticksPerSecond)
}

func buildTrack(events []noteEvent) []byte {
	var evs []midiEv

	// Set tempo meta event: FF 51 03 <tempo_bytes>
	tempoVal := uint32(tempo)
	evs = append(evs, midiEv{0, []byte{
		0xFF, 0x51, 0x03,
		byte(tempoVal >> 16), byte(tempoVal >> 8), byte(tempoVal), //nolint:gosec
	}})

	for _, e := range events {
		tick := durationToTicks(e.at)
		evs = append(evs, midiEv{tick, []byte{0x90, e.midi, 80}})                    // note on
		evs = append(evs, midiEv{tick + noteDurationTicks, []byte{0x80, e.midi, 0}}) // note off
	}

	sort.Slice(evs, func(i, j int) bool { return evs[i].tick < evs[j].tick })

	// End of track
	lastTick := evs[len(evs)-1].tick
	evs = append(evs, midiEv{lastTick, []byte{0xFF, 0x2F, 0x00}})

	var buf []byte

	var prevTick uint32

	for _, ev := range evs {
		delta := ev.tick - prevTick
		prevTick = ev.tick

		buf = append(buf, vlq(delta)...)
		buf = append(buf, ev.data...)
	}

	return buf
}

// midiToFreq converts a MIDI note number to its frequency in Hz.
func midiToFreq(note uint8) float64 {
	return 440.0 * math.Pow(2, (float64(note)-69)/12.0)
}

// renderPCM synthesizes all recorded notes into a mono float32 PCM buffer.
func renderPCM(events []noteEvent) []float32 {
	const tailSeconds = 3.0

	last := events[len(events)-1]
	totalSamples := int((last.at.Seconds()+tailSeconds)*float64(audio.SampleRate)) + 1

	pcm := make([]float32, totalSamples)
	chunk := make([]float32, 4096)

	for _, e := range events {
		startSample := int(e.at.Seconds() * float64(audio.SampleRate))
		voice := audio.NewVoice(midiToFreq(e.midi))

		pos := startSample
		for pos < totalSamples {
			chunkSize := len(chunk)
			if pos+chunkSize > totalSamples {
				chunkSize = totalSamples - pos
			}

			chunkSlice := chunk[:chunkSize]
			for idx := range chunkSlice {
				chunkSlice[idx] = 0
			}

			voice.Fill(chunkSlice)

			for sampleIdx, sampleVal := range chunkSlice {
				pcm[pos+sampleIdx] += sampleVal
			}

			pos += chunkSize

			if voice.Done() {
				break
			}
		}
	}

	return pcm
}

// normalizePCM converts float32 PCM [-1,1] to int16 little-endian bytes.
func normalizePCM(pcm []float32) ([]byte, float32) {
	buf := make([]byte, len(pcm)*2)

	var peak float32

	for sampleIdx, sample := range pcm {
		if sample > peak {
			peak = sample
		} else if -sample > peak {
			peak = -sample
		}

		if sample > 1 {
			sample = 1
		} else if sample < -1 {
			sample = -1
		}

		encoded := int16(sample * 32767)
		buf[2*sampleIdx] = byte(encoded)        //nolint:gosec
		buf[2*sampleIdx+1] = byte(encoded >> 8) //nolint:gosec
	}

	return buf, peak
}

// setupEncoder configures the lame MP3 encoder settings.
func setupEncoder(enc *lame.Encoder) error {
	err := enc.SetInSamplerate(audio.SampleRate)
	if err != nil {
		return fmt.Errorf("mp3 samplerate: %w", err)
	}

	err = enc.SetNumChannels(1)
	if err != nil {
		return fmt.Errorf("mp3 channels: %w", err)
	}

	err = enc.SetMode(lame.MpegMono)
	if err != nil {
		return fmt.Errorf("mp3 mode: %w", err)
	}

	err = enc.SetQuality(5)
	if err != nil {
		return fmt.Errorf("mp3 quality: %w", err)
	}

	return nil
}

// SaveMP3 synthesizes the recorded notes and writes them as an MP3 file.
func (s *Song) SaveMP3(path string) error {
	s.mu.Lock()
	events := make([]noteEvent, len(s.events))
	copy(events, s.events)
	s.mu.Unlock()

	if len(events) == 0 {
		return fmt.Errorf("no notes recorded")
	}

	pcm := renderPCM(events)

	outFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create MP3 file: %w", err)
	}

	defer outFile.Close()

	enc := lame.NewEncoder(outFile)

	err = setupEncoder(enc)
	if err != nil {
		return err
	}

	buf, peak := normalizePCM(pcm)

	log.Printf("soundofcode: rendering %d samples, peak amplitude %.4f", len(pcm), peak)

	_, err = enc.Write(buf)
	if err != nil {
		return fmt.Errorf("mp3 encode: %w", err)
	}

	_, err = enc.Flush()
	if err != nil {
		return fmt.Errorf("mp3 flush: %w", err)
	}

	enc.Close()

	return nil
}

// vlq encodes a uint32 as a MIDI variable-length quantity.
func vlq(val uint32) []byte {
	if val < 0x80 {
		return []byte{byte(val)}
	}

	var encoded []byte

	for val > 0 {
		encoded = append([]byte{byte(val & 0x7F)}, encoded...)
		val >>= 7
	}

	for idx := 0; idx < len(encoded)-1; idx++ {
		encoded[idx] |= 0x80
	}

	return encoded
}
