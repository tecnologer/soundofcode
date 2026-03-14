package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	"github.com/ebitengine/oto/v3"
)

const maxVoices = 16

// Mixer sums all active voices into a single PCM stream (implements io.Reader).
type Mixer struct {
	mu     sync.Mutex
	voices []*Voice
}

func (m *Mixer) Read(buf []byte) (int, error) {
	numSamples := len(buf) / 4 // float32LE = 4 bytes per sample
	samples := make([]float32, numSamples)

	m.mu.Lock()
	alive := make([]*Voice, 0, len(m.voices))

	for _, voice := range m.voices {
		voice.Fill(samples)

		if !voice.Done() {
			alive = append(alive, voice)
		}
	}

	if len(m.voices) != len(alive) {
		m.voices = alive
	}

	m.mu.Unlock()

	for i, s := range samples {
		clipped := float32(math.Tanh(float64(s))) // soft clip to prevent distortion
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(clipped))
	}

	return len(buf), nil
}

func (m *Mixer) addVoice(voice *Voice) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.voices) >= maxVoices {
		m.voices = m.voices[1:] // drop oldest to stay within polyphony limit
	}

	m.voices = append(m.voices, voice)
}

// Engine manages the oto audio context and plays synthesized notes.
type Engine struct {
	ctx    *oto.Context
	player *oto.Player
	mixer  *Mixer
}

// NewEngine initializes ALSA via oto and starts streaming silence.
func NewEngine() (*Engine, error) {
	mixer := &Mixer{}

	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: 1,
		Format:       oto.FormatFloat32LE,
	})
	if err != nil {
		return nil, fmt.Errorf("oto context: %w", err)
	}

	<-ready

	player := ctx.NewPlayer(mixer)
	player.Play()

	return &Engine{ctx: ctx, player: player, mixer: mixer}, nil
}

// PlayNote schedules a new voice for the given frequency.
func (e *Engine) PlayNote(freq float64) {
	e.mixer.addVoice(newVoice(freq))
}

// Close stops the audio player and releases resources.
func (e *Engine) Close() {
	_ = e.player.Close() //nolint:staticcheck
}
