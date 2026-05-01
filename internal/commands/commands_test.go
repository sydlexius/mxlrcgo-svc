package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sydlexius/mxlrcgo-svc/internal/config"
	"github.com/sydlexius/mxlrcgo-svc/internal/db"
	"github.com/sydlexius/mxlrcgo-svc/internal/library"
	"github.com/sydlexius/mxlrcgo-svc/internal/lyrics"
	"github.com/sydlexius/mxlrcgo-svc/internal/models"
	"github.com/sydlexius/mxlrcgo-svc/internal/musixmatch"
	"github.com/sydlexius/mxlrcgo-svc/internal/queue"
	"github.com/sydlexius/mxlrcgo-svc/internal/scan"
	"github.com/sydlexius/mxlrcgo-svc/internal/scanner"
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

func TestNormalizeWorkerInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		want     time.Duration
	}{
		{name: "zero", interval: 0, want: 15 * time.Second},
		{name: "below minimum", interval: 5 * time.Second, want: 15 * time.Second},
		{name: "minimum", interval: 15 * time.Second, want: 15 * time.Second},
		{name: "above minimum", interval: 30 * time.Second, want: 30 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeWorkerInterval(tc.interval); got != tc.want {
				t.Fatalf("normalizeWorkerInterval(%s) = %s; want %s", tc.interval, got, tc.want)
			}
		})
	}
}

func TestServeWorkerIntervalUsesConfigUnlessFlagProvided(t *testing.T) {
	cfg := config.Config{
		API: config.APIConfig{Cooldown: 45},
	}

	if got := serveWorkerInterval(cfg, ServeCmd{}); got != 45*time.Second {
		t.Fatalf("serveWorkerInterval without flag = %s; want 45s", got)
	}

	flag := 30
	if got := serveWorkerInterval(cfg, ServeCmd{WorkInterval: &flag}); got != 30*time.Second {
		t.Fatalf("serveWorkerInterval with flag = %s; want 30s", got)
	}
}

func TestSchedulerBuildsScanEnqueuer(t *testing.T) {
	sqlDB, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	libRepo := library.New(sqlDB)
	lib, err := libRepo.Add(context.Background(), "/music", "Music")
	if err != nil {
		t.Fatalf("Add library: %v", err)
	}
	scanRepo := scan.New(sqlDB)
	if err := scanRepo.Upsert(context.Background(), lib.ID, []models.ScanResult{{
		FilePath: "/music/a.mp3",
		Track:    models.Track{ArtistName: "Artist", TrackName: "Title"},
		Outdir:   "/music",
		Filename: "a.lrc",
		Status:   scan.StatusPending,
	}}); err != nil {
		t.Fatalf("Upsert scan result: %v", err)
	}

	got := scheduler(sqlDB, scanner.ScanOptions{MaxDepth: 7})
	if got.OnScanComplete == nil {
		t.Fatal("scheduler OnScanComplete = nil; want enqueue callback")
	}
	if err := got.OnScanComplete(context.Background(), models.Library{ID: lib.ID}, nil); err != nil {
		t.Fatalf("OnScanComplete: %v", err)
	}
	item, err := queue.NewDBQueue(sqlDB).Dequeue(context.Background())
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if item.Priority != queue.PriorityScan {
		t.Fatalf("Dequeue priority = %d; want scan priority %d", item.Priority, queue.PriorityScan)
	}
}

func TestRunLibraryUpdate(t *testing.T) {
	isolateCommandsEnv(t)
	ctx := context.Background()
	dir := t.TempDir()
	cfg := writeCommandsConfig(t, filepath.Join(dir, "state", "test.db"))
	libPath := filepath.Join(dir, "music")
	if err := os.Mkdir(libPath, 0o750); err != nil {
		t.Fatalf("mkdir library: %v", err)
	}

	var out bytes.Buffer
	code := runLibrary(ctx, &out, LibraryCmd{Add: &LibraryAddCmd{
		Path:       libPath,
		Name:       "Music",
		ConfigPath: cfg,
	}})
	if code != 0 {
		t.Fatalf("library add exit code = %d; want 0", code)
	}

	renamedPath := filepath.Join(dir, "renamed")
	if err := os.Mkdir(renamedPath, 0o750); err != nil {
		t.Fatalf("mkdir renamed library: %v", err)
	}
	out.Reset()
	code = runLibrary(ctx, &out, LibraryCmd{Update: &LibraryUpdateCmd{
		ID:         1,
		Path:       renamedPath,
		Name:       "Renamed",
		ConfigPath: cfg,
	}})
	if code != 0 {
		t.Fatalf("library update exit code = %d; want 0", code)
	}
	gotOut := strings.TrimSpace(out.String())
	wantOut := "1\tRenamed\t" + renamedPath
	if gotOut != wantOut {
		t.Fatalf("library update output = %q; want %q", gotOut, wantOut)
	}

	out.Reset()
	code = runLibrary(ctx, &out, LibraryCmd{Update: &LibraryUpdateCmd{
		ID:         1,
		Name:       "Display",
		ConfigPath: cfg,
	}})
	if code != 0 {
		t.Fatalf("library update name exit code = %d; want 0", code)
	}
	gotOut = strings.TrimSpace(out.String())
	wantOut = "1\tDisplay\t" + renamedPath
	if gotOut != wantOut {
		t.Fatalf("library update name output = %q; want %q", gotOut, wantOut)
	}
}

func TestRunLibraryUpdateFailures(t *testing.T) {
	isolateCommandsEnv(t)
	ctx := context.Background()
	dir := t.TempDir()
	cfg := writeCommandsConfig(t, filepath.Join(dir, "state", "test.db"))

	var out bytes.Buffer
	code := runLibrary(ctx, &out, LibraryCmd{Update: &LibraryUpdateCmd{
		ID:         1,
		ConfigPath: cfg,
	}})
	if code != 2 {
		t.Fatalf("library update without changes exit code = %d; want 2", code)
	}
	if !strings.Contains(out.String(), "requires --path") {
		t.Fatalf("library update without changes output = %q; want validation message", out.String())
	}

	out.Reset()
	code = runLibrary(ctx, &out, LibraryCmd{Update: &LibraryUpdateCmd{
		ID:         99,
		Name:       "Missing",
		ConfigPath: cfg,
	}})
	if code != 1 {
		t.Fatalf("library update missing exit code = %d; want 1", code)
	}
	if !strings.Contains(out.String(), "library 99 not found") {
		t.Fatalf("library update missing output = %q; want not-found message", out.String())
	}

	libPath := filepath.Join(dir, "music")
	if err := os.Mkdir(libPath, 0o750); err != nil {
		t.Fatalf("mkdir library: %v", err)
	}
	out.Reset()
	code = runLibrary(ctx, &out, LibraryCmd{Add: &LibraryAddCmd{
		Path:       libPath,
		Name:       "Music",
		ConfigPath: cfg,
	}})
	if code != 0 {
		t.Fatalf("library add exit code = %d; want 0", code)
	}

	out.Reset()
	code = runLibrary(ctx, &out, LibraryCmd{Update: &LibraryUpdateCmd{
		ID:         1,
		Name:       " ",
		ConfigPath: cfg,
	}})
	if code != 1 {
		t.Fatalf("library update invalid exit code = %d; want 1", code)
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

func isolateCommandsEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{
		"MUSIXMATCH_TOKEN", "MXLRC_API_TOKEN",
		"MXLRC_API_COOLDOWN", "MXLRC_COOLDOWN",
		"MXLRC_OUTPUT_DIR", "MXLRC_DB_PATH", "MXLRC_SERVER_ADDR", "MXLRC_WEBHOOK_API_KEY",
		"MXLRC_PROVIDER_PRIMARY", "MXLRC_PROVIDERS_DISABLED",
		"MXLRC_VERIFICATION_ENABLED", "MXLRC_VERIFICATION_WHISPER_URL", "MXLRC_WHISPER_URL",
		"MXLRC_VERIFICATION_SAMPLE_DURATION_SECONDS", "MXLRC_VERIFICATION_SAMPLE_DURATION",
		"MXLRC_VERIFICATION_MIN_CONFIDENCE", "MXLRC_VERIFICATION_MIN_SIMILARITY",
	} {
		t.Setenv(v, "")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func writeCommandsConfig(t *testing.T, dbPath string) string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "config.toml")
	content := "[db]\npath = \"" + filepath.ToSlash(dbPath) + "\"\n"
	if err := os.WriteFile(cfg, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfg
}

var _ lyrics.Writer = fakeWriter{}
