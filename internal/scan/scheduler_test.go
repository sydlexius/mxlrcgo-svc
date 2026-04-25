package scan_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sydlexius/mxlrcsvc-go/internal/models"
	"github.com/sydlexius/mxlrcsvc-go/internal/scan"
	"github.com/sydlexius/mxlrcsvc-go/internal/scanner"
)

type fakeLibraries struct {
	libs []models.Library
	err  error
}

func (f fakeLibraries) List(context.Context) ([]models.Library, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.libs, nil
}

type fakeResults struct {
	calls     int
	libraryID int64
	results   []models.ScanResult
	err       error
}

func (f *fakeResults) Upsert(_ context.Context, libraryID int64, results []models.ScanResult) error {
	f.calls++
	f.libraryID = libraryID
	f.results = results
	if f.err != nil {
		return f.err
	}
	return nil
}

type fakeScanner struct {
	results []models.ScanResult
	err     error
}

func (f fakeScanner) ScanLibrary(string, scanner.ScanOptions) ([]models.ScanResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func TestScheduler_RunOncePersistsAndCallsCallback(t *testing.T) {
	ctx := context.Background()
	store := &fakeResults{}
	called := false
	s := scan.Scheduler{
		Libraries: fakeLibraries{libs: []models.Library{{ID: 7, Path: "/music", Name: "Music"}}},
		Results:   store,
		Scanner: fakeScanner{results: []models.ScanResult{{
			FilePath: "/music/a.mp3",
			Track:    models.Track{ArtistName: "Artist", TrackName: "Title"},
		}}},
		OnScanComplete: func(_ context.Context, lib models.Library, results []models.ScanResult) error {
			called = true
			if lib.ID != 7 {
				t.Errorf("callback lib ID = %d; want 7", lib.ID)
			}
			if len(results) != 1 || results[0].LibraryID != 7 {
				t.Errorf("callback results = %+v; want library ID 7", results)
			}
			return nil
		},
	}

	if err := s.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if store.calls != 1 {
		t.Fatalf("Upsert calls = %d; want 1", store.calls)
	}
	if store.libraryID != 7 {
		t.Errorf("Upsert libraryID = %d; want 7", store.libraryID)
	}
	if len(store.results) != 1 || store.results[0].Status != scan.StatusPending {
		t.Errorf("Upsert results = %+v; want one pending result", store.results)
	}
	if !called {
		t.Fatal("OnScanComplete was not called")
	}
}

func TestScheduler_RunWithNoIntervalRunsOnce(t *testing.T) {
	ctx := context.Background()
	store := &fakeResults{}
	s := scan.Scheduler{
		Libraries: fakeLibraries{libs: []models.Library{{ID: 7, Path: "/music", Name: "Music"}}},
		Results:   store,
		Scanner: fakeScanner{results: []models.ScanResult{{
			FilePath: "/music/a.mp3",
		}}},
	}

	if err := s.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if store.calls != 1 {
		t.Fatalf("Upsert calls = %d; want 1", store.calls)
	}
}

func TestScheduler_RunOnceRequiresDependencies(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		s    scan.Scheduler
	}{
		{name: "missing libraries", s: scan.Scheduler{}},
		{name: "missing results", s: scan.Scheduler{Libraries: fakeLibraries{}}},
		{name: "missing scanner", s: scan.Scheduler{Libraries: fakeLibraries{}, Results: &fakeResults{}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.s.RunOnce(ctx); err == nil {
				t.Fatal("RunOnce returned nil error; want dependency error")
			}
		})
	}
}

func TestScheduler_RunOncePropagatesErrors(t *testing.T) {
	ctx := context.Background()
	listErr := errors.New("list failed")
	scanErr := errors.New("scan failed")
	storeErr := errors.New("store failed")
	callbackErr := errors.New("callback failed")

	tests := []struct {
		name string
		s    scan.Scheduler
		want error
	}{
		{
			name: "list",
			s: scan.Scheduler{
				Libraries: fakeLibraries{err: listErr},
				Results:   &fakeResults{},
				Scanner:   fakeScanner{},
			},
			want: listErr,
		},
		{
			name: "scan",
			s: scan.Scheduler{
				Libraries: fakeLibraries{libs: []models.Library{{ID: 1, Path: "/music"}}},
				Results:   &fakeResults{},
				Scanner:   fakeScanner{err: scanErr},
			},
			want: scanErr,
		},
		{
			name: "store",
			s: scan.Scheduler{
				Libraries: fakeLibraries{libs: []models.Library{{ID: 1, Path: "/music"}}},
				Results:   &fakeResults{err: storeErr},
				Scanner: fakeScanner{results: []models.ScanResult{{
					FilePath: "/music/a.mp3",
				}}},
			},
			want: storeErr,
		},
		{
			name: "callback",
			s: scan.Scheduler{
				Libraries: fakeLibraries{libs: []models.Library{{ID: 1, Path: "/music"}}},
				Results:   &fakeResults{},
				Scanner: fakeScanner{results: []models.ScanResult{{
					FilePath: "/music/a.mp3",
				}}},
				OnScanComplete: func(context.Context, models.Library, []models.ScanResult) error {
					return callbackErr
				},
			},
			want: callbackErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.s.RunOnce(ctx)
			if !errors.Is(err, tc.want) {
				t.Fatalf("RunOnce error = %v; want wrapping %v", err, tc.want)
			}
		})
	}
}
