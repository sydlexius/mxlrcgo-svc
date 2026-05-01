package commands

import (
	"bytes"
	"context"
	"strings"
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

func TestNewVerifierDisabledDoesNotRequireFFmpeg(t *testing.T) {
	got, err := newVerifier(config.Config{
		Verification: config.VerificationConfig{
			Enabled:    false,
			FFmpegPath: "/path/that/does/not/exist",
		},
	})
	if err != nil {
		t.Fatalf("newVerifier: %v", err)
	}
	if got != nil {
		t.Fatalf("newVerifier = %#v; want nil", got)
	}
}

func TestConfigureWorkerVerificationAcceptsNilVerifier(t *testing.T) {
	w := worker.New(nil, nil, fakeFetcher{}, fakeWriter{})
	configureWorkerVerification(w, config.Config{}, nil)
}

func TestRunSubcommandHelpShowsSelectedCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "serve",
			args: []string{"serve", "--help"},
			want: []string{"Usage: mxlrcgo-svc serve", "--scan-interval", "--work-interval"},
		},
		{
			name: "scan",
			args: []string{"scan", "--help"},
			want: []string{"Usage: mxlrcgo-svc scan", "--upgrade", "--bfs"},
		},
		{
			name: "library",
			args: []string{"library", "--help"},
			want: []string{"Usage: mxlrcgo-svc library", "add", "list"},
		},
		{
			name: "library add",
			args: []string{"library", "add", "--help"},
			want: []string{"Usage: mxlrcgo-svc library add", "--name", "--config"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			code := Run(context.Background(), tc.args, &out, Deps{})
			if code != 0 {
				t.Fatalf("Run exit code = %d; want 0", code)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("help output = %q; want %q", out.String(), want)
				}
			}
		})
	}
}

func TestRunSubcommandParseErrorShowsSelectedUsage(t *testing.T) {
	var out bytes.Buffer
	code := Run(context.Background(), []string{"serve", "--not-a-real-flag"}, &out, Deps{})
	if code != 2 {
		t.Fatalf("Run exit code = %d; want 2", code)
	}
	if !strings.Contains(out.String(), "Usage: mxlrcgo-svc serve") {
		t.Fatalf("usage output = %q; want serve usage", out.String())
	}
	if strings.Contains(out.String(), "Usage: mxlrcgo-svc <command>") {
		t.Fatalf("usage output = %q; want selected subcommand usage, not top-level usage", out.String())
	}
}

func TestVerificationConfigKeys(t *testing.T) {
	cfg := config.Config{
		Verification: config.VerificationConfig{
			Enabled:               true,
			WhisperURL:            "http://whisper:9000",
			FFmpegPath:            "/usr/bin/ffmpeg",
			SampleDurationSeconds: 45,
			MinConfidence:         0.7,
			MinSimilarity:         0.5,
		},
	}
	tests := map[string]string{
		"verification.enabled":                 "true",
		"verification.whisper_url":             "http://whisper:9000",
		"verification.ffmpeg_path":             "/usr/bin/ffmpeg",
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
	if err := setConfigValue(&cfg, "verification.ffmpeg_path", " "); err == nil {
		t.Fatal("setConfigValue accepted blank verification.ffmpeg_path")
	}
}

func TestConfigKeysIncludesVerificationFFmpegPath(t *testing.T) {
	for _, key := range configKeys() {
		if key == "verification.ffmpeg_path" {
			return
		}
	}
	t.Fatal("configKeys missing verification.ffmpeg_path")
}

var _ lyrics.Writer = fakeWriter{}
