package worker

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/sydlexius/mxlrcsvc-go/internal/models"
	"github.com/sydlexius/mxlrcsvc-go/internal/queue"
)

type fakeQueue struct {
	items      []queue.WorkItem
	completed  []int64
	failed     []int64
	failCauses []error
}

func (q *fakeQueue) Dequeue(_ context.Context) (queue.WorkItem, error) {
	if len(q.items) == 0 {
		return queue.WorkItem{}, sql.ErrNoRows
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, nil
}

func (q *fakeQueue) Complete(_ context.Context, id int64) error {
	q.completed = append(q.completed, id)
	return nil
}

func (q *fakeQueue) Fail(_ context.Context, id int64, cause error) (queue.WorkItem, error) {
	q.failed = append(q.failed, id)
	q.failCauses = append(q.failCauses, cause)
	return queue.WorkItem{ID: id, Status: queue.StatusFailed}, nil
}

type fakeCache struct {
	exact    string
	fallback string
	err      error
	stores   []string
}

func (c *fakeCache) LookupExact(context.Context, string, string, string) (string, error) {
	if c.err != nil {
		return "", c.err
	}
	if c.exact == "" {
		return "", sql.ErrNoRows
	}
	return c.exact, nil
}

func (c *fakeCache) LookupFallback(context.Context, string, string) (string, error) {
	if c.err != nil {
		return "", c.err
	}
	if c.fallback == "" {
		return "", sql.ErrNoRows
	}
	return c.fallback, nil
}

func (c *fakeCache) Store(_ context.Context, _, _, _, lyrics string) error {
	c.stores = append(c.stores, lyrics)
	return nil
}

type fakeFetcher struct {
	song  models.Song
	err   error
	calls int
}

func (f *fakeFetcher) FindLyrics(context.Context, models.Track) (models.Song, error) {
	f.calls++
	if f.err != nil {
		return models.Song{}, f.err
	}
	return f.song, nil
}

type fakeWriter struct {
	writes []models.OutputPath
	err    error
}

func (w *fakeWriter) WriteLRC(_ models.Song, filename string, outdir string) error {
	w.writes = append(w.writes, models.OutputPath{Outdir: outdir, Filename: filename})
	return w.err
}

func TestRunOnceCacheHitAvoidsFetcherAndCompletes(t *testing.T) {
	track := models.Track{ArtistName: "Artist", TrackName: "Title"}
	song := models.Song{
		Track:  track,
		Lyrics: models.Lyrics{LyricsBody: "cached lyrics"},
	}
	cached, err := encodeSong(song)
	if err != nil {
		t.Fatalf("encodeSong: %v", err)
	}
	q := &fakeQueue{items: []queue.WorkItem{{
		ID: 1,
		Inputs: models.Inputs{
			Track:    track,
			Outdir:   "out",
			Filename: "artist-title.lrc",
		},
	}}}
	cache := &fakeCache{exact: cached}
	fetcher := &fakeFetcher{}
	writer := &fakeWriter{}

	w := New(q, cache, fetcher, writer)
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if fetcher.calls != 0 {
		t.Fatalf("fetcher calls = %d; want 0", fetcher.calls)
	}
	if len(writer.writes) != 1 || writer.writes[0].Outdir != "out" || writer.writes[0].Filename != "artist-title.lrc" {
		t.Fatalf("writes = %+v; want one out/artist-title.lrc write", writer.writes)
	}
	if len(q.completed) != 1 || q.completed[0] != 1 {
		t.Fatalf("completed = %v; want [1]", q.completed)
	}
	if len(q.failed) != 0 {
		t.Fatalf("failed = %v; want none", q.failed)
	}
}

func TestRunOnceFetchesCachesWritesAllOutputsAndCompletes(t *testing.T) {
	track := models.Track{ArtistName: "Artist", TrackName: "Title"}
	q := &fakeQueue{items: []queue.WorkItem{{
		ID: 2,
		Inputs: models.Inputs{
			Track: track,
			OutputPaths: []models.OutputPath{
				{Outdir: "out-a", Filename: "a.lrc"},
				{Outdir: "out-b", Filename: "b.lrc"},
			},
		},
	}}}
	cache := &fakeCache{}
	fetcher := &fakeFetcher{song: models.Song{
		Track:  track,
		Lyrics: models.Lyrics{LyricsBody: "fresh lyrics"},
	}}
	writer := &fakeWriter{}

	w := New(q, cache, fetcher, writer)
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if fetcher.calls != 1 {
		t.Fatalf("fetcher calls = %d; want 1", fetcher.calls)
	}
	if len(cache.stores) != 1 {
		t.Fatalf("cache stores = %d; want 1", len(cache.stores))
	}
	if len(writer.writes) != 2 {
		t.Fatalf("writes = %d; want 2", len(writer.writes))
	}
	if writer.writes[0].Outdir != "out-a" || writer.writes[1].Outdir != "out-b" {
		t.Fatalf("writes = %+v; want both output paths", writer.writes)
	}
	if len(q.completed) != 1 || q.completed[0] != 2 {
		t.Fatalf("completed = %v; want [2]", q.completed)
	}
}

func TestRunOnceFailureMarksQueueFailed(t *testing.T) {
	wantErr := errors.New("fetch failed")
	q := &fakeQueue{items: []queue.WorkItem{{
		ID:     3,
		Inputs: models.Inputs{Track: models.Track{ArtistName: "Artist", TrackName: "Title"}},
	}}}
	w := New(q, &fakeCache{}, &fakeFetcher{err: wantErr}, &fakeWriter{})

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(q.failed) != 1 || q.failed[0] != 3 {
		t.Fatalf("failed = %v; want [3]", q.failed)
	}
	if !errors.Is(q.failCauses[0], wantErr) {
		t.Fatalf("fail cause = %v; want %v", q.failCauses[0], wantErr)
	}
	if len(q.completed) != 0 {
		t.Fatalf("completed = %v; want none", q.completed)
	}
}

func TestRunDrainsReadyItemsUntilQueueEmpty(t *testing.T) {
	track := models.Track{ArtistName: "Artist", TrackName: "Title"}
	q := &fakeQueue{items: []queue.WorkItem{
		{
			ID:     4,
			Inputs: models.Inputs{Track: track, Outdir: "out-a", Filename: "a.lrc"},
		},
		{
			ID:     5,
			Inputs: models.Inputs{Track: track, Outdir: "out-b", Filename: "b.lrc"},
		},
	}}
	cache := &fakeCache{}
	fetcher := &fakeFetcher{song: models.Song{
		Track:  track,
		Lyrics: models.Lyrics{LyricsBody: "fresh lyrics"},
	}}
	writer := &fakeWriter{}

	w := New(q, cache, fetcher, writer)
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(q.completed) != 2 || q.completed[0] != 4 || q.completed[1] != 5 {
		t.Fatalf("completed = %v; want [4 5]", q.completed)
	}
	if len(writer.writes) != 2 {
		t.Fatalf("writes = %d; want 2", len(writer.writes))
	}
	if writer.writes[0].Outdir != "out-a" || writer.writes[0].Filename != "a.lrc" {
		t.Fatalf("writes[0] = %+v; want out-a/a.lrc", writer.writes[0])
	}
	if writer.writes[1].Outdir != "out-b" || writer.writes[1].Filename != "b.lrc" {
		t.Fatalf("writes[1] = %+v; want out-b/b.lrc", writer.writes[1])
	}
}

func TestConfidence(t *testing.T) {
	want := models.Track{ArtistName: "  Héllo ", TrackName: "World"}
	got := models.Track{ArtistName: "hello", TrackName: " world "}

	if score := Confidence(want, got); score != 1 {
		t.Fatalf("Confidence() = %v; want 1", score)
	}
}
