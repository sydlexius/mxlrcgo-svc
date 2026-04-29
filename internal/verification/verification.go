package verification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/sydlexius/mxlrcgo-svc/internal/models"
	"github.com/sydlexius/mxlrcgo-svc/internal/normalize"
)

// Result describes an STT verification decision.
type Result struct {
	Accepted   bool
	Similarity float64
	Transcript string
}

// Verifier checks whether an audio transcript substantially matches lyrics.
type Verifier interface {
	Verify(ctx context.Context, audioPath string, song models.Song) (Result, error)
}

// HTTPVerifier calls a Whisper-compatible HTTP sidecar.
type HTTPVerifier struct {
	baseURL               string
	sampleDurationSeconds int
	minSimilarity         float64
	client                *http.Client
}

// NewHTTPVerifier creates a verifier for an OpenAI-compatible transcription API.
func NewHTTPVerifier(baseURL string, sampleDurationSeconds int, minSimilarity float64) (*HTTPVerifier, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("verification: whisper_url must not be empty")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("verification: invalid whisper_url: %w", err)
	}
	if sampleDurationSeconds <= 0 {
		sampleDurationSeconds = 30
	}
	if minSimilarity <= 0 || minSimilarity > 1 {
		minSimilarity = 0.35
	}
	return &HTTPVerifier{
		baseURL:               strings.TrimRight(baseURL, "/"),
		sampleDurationSeconds: sampleDurationSeconds, // reserved for ffmpeg sampling before upload
		minSimilarity:         minSimilarity,
		client:                &http.Client{Timeout: 2 * time.Minute},
	}, nil
}

// Verify transcribes audio and compares the transcript to candidate lyrics.
func (v *HTTPVerifier) Verify(ctx context.Context, audioPath string, song models.Song) (Result, error) {
	if strings.TrimSpace(audioPath) == "" {
		return Result{}, fmt.Errorf("verification: audio path is empty")
	}
	lyrics := SongText(song)
	if lyrics == "" {
		return Result{}, fmt.Errorf("verification: lyrics text is empty")
	}
	transcript, err := v.transcribe(ctx, audioPath)
	if err != nil {
		return Result{}, err
	}
	similarity := Similarity(transcript, lyrics)
	return Result{
		Accepted:   similarity >= v.minSimilarity,
		Similarity: similarity,
		Transcript: transcript,
	}, nil
}

func (v *HTTPVerifier) transcribe(ctx context.Context, audioPath string) (text string, err error) {
	f, err := os.Open(audioPath) //nolint:gosec // path comes from scanned audio file
	if err != nil {
		return "", fmt.Errorf("verification: open audio: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("verification: close audio: %w", closeErr)
		}
	}()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return "", fmt.Errorf("verification: create multipart file: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return "", fmt.Errorf("verification: copy audio: %w", err)
	}
	if err := mw.WriteField("model", "base"); err != nil {
		return "", fmt.Errorf("verification: write model field: %w", err)
	}
	if err := mw.WriteField("response_format", "json"); err != nil {
		return "", fmt.Errorf("verification: write response format field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("verification: close multipart body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.transcriptionURL(), &body)
	if err != nil {
		return "", fmt.Errorf("verification: create request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	res, err := v.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("verification: transcribe audio: %w", err)
	}
	defer func() {
		if closeErr := res.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("verification: close response body: %w", closeErr)
		}
	}()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
		return "", fmt.Errorf("verification: transcribe status %d: %s", res.StatusCode, strings.TrimSpace(string(errBody)))
	}
	const maxResponseSize = 1 << 20
	b, err := io.ReadAll(io.LimitReader(res.Body, maxResponseSize+1))
	if err != nil {
		return "", fmt.Errorf("verification: read response: %w", err)
	}
	if len(b) > maxResponseSize {
		return "", fmt.Errorf("verification: response too large (%d bytes)", len(b))
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return "", fmt.Errorf("verification: decode response: %w", err)
	}
	payload.Text = strings.TrimSpace(payload.Text)
	if payload.Text == "" {
		return "", fmt.Errorf("verification: transcript is empty")
	}
	return payload.Text, nil
}

func (v *HTTPVerifier) transcriptionURL() string {
	baseURL := strings.TrimRight(v.baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1/audio/transcriptions") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/audio/transcriptions"
	}
	return baseURL + "/v1/audio/transcriptions"
}

// SongText returns the best lyrics text available for transcript comparison.
func SongText(song models.Song) string {
	if len(song.Subtitles.Lines) > 0 {
		var b strings.Builder
		for _, line := range song.Subtitles.Lines {
			if text := strings.TrimSpace(line.Text); text != "" {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(text)
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
	}
	return strings.TrimSpace(song.Lyrics.LyricsBody)
}

// Similarity scores token overlap between a transcript and candidate lyrics.
func Similarity(transcript string, lyrics string) float64 {
	transcriptTokens := tokenSet(transcript)
	lyricsTokens := tokenSet(lyrics)
	if len(transcriptTokens) == 0 || len(lyricsTokens) == 0 {
		return 0
	}
	matches := 0
	for token := range transcriptTokens {
		if lyricsTokens[token] {
			matches++
		}
	}
	return float64(matches) / float64(len(transcriptTokens))
}

func tokenSet(s string) map[string]bool {
	fields := strings.FieldsFunc(normalize.NormalizeKey(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := make(map[string]bool, len(fields))
	for _, f := range fields {
		if len([]rune(f)) > 1 {
			out[f] = true
		}
	}
	return out
}
