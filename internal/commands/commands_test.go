package commands

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
	}}, scan.UpsertOptions{}); err != nil {
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

func TestCircuitOpenDurationConfigKey(t *testing.T) {
	cfg := config.Config{API: config.APIConfig{CircuitOpenDuration: 1800}}

	got, ok := configValue(cfg, "api.circuit_open_duration")
	if !ok {
		t.Fatal("configValue(api.circuit_open_duration) ok = false; want true")
	}
	if got != "1800" {
		t.Fatalf("configValue(api.circuit_open_duration) = %q; want %q", got, "1800")
	}

	if err := setConfigValue(&cfg, "api.circuit_open_duration", "600"); err != nil {
		t.Fatalf("setConfigValue valid: %v", err)
	}
	if cfg.API.CircuitOpenDuration != 600 {
		t.Fatalf("CircuitOpenDuration = %d; want 600", cfg.API.CircuitOpenDuration)
	}
	for _, bad := range []string{"", "abc", "0", "-30"} {
		if err := setConfigValue(&cfg, "api.circuit_open_duration", bad); err == nil {
			t.Fatalf("setConfigValue accepted invalid api.circuit_open_duration %q", bad)
		}
	}

	if !slices.Contains(configKeys(), "api.circuit_open_duration") {
		t.Fatal("configKeys missing api.circuit_open_duration")
	}
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

// commandsTestEnv prepares an isolated config + DB and pre-seeds a library and
// optional queue/scan rows. Returns the config path used by run* helpers.
func commandsTestEnv(t *testing.T) (cfgPath string, dbPath string) {
	t.Helper()
	isolateCommandsEnv(t)
	dir := t.TempDir()
	dbPath = filepath.Join(dir, "state", "test.db")
	cfgPath = writeCommandsConfig(t, dbPath)
	return cfgPath, dbPath
}

func TestRunQueueList_FiltersByStatus(t *testing.T) {
	cfg, dbPath := commandsTestEnv(t)
	ctx := context.Background()

	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	q := queue.NewDBQueue(sqlDB)
	if _, err := q.Enqueue(ctx, models.Inputs{Track: models.Track{ArtistName: "Pending", TrackName: "Track"}}, 1); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := q.Enqueue(ctx, models.Inputs{Track: models.Track{ArtistName: "Failing", TrackName: "Track"}}, 1); err != nil {
		t.Fatalf("Enqueue 2: %v", err)
	}
	claimed, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if _, err := q.Fail(ctx, claimed.ID, errorsNew("boom")); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	_ = sqlDB.Close()

	var out bytes.Buffer
	code := Run(ctx, []string{"queue", "list", "--status", "failed", "--config", cfg}, &out, Deps{})
	if code != 0 {
		t.Fatalf("Run exit code = %d; want 0; out=%s", code, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "ID") || !strings.Contains(got, "Status") || !strings.Contains(got, "LastError") {
		t.Fatalf("missing header in output: %q", got)
	}
	// The first dequeue (FIFO) claims the row inserted first ("Pending"),
	// which is then failed. So a single failed row shows up; the "Failing"
	// row stays pending and must NOT appear under --status=failed.
	if !strings.Contains(got, "Pending") {
		t.Fatalf("expected failed row (artist=Pending) in output: %q", got)
	}
	if strings.Contains(got, "Failing") {
		t.Fatalf("status filter leaked pending row: %q", got)
	}
}

func TestRunQueueRetry_RejectsNonFailedRow(t *testing.T) {
	cfg, dbPath := commandsTestEnv(t)
	ctx := context.Background()

	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	q := queue.NewDBQueue(sqlDB)
	item, err := q.Enqueue(ctx, models.Inputs{Track: models.Track{ArtistName: "Artist", TrackName: "Pending"}}, 1)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	_ = sqlDB.Close()

	var out bytes.Buffer
	code := Run(ctx, []string{"queue", "retry", strconvFormatInt(item.ID), "--config", cfg}, &out, Deps{})
	if code == 0 {
		t.Fatalf("Run exit code = 0; want non-zero (retry on pending must fail). out=%s", out.String())
	}
	if !strings.Contains(out.String(), "not in failed status") {
		t.Fatalf("expected not-retryable message; got %q", out.String())
	}
}

func TestRunQueueRetry_ResetsFailedRow(t *testing.T) {
	cfg, dbPath := commandsTestEnv(t)
	ctx := context.Background()

	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	q := queue.NewDBQueue(sqlDB)
	if _, err := q.Enqueue(ctx, models.Inputs{Track: models.Track{ArtistName: "Artist", TrackName: "Title"}}, 1); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	claimed, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if _, err := q.Fail(ctx, claimed.ID, errorsNew("boom")); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	_ = sqlDB.Close()

	var out bytes.Buffer
	code := Run(ctx, []string{"queue", "retry", strconvFormatInt(claimed.ID), "--config", cfg}, &out, Deps{})
	if code != 0 {
		t.Fatalf("Run exit code = %d; want 0; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "retried") {
		t.Fatalf("expected retried message; got %q", out.String())
	}

	// Verify state.
	sqlDB, err = db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer sqlDB.Close()
	items, err := queue.NewDBQueue(sqlDB).List(ctx, queue.ListFilter{Status: queue.StatusPending})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Attempts != 0 {
		t.Fatalf("post-retry pending items = %+v; want one with attempts=0", items)
	}
}

func TestRunQueueClear_DryRunDoesNotDelete(t *testing.T) {
	cfg, dbPath := commandsTestEnv(t)
	ctx := context.Background()

	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	q := queue.NewDBQueue(sqlDB)
	if _, err := q.Enqueue(ctx, models.Inputs{Track: models.Track{ArtistName: "Artist", TrackName: "Title"}}, 1); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	claimed, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if err := q.Complete(ctx, claimed.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	_ = sqlDB.Close()

	var out bytes.Buffer
	code := Run(ctx, []string{"queue", "clear", "--done", "--config", cfg}, &out, Deps{})
	if code != 0 {
		t.Fatalf("Run dry-run exit code = %d; want 0; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "would delete 1") {
		t.Fatalf("expected dry-run message; got %q", out.String())
	}

	// Re-open and confirm row still there.
	sqlDB, err = db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	c, err := queue.NewDBQueue(sqlDB).CountDone(ctx)
	if err != nil {
		t.Fatalf("CountDone: %v", err)
	}
	_ = sqlDB.Close()
	if c != 1 {
		t.Fatalf("dry-run deleted rows: CountDone=%d, want 1", c)
	}

	// Now actually delete.
	out.Reset()
	code = Run(ctx, []string{"queue", "clear", "--done", "--yes", "--config", cfg}, &out, Deps{})
	if code != 0 {
		t.Fatalf("Run --yes exit code = %d; want 0", code)
	}
	if !strings.Contains(out.String(), "deleted 1") {
		t.Fatalf("expected deletion message; got %q", out.String())
	}
}

func TestRunScanResults_ResolvesLibraryByNameAndID(t *testing.T) {
	cfg, dbPath := commandsTestEnv(t)
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	libRepo := library.New(sqlDB)
	lib, err := libRepo.Add(ctx, "/music", "MusicLib")
	if err != nil {
		t.Fatalf("Add library: %v", err)
	}
	scanRepo := scan.New(sqlDB)
	if err := scanRepo.Upsert(ctx, lib.ID, []models.ScanResult{{
		FilePath: "/music/a.mp3",
		Track:    models.Track{ArtistName: "Artist", TrackName: "Title"},
		Outdir:   "/music",
		Filename: "a.lrc",
	}}, scan.UpsertOptions{}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	_ = sqlDB.Close()

	for _, ref := range []string{"MusicLib", strconvFormatInt(lib.ID)} {
		var out bytes.Buffer
		code := Run(ctx, []string{"scan", "results", "--library", ref, "--config", cfg}, &out, Deps{})
		if code != 0 {
			t.Fatalf("Run --library=%q exit code = %d; want 0; out=%s", ref, code, out.String())
		}
		if !strings.Contains(out.String(), "/music/a.mp3") {
			t.Fatalf("expected scan_result row for ref=%q; got %q", ref, out.String())
		}
		if !strings.Contains(out.String(), "MusicLib") {
			t.Fatalf("expected library name in output for ref=%q; got %q", ref, out.String())
		}
	}
}

func TestRunScanClear_DryRunAndConfirm(t *testing.T) {
	cfg, dbPath := commandsTestEnv(t)
	ctx := context.Background()

	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	libRepo := library.New(sqlDB)
	lib, err := libRepo.Add(ctx, "/music", "MusicLib")
	if err != nil {
		t.Fatalf("Add library: %v", err)
	}
	scanRepo := scan.New(sqlDB)
	if err := scanRepo.Upsert(ctx, lib.ID, []models.ScanResult{
		{FilePath: "/music/a.mp3", Track: models.Track{ArtistName: "Artist", TrackName: "A"}},
		{FilePath: "/music/b.mp3", Track: models.Track{ArtistName: "Artist", TrackName: "B"}},
	}, scan.UpsertOptions{}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	_ = sqlDB.Close()

	// Dry run.
	var out bytes.Buffer
	code := Run(ctx, []string{"scan", "clear", "--library", "MusicLib", "--config", cfg}, &out, Deps{})
	if code != 0 {
		t.Fatalf("dry-run exit code = %d; want 0; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "would delete 2") {
		t.Fatalf("expected dry-run message; got %q", out.String())
	}

	// Confirm dry run did nothing.
	sqlDB, err = db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	c, err := scan.New(sqlDB).CountByLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatalf("CountByLibrary: %v", err)
	}
	_ = sqlDB.Close()
	if c != 2 {
		t.Fatalf("CountByLibrary post dry-run = %d; want 2", c)
	}

	// Confirm.
	out.Reset()
	code = Run(ctx, []string{"scan", "clear", "--library", "MusicLib", "--yes", "--config", cfg}, &out, Deps{})
	if code != 0 {
		t.Fatalf("--yes exit code = %d; want 0; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "deleted 2") {
		t.Fatalf("expected deletion message; got %q", out.String())
	}

	// Confirm library still exists.
	sqlDB, err = db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer sqlDB.Close()
	if _, err := library.New(sqlDB).Get(ctx, lib.ID); err != nil {
		t.Fatalf("library removed by scan clear: %v", err)
	}
}

func errorsNew(s string) error { return errors.New(s) }

func strconvFormatInt(i int64) string { return strconv.FormatInt(i, 10) }

func TestValidateQueueStatus(t *testing.T) {
	for _, ok := range []string{"", "pending", "processing", "failed", "done"} {
		if err := validateQueueStatus(ok); err != nil {
			t.Errorf("validateQueueStatus(%q) = %v; want nil", ok, err)
		}
	}
	for _, bad := range []string{"running", "PENDING", "x"} {
		if err := validateQueueStatus(bad); err == nil {
			t.Errorf("validateQueueStatus(%q) = nil; want error", bad)
		}
	}
}

func TestValidateScanStatus(t *testing.T) {
	for _, ok := range []string{"", "pending", "processing", "done", "failed"} {
		if err := validateScanStatus(ok); err != nil {
			t.Errorf("validateScanStatus(%q) = %v; want nil", ok, err)
		}
	}
	for _, bad := range []string{"queued", "DONE", "?"} {
		if err := validateScanStatus(bad); err == nil {
			t.Errorf("validateScanStatus(%q) = nil; want error", bad)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"", 5, ""},
		{"abc", 5, "abc"},
		{"abcdef", 6, "abcdef"},
		{"abcdef", 5, "ab..."},
		{"abcdef", 3, "abc"},
		{"abcdef", 1, "a"},
		{"abcdef", 0, "abcdef"},
		{"abcdef", -1, "abcdef"},
	}
	for _, tc := range cases {
		got := truncate(tc.in, tc.max)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q; want %q", tc.in, tc.max, got, tc.want)
		}
	}
}

func TestRunQueueList_InvalidStatusReturnsError(t *testing.T) {
	cfg, _ := commandsTestEnv(t)
	var out bytes.Buffer
	code := Run(context.Background(), []string{"queue", "list", "--status", "bogus", "--config", cfg}, &out, Deps{})
	if code == 0 {
		t.Fatalf("queue list with bogus status: exit code 0; want non-zero. out=%s", out.String())
	}
	if !strings.Contains(out.String(), "invalid status") {
		t.Fatalf("output missing 'invalid status': %q", out.String())
	}
}

func TestRunQueueRetry_MissingIDSurfacesNotFound(t *testing.T) {
	cfg, _ := commandsTestEnv(t)
	var out bytes.Buffer
	code := Run(context.Background(), []string{"queue", "retry", "--config", cfg, "9999"}, &out, Deps{})
	if code == 0 {
		t.Fatalf("queue retry of missing id: exit code 0; want non-zero. out=%s", out.String())
	}
}

func TestRunQueueClear_ConfirmDeletes(t *testing.T) {
	cfg, dbPath := commandsTestEnv(t)
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	q := queue.NewDBQueue(sqlDB)
	item, err := q.Enqueue(ctx, models.Inputs{Track: models.Track{ArtistName: "A", TrackName: "Done"}}, 1)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := q.Dequeue(ctx); err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if err := q.Complete(ctx, item.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	_ = sqlDB.Close()

	var out bytes.Buffer
	code := Run(ctx, []string{"queue", "clear", "--done", "--yes", "--config", cfg}, &out, Deps{})
	if code != 0 {
		t.Fatalf("queue clear --yes exit code = %d; want 0. out=%s", code, out.String())
	}
}

func TestRunScanResults_InvalidStatusReturnsError(t *testing.T) {
	cfg, _ := commandsTestEnv(t)
	var out bytes.Buffer
	code := Run(context.Background(), []string{"scan", "results", "--status", "bogus", "--config", cfg}, &out, Deps{})
	if code == 0 {
		t.Fatalf("scan results bogus status: exit code 0; want non-zero. out=%s", out.String())
	}
	if !strings.Contains(out.String(), "invalid status") {
		t.Fatalf("output missing 'invalid status': %q", out.String())
	}
}

func TestRunScanClear_RequiresLibrary(t *testing.T) {
	cfg, _ := commandsTestEnv(t)
	var out bytes.Buffer
	code := Run(context.Background(), []string{"scan", "clear", "--library", "Nonexistent", "--config", cfg}, &out, Deps{})
	if code == 0 {
		t.Fatalf("scan clear unknown library: exit code 0; want non-zero. out=%s", out.String())
	}
}

func TestResolveLibraryRejectsBlankRef(t *testing.T) {
	cfg, dbPath := commandsTestEnv(t)
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()
	repo := library.New(sqlDB)
	if _, err := resolveLibrary(ctx, repo, ""); err == nil {
		t.Fatal("resolveLibrary empty ref returned nil error")
	}
	if _, err := resolveLibrary(ctx, repo, "  "); err == nil {
		t.Fatal("resolveLibrary whitespace ref returned nil error")
	}
	_ = cfg
}

func TestRunQueueCmd_MissingSubcommand(t *testing.T) {
	var out bytes.Buffer
	code := runQueueCmd(context.Background(), &out, QueueCmd{})
	if code != 2 {
		t.Fatalf("runQueueCmd empty = %d; want 2", code)
	}
	if !strings.Contains(out.String(), "missing queue subcommand") {
		t.Fatalf("output = %q; want missing-subcommand message", out.String())
	}
}

func TestRunQueueCmd_FailedRoutesToList(t *testing.T) {
	cfg, _ := commandsTestEnv(t)
	var out bytes.Buffer
	code := runQueueCmd(context.Background(), &out, QueueCmd{Failed: &QueueFailedCmd{ConfigPath: cfg, Limit: 5}})
	if code != 0 {
		t.Fatalf("queue failed = %d; want 0. out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "ID") {
		t.Fatalf("queue failed missing header: %q", out.String())
	}
}

func TestRunQueueClear_RequiresDoneFlag(t *testing.T) {
	cfg, _ := commandsTestEnv(t)
	var out bytes.Buffer
	code := runQueueClear(context.Background(), &out, QueueClearCmd{ConfigPath: cfg, Done: false})
	if code != 2 {
		t.Fatalf("queue clear without --done = %d; want 2", code)
	}
	if !strings.Contains(out.String(), "requires --done") {
		t.Fatalf("output = %q; want --done required message", out.String())
	}
}

func TestRunScanResults_EmptyDBPrintsHeaderOnly(t *testing.T) {
	cfg, _ := commandsTestEnv(t)
	var out bytes.Buffer
	code := runScanResults(context.Background(), &out, ScanResultsCmd{ConfigPath: cfg})
	if code != 0 {
		t.Fatalf("scan results empty = %d; want 0. out=%s", code, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "ID") || !strings.Contains(got, "Library") {
		t.Fatalf("output missing header: %q", got)
	}
}

func TestRunQueueList_EmptyDBPrintsHeaderOnly(t *testing.T) {
	cfg, _ := commandsTestEnv(t)
	var out bytes.Buffer
	code := runQueueList(context.Background(), &out, QueueListCmd{ConfigPath: cfg})
	if code != 0 {
		t.Fatalf("queue list empty = %d; want 0. out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "ID") {
		t.Fatalf("output missing header: %q", out.String())
	}
}

func TestRunQueueClear_DryRunCountsZero(t *testing.T) {
	cfg, _ := commandsTestEnv(t)
	var out bytes.Buffer
	code := runQueueClear(context.Background(), &out, QueueClearCmd{ConfigPath: cfg, Done: true, Yes: false})
	if code != 0 {
		t.Fatalf("dry-run on empty db = %d; want 0. out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "would delete 0") {
		t.Fatalf("dry-run output = %q; want 'would delete 0'", out.String())
	}
}

func TestRunScanClear_DryRunOnEmpty(t *testing.T) {
	cfg, dbPath := commandsTestEnv(t)
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	libRepo := library.New(sqlDB)
	if _, err := libRepo.Add(ctx, "/music", "Music"); err != nil {
		t.Fatalf("Add library: %v", err)
	}
	_ = sqlDB.Close()

	var out bytes.Buffer
	code := runScanClear(ctx, &out, ScanClearCmd{ConfigPath: cfg, Library: "Music", Yes: false})
	if code != 0 {
		t.Fatalf("scan clear dry-run = %d; want 0. out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "would delete 0") {
		t.Fatalf("scan clear dry-run output = %q; want 'would delete 0'", out.String())
	}
}

func TestRunScanClear_RequiresLibraryFlag(t *testing.T) {
	cfg, _ := commandsTestEnv(t)
	var out bytes.Buffer
	code := runScanClear(context.Background(), &out, ScanClearCmd{ConfigPath: cfg})
	if code == 0 {
		t.Fatalf("scan clear without --library = 0; want non-zero")
	}
}

func TestRunScanResults_FilterByLibraryByID(t *testing.T) {
	cfg, dbPath := commandsTestEnv(t)
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	libRepo := library.New(sqlDB)
	lib, err := libRepo.Add(ctx, "/music", "Music")
	if err != nil {
		t.Fatalf("Add library: %v", err)
	}
	scanRepo := scan.New(sqlDB)
	if err := scanRepo.Upsert(ctx, lib.ID, []models.ScanResult{{
		FilePath: "/music/a.mp3",
		Track:    models.Track{ArtistName: "A", TrackName: "Track"},
		Outdir:   "/music",
		Filename: "a.lrc",
		Status:   scan.StatusPending,
	}}, scan.UpsertOptions{}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	_ = sqlDB.Close()

	var out bytes.Buffer
	code := runScanResults(ctx, &out, ScanResultsCmd{ConfigPath: cfg, Library: strconv.FormatInt(lib.ID, 10)})
	if code != 0 {
		t.Fatalf("scan results --library <id> = %d; want 0. out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "/music/a.mp3") {
		t.Fatalf("scan results --library <id> missing row: %q", out.String())
	}
}

func TestResolveLibrary_NumericNameLooksUpByName(t *testing.T) {
	cfg, dbPath := commandsTestEnv(t)
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()
	repo := library.New(sqlDB)
	added, err := repo.Add(ctx, "/music/numeric", "9999")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, err := resolveLibrary(ctx, repo, "9999")
	if err != nil {
		t.Fatalf("resolveLibrary numeric-name: %v", err)
	}
	if got.ID != added.ID {
		t.Fatalf("resolveLibrary id = %d; want %d", got.ID, added.ID)
	}
	_ = cfg
}

func TestResolveLibrary_NumericRefAmbiguousIDvsName(t *testing.T) {
	cfg, dbPath := commandsTestEnv(t)
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()
	repo := library.New(sqlDB)
	// Library #1 (Music)
	if _, err := repo.Add(ctx, "/music/a", "Music"); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	// Library #2 named "1" — ref "1" now matches BOTH id=1 and name="1".
	if _, err := repo.Add(ctx, "/music/b", "1"); err != nil {
		t.Fatalf("Add b: %v", err)
	}

	_, err = resolveLibrary(ctx, repo, "1")
	if err == nil {
		t.Fatal("resolveLibrary returned nil error for ambiguous ID-vs-name match")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err = %v; want substring 'ambiguous'", err)
	}
	_ = cfg
}
