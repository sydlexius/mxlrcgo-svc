package scanner

import (
	"context"
	"path/filepath"
	"testing"
)

// recordedFail captures a RecordFailure call for assertions.
type recordedFail struct {
	path  string
	mtime int64
	size  int64
}

// fakeFailStore is an in-memory MetadataFailureStore for scanner tests.
type fakeFailStore struct {
	skip     map[string]bool
	recorded []recordedFail
}

func (f *fakeFailStore) ShouldSkip(_ context.Context, path string, _, _ int64) (bool, error) {
	return f.skip[path], nil
}

func (f *fakeFailStore) RecordFailure(_ context.Context, path string, mtime, size int64, _ error) error {
	f.recorded = append(f.recorded, recordedFail{path: path, mtime: mtime, size: size})
	return nil
}

// TestScanLibrary_RecordsMetadataFailure verifies that a file which fails
// metadata read is recorded in the failure store (so later scans can skip it).
func TestScanLibrary_RecordsMetadataFailure(t *testing.T) {
	dir := t.TempDir()
	touchFile(t, dir, "bad.mp3") // empty file -> tag.ReadFrom fails

	store := &fakeFailStore{skip: map[string]bool{}}
	sc := &Scanner{probeFunc: func(string) (int, error) { return 0, nil }, failures: store}

	if _, err := sc.ScanLibrary(context.Background(), dir, ScanOptions{MaxDepth: 0}); err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	if len(store.recorded) != 1 {
		t.Fatalf("want 1 recorded metadata failure, got %d", len(store.recorded))
	}
	want := filepath.Join(dir, "bad.mp3")
	if store.recorded[0].path != want {
		t.Errorf("recorded path = %q, want %q", store.recorded[0].path, want)
	}
	if store.recorded[0].size != 0 {
		t.Errorf("recorded size = %d, want 0 for the empty fixture", store.recorded[0].size)
	}
}

// TestScanLibrary_SkipsKnownBadFile verifies that a file the store reports as
// previously-failed is not re-read (and so not re-recorded or re-warned).
func TestScanLibrary_SkipsKnownBadFile(t *testing.T) {
	dir := t.TempDir()
	touchFile(t, dir, "bad.mp3")
	path := filepath.Join(dir, "bad.mp3")

	store := &fakeFailStore{skip: map[string]bool{path: true}}
	sc := &Scanner{probeFunc: func(string) (int, error) { return 0, nil }, failures: store}

	if _, err := sc.ScanLibrary(context.Background(), dir, ScanOptions{MaxDepth: 0}); err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	if len(store.recorded) != 0 {
		t.Errorf("a known-bad file must be skipped, not re-read; got %d new records", len(store.recorded))
	}
}
