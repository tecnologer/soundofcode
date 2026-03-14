package audio

import "math"

const sampleRate = 44100

// SampleRate is the audio sample rate used for synthesis.
const SampleRate = sampleRate

type adsr struct {
	attack  float64 // seconds
	decay   float64 // seconds
	sustain float64 // amplitude [0,1]
	release float64 // seconds (exponential decay constant)
}

// Voice is a single synthesized piano note.
type Voice struct {
	freq    float64
	elapsed float64 // elapsed time in seconds
}

func newVoice(freq float64) *Voice {
	return &Voice{freq: freq}
}

// NewVoice creates a new synthesized voice for the given frequency.
func NewVoice(freq float64) *Voice {
	return newVoice(freq)
}

// Fill adds this voice's samples into buf (accumulating, not overwriting).
func (v *Voice) Fill(buf []float32) {
	const deltaT = 1.0 / sampleRate
	// Additive synthesis: fundamental + harmonics with inharmonicity
	type harmonic struct{ mult, amp float64 }

	harmonics := [6]harmonic{
		{1.0, 1.00},
		{2.0, 0.45},
		{3.0, 0.22},
		{4.0, 0.10},
		{5.0, 0.05},
		{6.0, 0.02},
	}

	const inharmonicity = 0.00015 // inharmonicity factor (piano-like partial stretching)

	for idx := range buf {
		amp := v.amplitude()
		sample := 0.0

		for n, h := range harmonics {
			pf := h.mult * v.freq * math.Sqrt(1+inharmonicity*float64(n+1)*float64(n+1))
			sample += h.amp * math.Sin(2*math.Pi*pf*v.elapsed)
		}

		buf[idx] += float32(sample * amp * 0.15)
		v.elapsed += deltaT
	}
}

// Done returns true when the voice has decayed to near silence.
func (v *Voice) Done() bool {
	return v.amplitude() < 0.001
}

// amplitude returns the envelope gain at the current time.
func (v *Voice) amplitude() float64 {
	pianoEnv := adsr{
		attack:  0.005,
		decay:   0.12,
		sustain: 0.25,
		release: 1.8,
	}

	elapsed := v.elapsed
	env := pianoEnv

	switch {
	case elapsed < env.attack:
		return elapsed / env.attack
	case elapsed < env.attack+env.decay:
		return 1.0 - (1.0-env.sustain)*(elapsed-env.attack)/env.decay
	default:
		rel := elapsed - (env.attack + env.decay)

		return env.sustain * math.Exp(-rel*3.0/env.release)
	}
}
