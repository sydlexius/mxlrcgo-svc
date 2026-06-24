// Package detector provides an optional audio-based instrumental detection
// sidecar. It sends short audio windows to an external AudioSet classifier
// (e.g. YAMNet/PANNs served over FastAPI) and aggregates per-class
// probabilities to determine whether a track is instrumental.
//
// The sidecar pattern mirrors internal/verification: a Go HTTP client drives
// an out-of-process ML model; the heavy inference never enters the no-CGO Go
// binary. All detector errors are non-fatal: callers log a warning and fall
// through to miss handling. Detector starvation (slow or unavailable sidecar)
// is acceptable; host CPU starvation is not. The HTTPDetector serializes
// inference calls and enforces a per-call cooldown to prevent runaway resource
// use.
package detector

import (
	"context"
	"errors"
)

// ErrClassifierUnavailable is returned when the classifier HTTP endpoint
// cannot be reached or returns a non-2xx status.
var ErrClassifierUnavailable = errors.New("detector: classifier unavailable")

// ErrInvalidResponse is returned when the classifier returns a response that
// cannot be decoded as a class-probability map.
var ErrInvalidResponse = errors.New("detector: invalid classifier response")

// ErrCooldownInterrupted is returned when the context is canceled while the
// detector is waiting for the cooldown between inference calls to expire.
var ErrCooldownInterrupted = errors.New("detector: cooldown interrupted by context cancellation")

const (
	minSampleDurationSeconds = 30
	maxSampleDurationSeconds = 60
)

// Result describes an instrumental detection decision.
type Result struct {
	// Instrumental is true only when the summed mean probability of the
	// configured InstrumentalClasses meets or exceeds MinConfidence (the music
	// gate) AND the peak (max-over-frames) of every configured VocalClass stays
	// below VocalMaxConfidence (the vocal gate). Any doubt resolves to false: a
	// false instrumental suppresses a real lyrics fetch.
	Instrumental bool
	// Confidence is the summed instrumental-class MEAN probability (the music
	// score) for the classified sample.
	Confidence float64
	// VocalConfidence is the peak vocal-class score (max over the configured
	// VocalClasses of their max-over-frames value). A high value means vocals
	// were detected somewhere in the sample.
	VocalConfidence float64
	// Classes is the per-class MEAN probability map from the classified sample,
	// retained for debugging and observability.
	Classes map[string]float64
}

// Detector checks whether an audio file is instrumental.
type Detector interface {
	Detect(ctx context.Context, audioPath string) (Result, error)
}

// Config holds the construction parameters for an HTTPDetector. Zero values for
// SampleDurationSeconds, MinConfidence, InstrumentalClasses, VocalClasses,
// VocalMaxConfidence, and SpreadSamples are replaced with built-in defaults by
// NewHTTPDetector; CooldownSeconds < 0 is clamped to 0. FFprobePath empty means
// auto-discover (sibling of ffmpeg, then PATH).
type Config struct {
	ClassifierURL         string
	SampleDurationSeconds int
	MinConfidence         float64
	InstrumentalClasses   []string
	VocalClasses          []string
	VocalMaxConfidence    float64
	SpreadSamples         int
	FFmpegPath            string
	FFprobePath           string
	CooldownSeconds       int
}

// clampSampleDuration clamps d to [minSampleDurationSeconds, maxSampleDurationSeconds].
func clampSampleDuration(d int) int {
	if d < minSampleDurationSeconds {
		return minSampleDurationSeconds
	}
	if d > maxSampleDurationSeconds {
		return maxSampleDurationSeconds
	}
	return d
}
