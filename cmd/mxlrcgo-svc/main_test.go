package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sydlexius/mxlrcgo-svc/internal/lyrics"
	"github.com/sydlexius/mxlrcgo-svc/internal/models"
	"github.com/sydlexius/mxlrcgo-svc/internal/musixmatch"
	"github.com/sydlexius/mxlrcgo-svc/internal/queue"
)

type runRecord struct {
	token       string
	findCalls   int
	appCreated  bool
	runCalls    int
	cooldown    int
	mode        string
	inputs      []models.Inputs
	runErr      error
	createAppFn func(*runRecord)
}

type fakeFetcher struct {
	rec *runRecord
}

func (f *fakeFetcher) FindLyrics(context.Context, models.Track) (models.Song, error) {
	f.rec.findCalls++
	return models.Song{}, errors.New("unexpected lyrics fetch in startup test")
}

type fakeWriter struct{}

func (fakeWriter) WriteLRC(models.Song, string, string) error {
	return errors.New("unexpected LRC write in startup test")
}

type fakeRunner struct {
	rec *runRecord
}

func (r fakeRunner) Run(context.Context) error {
	r.rec.runCalls++
	return r.rec.runErr
}

func isolateCLIEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{
		"MUSIXMATCH_TOKEN", "MXLRC_API_TOKEN",
		"MXLRC_API_COOLDOWN", "MXLRC_COOLDOWN",
		"MXLRC_OUTPUT_DIR", "MXLRC_DB_PATH", "MXLRC_SERVER_ADDR", "MXLRC_WEBHOOK_API_KEY",
	} {
		t.Setenv(v, "")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func TestWebhookAuthServiceValidatesConfiguredKey(t *testing.T) {
	svc, err := webhookAuthService([]string{" mxlrc_configured "})
	if err != nil {
		t.Fatalf("webhookAuthService: %v", err)
	}
	if _, err := svc.ValidateKey(context.Background(), "mxlrc_configured", "webhook"); err != nil {
		t.Fatalf("ValidateKey configured key: %v", err)
	}
}

func TestWebhookAuthServiceRejectsMalformedKey(t *testing.T) {
	if _, err := webhookAuthService([]string{"secret"}); err == nil {
		t.Fatal("webhookAuthService malformed key returned nil error")
	}
}

func writeConfig(t *testing.T, token string, cooldown int, outdir string, dbPath string) string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "config.toml")
	content := "[api]\n" +
		"token = \"" + token + "\"\n" +
		"cooldown = " + strconv.Itoa(cooldown) + "\n" +
		"[output]\n" +
		"dir = \"" + filepath.ToSlash(outdir) + "\"\n" +
		"[db]\n" +
		"path = \"" + filepath.ToSlash(dbPath) + "\"\n"
	if err := os.WriteFile(cfg, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfg
}

func runStartup(t *testing.T, args []string, rec *runRecord) int {
	t.Helper()
	return runStartupWithDotenv(t, args, rec, func() error { return nil })
}

func runStartupWithDotenv(t *testing.T, args []string, rec *runRecord, loadDotenv func() error) int {
	t.Helper()
	return runWithOptions(runOptions{
		args:       args,
		loadDotenv: loadDotenv,
		newFetcher: func(token string) musixmatch.Fetcher {
			rec.token = token
			return &fakeFetcher{rec: rec}
		},
		newWriter: func() lyrics.Writer {
			return fakeWriter{}
		},
		newApp: func(_ musixmatch.Fetcher, _ lyrics.Writer, inputs *queue.InputsQueue, cooldown int, mode string) appRunner {
			rec.appCreated = true
			rec.cooldown = cooldown
			rec.mode = mode
			for !inputs.Empty() {
				v, err := inputs.Pop()
				if err != nil {
					t.Fatalf("pop input: %v", err)
				}
				rec.inputs = append(rec.inputs, v)
			}
			if rec.createAppFn != nil {
				rec.createAppFn(rec)
			}
			return fakeRunner{rec: rec}
		},
	})
}

func TestRunWithOptions_HelpDoesNotStartApplication(t *testing.T) {
	isolateCLIEnv(t)
	var out bytes.Buffer
	rec := &runRecord{}

	code := runWithOptions(runOptions{
		args:       []string{"--help"},
		out:        &out,
		loadDotenv: func() error { return nil },
		newFetcher: func(token string) musixmatch.Fetcher {
			rec.token = token
			return &fakeFetcher{rec: rec}
		},
		newWriter: func() lyrics.Writer { return fakeWriter{} },
		newApp: func(musixmatch.Fetcher, lyrics.Writer, *queue.InputsQueue, int, string) appRunner {
			rec.appCreated = true
			return fakeRunner{rec: rec}
		},
	})
	if code != 0 {
		t.Fatalf("run exit code = %d; want 0", code)
	}
	if rec.appCreated {
		t.Fatal("app was created; want help to stop before startup")
	}
	if !strings.Contains(out.String(), "Usage: mxlrcgo-svc") {
		t.Fatalf("help output = %q; want usage", out.String())
	}
}

func TestRunWithOptions_CLIPrecedenceAndPairInput(t *testing.T) {
	isolateCLIEnv(t)
	dir := t.TempDir()
	cfgOut := filepath.Join(dir, "config-out")
	envOut := filepath.Join(dir, "env-out")
	cliOut := filepath.Join(dir, "cli-out")
	dbPath := filepath.Join(dir, "state", "test.db")
	cfg := writeConfig(t, "config-token", 9, cfgOut, dbPath)
	t.Setenv("MUSIXMATCH_TOKEN", "env-token")
	t.Setenv("MXLRC_API_COOLDOWN", "7")
	t.Setenv("MXLRC_OUTPUT_DIR", envOut)

	rec := &runRecord{}
	code := runStartup(t, []string{
		"--config", cfg,
		"--token", "cli-token",
		"--cooldown", "0",
		"--outdir", cliOut,
		"Artist,Title",
	}, rec)
	if code != 0 {
		t.Fatalf("run exit code = %d; want 0", code)
	}
	if rec.token != "cli-token" {
		t.Fatalf("token = %q; want CLI token", rec.token)
	}
	if rec.cooldown != 0 {
		t.Fatalf("cooldown = %d; want CLI cooldown 0", rec.cooldown)
	}
	if rec.mode != "cli" {
		t.Fatalf("mode = %q; want cli", rec.mode)
	}
	if len(rec.inputs) != 1 {
		t.Fatalf("inputs = %d; want 1", len(rec.inputs))
	}
	got := rec.inputs[0]
	if got.Track.ArtistName != "Artist" || got.Track.TrackName != "Title" {
		t.Fatalf("track = %+v; want Artist/Title", got.Track)
	}
	if got.Outdir != cliOut {
		t.Fatalf("outdir = %q; want %q", got.Outdir, cliOut)
	}
	if rec.findCalls != 0 {
		t.Fatalf("FindLyrics calls = %d; want 0", rec.findCalls)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("stat db path: %v", err)
	}
}

func TestRunWithOptions_DotenvTokenPrecedence(t *testing.T) {
	isolateCLIEnv(t)
	dir := t.TempDir()
	cfg := writeConfig(t, "config-token", 1, filepath.Join(dir, "out"), filepath.Join(dir, "state", "test.db"))
	loadDotenv := func() error {
		if os.Getenv("MUSIXMATCH_TOKEN") == "" {
			t.Setenv("MUSIXMATCH_TOKEN", "dotenv-token")
		}
		return nil
	}

	rec := &runRecord{}
	code := runStartupWithDotenv(t, []string{"--config", cfg, "Artist,Title"}, rec, loadDotenv)
	if code != 0 {
		t.Fatalf("run exit code = %d; want 0", code)
	}
	if rec.token != "dotenv-token" {
		t.Fatalf("token = %q; want dotenv token", rec.token)
	}

	t.Setenv("MUSIXMATCH_TOKEN", "env-token")
	rec = &runRecord{}
	code = runStartupWithDotenv(t, []string{"--config", cfg, "Artist,Title"}, rec, loadDotenv)
	if code != 0 {
		t.Fatalf("run exit code = %d; want 0", code)
	}
	if rec.token != "env-token" {
		t.Fatalf("token = %q; want env token", rec.token)
	}

	rec = &runRecord{}
	code = runStartupWithDotenv(t, []string{"--config", cfg, "--token", "cli-token", "Artist,Title"}, rec, loadDotenv)
	if code != 0 {
		t.Fatalf("run exit code = %d; want 0", code)
	}
	if rec.token != "cli-token" {
		t.Fatalf("token = %q; want CLI token", rec.token)
	}
}

func TestRunWithOptions_EnvPrecedenceOverConfig(t *testing.T) {
	isolateCLIEnv(t)
	dir := t.TempDir()
	cfgOut := filepath.Join(dir, "config-out")
	envOut := filepath.Join(dir, "env-out")
	dbPath := filepath.Join(dir, "state", "test.db")
	cfg := writeConfig(t, "config-token", 9, cfgOut, dbPath)
	t.Setenv("MUSIXMATCH_TOKEN", "env-token")
	t.Setenv("MXLRC_API_COOLDOWN", "7")
	t.Setenv("MXLRC_OUTPUT_DIR", envOut)

	rec := &runRecord{}
	code := runStartup(t, []string{"--config", cfg, "Artist,Title"}, rec)
	if code != 0 {
		t.Fatalf("run exit code = %d; want 0", code)
	}
	if rec.token != "env-token" {
		t.Fatalf("token = %q; want env token", rec.token)
	}
	if rec.cooldown != 7 {
		t.Fatalf("cooldown = %d; want env cooldown 7", rec.cooldown)
	}
	if len(rec.inputs) != 1 || rec.inputs[0].Outdir != envOut {
		t.Fatalf("inputs = %+v; want env output dir %q", rec.inputs, envOut)
	}
}

func TestRunWithOptions_MissingTokenFailsBeforeAppRun(t *testing.T) {
	isolateCLIEnv(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state", "test.db")
	cfg := writeConfig(t, "", 1, filepath.Join(dir, "out"), dbPath)

	rec := &runRecord{}
	code := runStartup(t, []string{"--config", cfg, "Artist,Title"}, rec)
	if code == 0 {
		t.Fatal("run exit code = 0; want failure")
	}
	if rec.appCreated {
		t.Fatal("app was created; want missing token to stop startup before app creation")
	}
	if rec.findCalls != 0 {
		t.Fatalf("FindLyrics calls = %d; want 0", rec.findCalls)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("stat db path error = %v; want not exist", err)
	}
}

func TestRunWithOptions_TextFileInputSetup(t *testing.T) {
	isolateCLIEnv(t)
	dir := t.TempDir()
	outdir := filepath.Join(dir, "lyrics")
	dbPath := filepath.Join(dir, "state", "test.db")
	cfg := writeConfig(t, "config-token", 1, outdir, dbPath)
	textFile := filepath.Join(dir, "songs.txt")
	if err := os.WriteFile(textFile, []byte("Artist A,Title A\nArtist B,Title B\n"), 0o600); err != nil {
		t.Fatalf("write text input: %v", err)
	}

	rec := &runRecord{}
	code := runStartup(t, []string{"--config", cfg, textFile}, rec)
	if code != 0 {
		t.Fatalf("run exit code = %d; want 0", code)
	}
	if rec.mode != "text" {
		t.Fatalf("mode = %q; want text", rec.mode)
	}
	if len(rec.inputs) != 2 {
		t.Fatalf("inputs = %d; want 2", len(rec.inputs))
	}
	if rec.inputs[0].Track.ArtistName != "Artist A" || rec.inputs[1].Track.TrackName != "Title B" {
		t.Fatalf("inputs = %+v; want text-file tracks", rec.inputs)
	}
	for _, v := range rec.inputs {
		if v.Outdir != outdir {
			t.Fatalf("input outdir = %q; want %q", v.Outdir, outdir)
		}
	}
}
