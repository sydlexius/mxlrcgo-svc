package worker

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/sydlexius/mxlrcgo-svc/internal/models"
	"github.com/sydlexius/mxlrcgo-svc/internal/queue"
	"github.com/sydlexius/mxlrcgo-svc/internal/verification"
)

type fakeQueue struct {
	items       []queue.WorkItem
	completed   []int64
	failed      []int64
	failCauses  []error
	completeErr error
	failErr     error
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
	if q.completeErr != nil {
		return q.completeErr
	}
	q.completed = append(q.completed, id)
	return nil
}

func (q *fakeQueue) Fail(_ context.Context, id int64, cause error) (queue.WorkItem, error) {
	if q.failErr != nil {
		return queue.WorkItem{}, q.failErr
	}
	q.failed = append(q.failed, id)
	q.failCauses = append(q.failCauses, cause)
	return queue.WorkItem{ID: id, Status: queue.StatusFailed}, nil
}

type cacheStore struct {
	artist string
	title  string
	album  string
	lyrics string
}

type fakeCache struct {
	exact    string
	fallback string
	err      error
	stores   []cacheStore
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

func (c *fakeCache) Store(_ context.Context, artist, title, album, lyrics string) error {
	c.stores = append(c.stores, cacheStore{artist: artist, title: title, album: album, lyrics: lyrics})
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

type fakeVerifier struct {
	results []verificationResult
	calls   []verifierCall
}

type verifierCall struct {
	path string
	song models.Song
}

type verificationResult struct {
	accepted bool
	err      error
}

func (v *fakeVerifier) Verify(_ context.Context, path string, song models.Song) (verification.Result, error) {
	res := verificationResult{accepted: true}
	if len(v.calls) < len(v.results) {
		res = v.results[len(v.calls)]
	}
	v.calls = append(v.calls, verifierCall{path: path, song: song})
	if res.err != nil {
		return verification.Result{}, res.err
	}
	return verification.Result{Accepted: res.accepted, Similarity: 1}, nil
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
	if cache.stores[0].artist != "Artist" || cache.stores[0].title != "Title" {
		t.Fatalf("cache store key = %+v; want Artist/Title", cache.stores[0])
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

func TestRunOnceVerifiesLowConfidenceScannedFetch(t *testing.T) {
	track := models.Track{ArtistName: "Requested Artist", TrackName: "Requested Title"}
	fetched := models.Track{ArtistName: "Different Artist", TrackName: "Different Title"}
	q := &fakeQueue{items: []queue.WorkItem{{
		ID: 20,
		Inputs: models.Inputs{
			Track:      track,
			Outdir:     "out",
			Filename:   "requested-title.lrc",
			SourcePath: "/music/requested-title.flac",
		},
	}}}
	fetcher := &fakeFetcher{song: models.Song{
		Track:  fetched,
		Lyrics: models.Lyrics{LyricsBody: "fresh lyrics"},
	}}
	verifier := &fakeVerifier{}
	w := New(q, &fakeCache{}, fetcher, &fakeWriter{})
	w.EnableVerification(verifier, 0.85)

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(verifier.calls) != 1 {
		t.Fatalf("verifier calls = %d; want 1", len(verifier.calls))
	}
	if verifier.calls[0].path != "/music/requested-title.flac" {
		t.Fatalf("verifier path = %q; want source path", verifier.calls[0].path)
	}
	if verifier.calls[0].song.Track.ArtistName != fetched.ArtistName || verifier.calls[0].song.Track.TrackName != fetched.TrackName {
		t.Fatalf("verifier song track = %+v; want fetched track %+v", verifier.calls[0].song.Track, fetched)
	}
	if len(q.completed) != 1 || q.completed[0] != 20 {
		t.Fatalf("completed = %v; want [20]", q.completed)
	}
}

func TestRunOnceSkipsVerificationForHighConfidenceMatch(t *testing.T) {
	track := models.Track{ArtistName: "Artist", TrackName: "Title"}
	q := &fakeQueue{items: []queue.WorkItem{{
		ID: 21,
		Inputs: models.Inputs{
			Track:      track,
			Outdir:     "out",
			Filename:   "artist-title.lrc",
			SourcePath: "/music/artist-title.flac",
		},
	}}}
	fetcher := &fakeFetcher{song: models.Song{
		Track:  track,
		Lyrics: models.Lyrics{LyricsBody: "fresh lyrics"},
	}}
	verifier := &fakeVerifier{}
	w := New(q, &fakeCache{}, fetcher, &fakeWriter{})
	w.EnableVerification(verifier, 0.85)

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(verifier.calls) != 0 {
		t.Fatalf("verifier calls = %d; want 0", len(verifier.calls))
	}
}

func TestRunOnceRejectedVerificationMarksQueueFailed(t *testing.T) {
	track := models.Track{ArtistName: "Requested Artist", TrackName: "Requested Title"}
	fetched := models.Track{ArtistName: "Different Artist", TrackName: "Different Title"}
	q := &fakeQueue{items: []queue.WorkItem{{
		ID: 22,
		Inputs: models.Inputs{
			Track:      track,
			Outdir:     "out",
			Filename:   "requested-title.lrc",
			SourcePath: "/music/requested-title.flac",
		},
	}}}
	fetcher := &fakeFetcher{song: models.Song{
		Track:  fetched,
		Lyrics: models.Lyrics{LyricsBody: "fresh lyrics"},
	}}
	verifier := &fakeVerifier{results: []verificationResult{{accepted: false}}}
	cache := &fakeCache{}
	w := New(q, cache, fetcher, &fakeWriter{})
	w.EnableVerification(verifier, 0.85)

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(verifier.calls) != 1 {
		t.Fatalf("verifier calls = %d; want 1", len(verifier.calls))
	}
	if verifier.calls[0].path != "/music/requested-title.flac" {
		t.Fatalf("verifier path = %q; want source path", verifier.calls[0].path)
	}
	if len(q.failed) != 1 || q.failed[0] != 22 {
		t.Fatalf("failed = %v; want [22]", q.failed)
	}
	if len(cache.stores) != 0 {
		t.Fatalf("cache stores = %d; want none for rejected verification", len(cache.stores))
	}
	if len(q.completed) != 0 {
		t.Fatalf("completed = %v; want none", q.completed)
	}
}

func TestRunOnceStoresCacheWithRequestedTrackKeys(t *testing.T) {
	track := models.Track{ArtistName: "Requested Artist", TrackName: "Requested Title", AlbumName: "Requested Album"}
	fetched := models.Track{ArtistName: "Canonical Artist", TrackName: "Canonical Title", AlbumName: "Canonical Album"}
	q := &fakeQueue{items: []queue.WorkItem{{
		ID: 6,
		Inputs: models.Inputs{
			Track:    track,
			Outdir:   "out",
			Filename: "requested-title.lrc",
		},
	}}}
	cache := &fakeCache{}
	fetcher := &fakeFetcher{song: models.Song{
		Track:  fetched,
		Lyrics: models.Lyrics{LyricsBody: "fresh lyrics"},
	}}

	w := New(q, cache, fetcher, &fakeWriter{})
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(cache.stores) != 1 {
		t.Fatalf("cache stores = %d; want 1", len(cache.stores))
	}
	store := cache.stores[0]
	if store.artist != track.ArtistName || store.title != track.TrackName || store.album != track.AlbumName {
		t.Fatalf("cache store key = %+v; want requested track %+v", store, track)
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

func TestRunOnceCompleteFailureMarksQueueFailed(t *testing.T) {
	completeErr := errors.New("complete failed")
	track := models.Track{ArtistName: "Artist", TrackName: "Title"}
	q := &fakeQueue{
		items: []queue.WorkItem{{
			ID: 7,
			Inputs: models.Inputs{
				Track:    track,
				Outdir:   "out",
				Filename: "artist-title.lrc",
			},
		}},
		completeErr: completeErr,
	}
	fetcher := &fakeFetcher{song: models.Song{
		Track:  track,
		Lyrics: models.Lyrics{LyricsBody: "fresh lyrics"},
	}}
	w := New(q, &fakeCache{}, fetcher, &fakeWriter{})

	err := w.RunOnce(context.Background())
	if !errors.Is(err, completeErr) {
		t.Fatalf("RunOnce error = %v; want complete failure", err)
	}
	if len(q.failed) != 1 || q.failed[0] != 7 {
		t.Fatalf("failed = %v; want [7]", q.failed)
	}
	if len(q.failCauses) != 1 || !errors.Is(q.failCauses[0], completeErr) {
		t.Fatalf("fail causes = %v; want complete failure", q.failCauses)
	}
	if len(q.completed) != 0 {
		t.Fatalf("completed = %v; want none", q.completed)
	}
}

func TestRunReturnsNilWhenQueueEmpty(t *testing.T) {
	w := New(&fakeQueue{}, &fakeCache{}, &fakeFetcher{}, &fakeWriter{})

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v; want nil", err)
	}
}

func TestRunReturnsCompleteErrNoRows(t *testing.T) {
	track := models.Track{ArtistName: "Artist", TrackName: "Title"}
	q := &fakeQueue{
		items: []queue.WorkItem{{
			ID: 8,
			Inputs: models.Inputs{
				Track:    track,
				Outdir:   "out",
				Filename: "artist-title.lrc",
			},
		}},
		completeErr: sql.ErrNoRows,
	}
	fetcher := &fakeFetcher{song: models.Song{
		Track:  track,
		Lyrics: models.Lyrics{LyricsBody: "fresh lyrics"},
	}}
	w := New(q, &fakeCache{}, fetcher, &fakeWriter{})

	err := w.Run(context.Background())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Run error = %v; want sql.ErrNoRows", err)
	}
}

func TestRunProcessesOneReadyItem(t *testing.T) {
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

	if len(q.completed) != 1 || q.completed[0] != 4 {
		t.Fatalf("completed = %v; want [4]", q.completed)
	}
	if len(writer.writes) != 1 {
		t.Fatalf("writes = %d; want 1", len(writer.writes))
	}
	if writer.writes[0].Outdir != "out-a" || writer.writes[0].Filename != "a.lrc" {
		t.Fatalf("writes[0] = %+v; want out-a/a.lrc", writer.writes[0])
	}
	if len(q.items) != 1 || q.items[0].ID != 5 {
		t.Fatalf("remaining items = %+v; want item 5 still queued", q.items)
	}
}

func TestConfidence(t *testing.T) {
	want := models.Track{ArtistName: "  Héllo ", TrackName: "World"}
	got := models.Track{ArtistName: "hello", TrackName: " world "}

	if score := Confidence(want, got); score != 1 {
		t.Fatalf("Confidence() = %v; want 1", score)
	}
}
