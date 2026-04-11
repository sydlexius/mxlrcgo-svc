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

	fn := Slugify("Test Artist - Lyric Track") + ".lrc"
	fp := filepath.Join(tmpDir, fn)
	if _, err := os.Stat(fp); err != nil {
		t.Fatalf("expected file %s to exist: %v", fp, err)
	}
}
