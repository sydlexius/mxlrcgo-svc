package lyrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sydlexius/mxlrcsvc-go/internal/models"
)

func TestWriteLRC_NothingToSave(t *testing.T) {
	w := NewLRCWriter()
	tmpDir := t.TempDir()

	song := models.Song{
		Track: models.Track{
			ArtistName:   "Test Artist",
			TrackName:    "Test Track",
			Instrumental: 0,
		},
		Lyrics:    models.Lyrics{LyricsBody: ""},
		Subtitles: models.Synced{Lines: nil},
	}

	err := w.WriteLRC(song, "", tmpDir)
	if err == nil {
		t.Fatal("expected error 'nothing to save', got nil")
	}

	// No file should have been created on disk
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("reading tmpDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files in tmpDir, found %d: %v", len(entries), entries)
	}
}

func TestWriteLRC_Instrumental(t *testing.T) {
	w := NewLRCWriter()
	tmpDir := t.TempDir()

	song := models.Song{
		Track: models.Track{
			ArtistName:   "Test Artist",
			TrackName:    "Instrumental Track",
			Instrumental: 1,
		},
		Lyrics:    models.Lyrics{LyricsBody: ""},
		Subtitles: models.Synced{Lines: nil},
	}

	err := w.WriteLRC(song, "", tmpDir)
	if err != nil {
		t.Fatalf("expected nil error for instrumental, got: %v", err)
	}

	fn := Slugify("Test Artist - Instrumental Track") + ".lrc"
	fp := filepath.Join(tmpDir, fn)
	data, err := os.ReadFile(fp) //nolint:gosec // test path constructed from known test data
	if err != nil {
		t.Fatalf("expected file %s to exist: %v", fp, err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Fatal("expected non-empty file content for instrumental")
	}
	const want = "[00:00.00]\u266a Instrumental \u266a"
	if !strings.Contains(content, want) {
		t.Fatalf("expected content to contain %q, got: %q", want, content)
	}
}

func TestWriteLRC_Unsynced(t *testing.T) {
	w := NewLRCWriter()
	tmpDir := t.TempDir()

	song := models.Song{
		Track: models.Track{
			ArtistName: "Test Artist",
			TrackName:  "Lyric Track",
		},
		Lyrics:    models.Lyrics{LyricsBody: "Hello world\nGoodbye world"},
		Subtitles: models.Synced{Lines: nil},
	}

	err := w.WriteLRC(song, "", tmpDir)
	if err != nil {
		t.Fatalf("expected nil error for unsynced, got: %v", err)
	}

	// Unsynced lyrics must be saved as .txt, not .lrc.
	fn := Slugify("Test Artist - Lyric Track") + ".txt"
	fp := filepath.Join(tmpDir, fn)
	data, err := os.ReadFile(fp) //nolint:gosec // test path constructed from known test data
	if err != nil {
		t.Fatalf("expected file %s to exist: %v", fp, err)
	}

	// File must contain plain text -- no LRC timestamp prefix.
	content := string(data)
	if strings.Contains(content, "[00:00.00]") {
		t.Fatalf("unsynced .txt must not contain LRC timestamps, got: %q", content)
	}
	if !strings.Contains(content, "Hello world") {
		t.Fatalf("expected plain lyrics in .txt file, got: %q", content)
	}

	// No .lrc file should have been created.
	lrcFn := Slugify("Test Artist - Lyric Track") + ".lrc"
	if _, err := os.Stat(filepath.Join(tmpDir, lrcFn)); err == nil {
		t.Fatal("expected no .lrc file for unsynced lyrics, but one was created")
	}
}

func TestWriteLRC_UnsyncedExplicitFilename(t *testing.T) {
	w := NewLRCWriter()
	tmpDir := t.TempDir()

	song := models.Song{
		Track: models.Track{
			ArtistName: "Test Artist",
			TrackName:  "Lyric Track",
		},
		Lyrics:    models.Lyrics{LyricsBody: "Verse one"},
		Subtitles: models.Synced{Lines: nil},
	}

	// Simulates dir-mode where the scanner passes an explicit .lrc filename.
	// The writer should swap to .txt since content is unsynced.
	err := w.WriteLRC(song, "song.lrc", tmpDir)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	fp := filepath.Join(tmpDir, "song.txt")
	if _, err := os.Stat(fp); err != nil {
		t.Fatalf("expected file %s to exist: %v", fp, err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "song.lrc")); err == nil {
		t.Fatal("expected no .lrc file when content is unsynced")
	}
}

func TestWriteLRC_Synced(t *testing.T) {
	w := NewLRCWriter()
	tmpDir := t.TempDir()

	song := models.Song{
		Track: models.Track{
			ArtistName: "Test Artist",
			TrackName:  "Synced Track",
		},
		Lyrics: models.Lyrics{},
		Subtitles: models.Synced{Lines: []models.Lines{
			{Text: "Line one", Time: models.Time{Minutes: 0, Seconds: 5, Hundredths: 10}},
		}},
	}

	err := w.WriteLRC(song, "", tmpDir)
	if err != nil {
		t.Fatalf("expected nil error for synced, got: %v", err)
	}

	fn := Slugify("Test Artist - Synced Track") + ".lrc"
	fp := filepath.Join(tmpDir, fn)
	data, err := os.ReadFile(fp) //nolint:gosec // test path constructed from known test data
	if err != nil {
		t.Fatalf("expected file %s to exist: %v", fp, err)
	}

	content := string(data)
	if !strings.Contains(content, "[00:05.10]Line one") {
		t.Fatalf("expected synced timestamp in .lrc, got: %q", content)
	}
	if !strings.Contains(content, "[ar:Test Artist]") {
		t.Fatalf("expected LRC tags in .lrc, got: %q", content)
	}
}
