package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sydlexius/mxlrcgo-svc/internal/lyrics"
	"github.com/sydlexius/mxlrcgo-svc/internal/models"
	"github.com/sydlexius/mxlrcgo-svc/internal/queue"
)

type fakeFetcher struct {
	song  models.Song
	err   error
	calls []models.Track
}

func (f *fakeFetcher) FindLyrics(ctx context.Context, track models.Track) (models.Song, error) {
	if err := ctx.Err(); err != nil {
		return models.Song{}, err
	}
	f.calls = append(f.calls, track)
	if f.err != nil {
		return models.Song{}, f.err
	}
	return f.song, nil
}

func TestRunFetchesAndWritesSyncedLyrics(t *testing.T) {
	t.Parallel()

	outdir := t.TempDir()
	track := models.Track{
		ArtistName: "Test Artist",
		TrackName:  "Test Song",
	}
	fetcher := &fakeFetcher{
		song: models.Song{
			Track: models.Track{
				ArtistName:  "Test Artist",
				TrackName:   "Test Song",
				AlbumName:   "Test Album",
				TrackLength: 123,
			},
			Subtitles: models.Synced{
				Lines: []models.Lines{
					{
						Text: "first line",
						Time: models.Time{Minutes: 0, Seconds: 1, Hundredths: 23},
					},
					{
						Text: "second line",
						Time: models.Time{Minutes: 0, Seconds: 2, Hundredths: 34},
					},
				},
			},
		},
	}
	inputs := queue.NewInputsQueue()
	inputs.Push(models.Inputs{
		Track:    track,
		Outdir:   outdir,
		Filename: "test-song.lrc",
	})

	a := NewApp(fetcher, lyrics.NewLRCWriter(), inputs, 0, "multi")
	if err := a.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(fetcher.calls) != 1 {
		t.Fatalf("FindLyrics() calls = %d, want 1", len(fetcher.calls))
	}
	if fetcher.calls[0] != track {
		t.Fatalf("FindLyrics() track = %#v, want %#v", fetcher.calls[0], track)
	}

	got, err := os.ReadFile(filepath.Join(outdir, "test-song.lrc"))
	if err != nil {
		t.Fatalf("reading LRC output: %v", err)
	}
	want := strings.Join([]string{
		"[by:mxlrcgo-svc]",
		"[ar:Test Artist]",
		"[ti:Test Song]",
		"[al:Test Album]",
		"[length:02:03]",
		"[00:01.23]first line",
		"[00:02.34]second line",
		"",
	}, "\n")
	if string(got) != want {
		t.Fatalf("LRC output = %q, want %q", got, want)
	}
}

func TestRunWritesFailedFileOnFetchFailure(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	track := models.Track{
		ArtistName: "Failure Artist",
		TrackName:  "Failure Song",
	}
	inputs := queue.NewInputsQueue()
	inputs.Push(models.Inputs{Track: track, Outdir: filepath.Join(tmp, "lyrics")})
	fetcher := &fakeFetcher{err: errors.New("fake fetch failed")}

	a := NewApp(fetcher, lyrics.NewLRCWriter(), inputs, 0, "multi")
	if err := a.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(tmp, "*_failed.txt"))
	if err != nil {
		t.Fatalf("globbing failed file: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("failed file count = %d, want 1: %v", len(matches), matches)
	}

	got, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("reading failed file: %v", err)
	}
	if string(got) != "Failure Artist,Failure Song\n" {
		t.Fatalf("failed file content = %q", got)
	}
}
