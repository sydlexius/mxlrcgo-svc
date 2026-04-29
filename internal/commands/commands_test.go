package commands

import (
	"context"
	"testing"

	"github.com/sydlexius/mxlrcgo-svc/internal/config"
	"github.com/sydlexius/mxlrcgo-svc/internal/lyrics"
	"github.com/sydlexius/mxlrcgo-svc/internal/models"
	"github.com/sydlexius/mxlrcgo-svc/internal/musixmatch"
	"github.com/sydlexius/mxlrcgo-svc/internal/worker"
)

type fakeFetcher struct{}

func (fakeFetcher) FindLyrics(context.Context, models.Track) (models.Song, error) {
	return models.Song{}, nil
}

type fakeWriter struct{}

func (fakeWriter) WriteLRC(models.Song, string, string) error {
	return nil
}

func TestSelectedProvider(t *testing.T) {
	cfg := config.Config{Providers: config.ProvidersConfig{Primary: "musixmatch"}}
	got, err := selectedProvider(cfg, "token", func(string) musixmatch.Fetcher { return fakeFetcher{} })
	if err != nil {
		t.Fatalf("selectedProvider: %v", err)
	}
	if got.Name() != "musixmatch" {
		t.Fatalf("provider name = %q; want musixmatch", got.Name())
	}

	cfg.Providers.Disabled = []string{"musixmatch"}
	if _, err := selectedProvider(cfg, "token", func(string) musixmatch.Fetcher { return fakeFetcher{} }); err == nil {
		t.Fatal("selectedProvider returned nil error for disabled provider")
	}
}

func TestNewVerifierRequiresURLWhenEnabled(t *testing.T) {
	_, err := newVerifier(config.Config{
		Verification: config.VerificationConfig{Enabled: true},
	})
	if err == nil {
		t.Fatal("newVerifier returned nil error; want missing URL error")
	}
}

func TestConfigureWorkerVerificationAcceptsNilVerifier(t *testing.T) {
	w := worker.New(nil, nil, fakeFetcher{}, fakeWriter{})
	configureWorkerVerification(w, config.Config{}, nil)
}

func TestVerificationConfigKeys(t *testing.T) {
	cfg := config.Config{
		Verification: config.VerificationConfig{
			Enabled:               true,
			WhisperURL:            "http://whisper:9000",
			SampleDurationSeconds: 45,
			MinConfidence:         0.7,
			MinSimilarity:         0.5,
		},
	}
	tests := map[string]string{
		"verification.enabled":                 "true",
		"verification.whisper_url":             "http://whisper:9000",
		"verification.sample_duration_seconds": "45",
		"verification.min_confidence":          "0.7",
		"verification.min_similarity":          "0.5",
	}
	for key, want := range tests {
		got, ok := configValue(cfg, key)
		if !ok {
			t.Fatalf("configValue(%q) ok = false; want true", key)
		}
		if got != want {
			t.Fatalf("configValue(%q) = %q; want %q", key, got, want)
		}
	}

	if err := setConfigValue(&cfg, "verification.min_similarity", "0"); err == nil {
		t.Fatal("setConfigValue accepted invalid verification.min_similarity")
	}
}

var _ lyrics.Writer = fakeWriter{}
