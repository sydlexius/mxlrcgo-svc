package verification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sydlexius/mxlrcgo-svc/internal/models"
)

func TestSongTextPrefersSubtitles(t *testing.T) {
	song := models.Song{
		Lyrics: models.Lyrics{LyricsBody: "unsynced"},
		Subtitles: models.Synced{Lines: []models.Lines{
			{Text: "line one"},
			{Text: "line two"},
		}},
	}
	if got := SongText(song); got != "line one line two" {
		t.Fatalf("SongText = %q; want subtitle text", got)
	}
}

func TestSongTextFallsBackWhenSubtitleLinesBlank(t *testing.T) {
	song := models.Song{
		Lyrics: models.Lyrics{LyricsBody: "unsynced"},
		Subtitles: models.Synced{Lines: []models.Lines{
			{Text: ""},
			{Text: "   "},
		}},
	}
	if got := SongText(song); got != "unsynced" {
		t.Fatalf("SongText = %q; want lyrics fallback", got)
	}
}

func TestSimilarityUsesTranscriptTokenCoverage(t *testing.T) {
	got := Similarity("hello bright world", "hello world this is the song")
	if got < 0.66 {
		t.Fatalf("Similarity = %v; want transcript token overlap", got)
	}
}

func TestHTTPVerifierVerifyPostsAudioAndComparesTranscript(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "song.flac")
	if err := os.WriteFile(audioPath, []byte("fake audio"), 0600); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("path = %q; want transcription endpoint", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		if got := r.FormValue("sample_duration_seconds"); got != "" {
			t.Fatalf("sample_duration_seconds = %q; want no nonstandard field", got)
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		_ = f.Close()
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "hello world"})
	}))
	defer srv.Close()

	v, err := NewHTTPVerifier(srv.URL, 45, 0.5)
	if err != nil {
		t.Fatalf("NewHTTPVerifier: %v", err)
	}
	if v.sampleDurationSeconds != 45 {
		t.Fatalf("sampleDurationSeconds = %d; want 45", v.sampleDurationSeconds)
	}
	res, err := v.Verify(context.Background(), audioPath, models.Song{
		Lyrics: models.Lyrics{LyricsBody: "hello world lyrics"},
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Accepted {
		t.Fatalf("accepted = false; want true")
	}
}

func TestHTTPVerifierTranscriptionURL(t *testing.T) {
	tests := map[string]string{
		"http://whisper:9000":                         "http://whisper:9000/v1/audio/transcriptions",
		"http://whisper:9000/":                        "http://whisper:9000/v1/audio/transcriptions",
		"http://whisper:9000/v1":                      "http://whisper:9000/v1/audio/transcriptions",
		"http://whisper:9000/v1/":                     "http://whisper:9000/v1/audio/transcriptions",
		"http://whisper:9000/v1/audio/transcriptions": "http://whisper:9000/v1/audio/transcriptions",
	}
	for rawURL, want := range tests {
		v, err := NewHTTPVerifier(rawURL, 30, 0.5)
		if err != nil {
			t.Fatalf("NewHTTPVerifier(%q): %v", rawURL, err)
		}
		if got := v.transcriptionURL(); got != want {
			t.Fatalf("transcriptionURL(%q) = %q; want %q", rawURL, got, want)
		}
	}
}
