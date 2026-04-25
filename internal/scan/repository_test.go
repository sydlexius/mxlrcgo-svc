package scan_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/sydlexius/mxlrcsvc-go/internal/db"
	"github.com/sydlexius/mxlrcsvc-go/internal/library"
	"github.com/sydlexius/mxlrcsvc-go/internal/models"
	"github.com/sydlexius/mxlrcsvc-go/internal/scan"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	sqlDB, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

func TestRepo_UpsertAndListByLibrary(t *testing.T) {
	ctx := context.Background()
	sqlDB := openTestDB(t)
	libRepo := library.New(sqlDB)
	scanRepo := scan.New(sqlDB)

	lib, err := libRepo.Add(ctx, "/music", "Music")
	if err != nil {
		t.Fatalf("Add library: %v", err)
	}

	results := []models.ScanResult{{
		FilePath: "/music/a.mp3",
		Track:    models.Track{ArtistName: "Artist", TrackName: "Title"},
		Status:   scan.StatusPending,
	}}
	if err := scanRepo.Upsert(ctx, lib.ID, results); err != nil {
		t.Fatalf("Upsert initial: %v", err)
	}
	results[0].Track.TrackName = "Updated Title"
	results[0].Status = scan.StatusDone
	if err := scanRepo.Upsert(ctx, lib.ID, results); err != nil {
		t.Fatalf("Upsert update: %v", err)
	}

	got, err := scanRepo.ListByLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatalf("ListByLibrary: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListByLibrary returned %d results; want 1", len(got))
	}
	if got[0].FilePath != "/music/a.mp3" {
		t.Errorf("FilePath = %q; want /music/a.mp3", got[0].FilePath)
	}
	if got[0].Track.TrackName != "Updated Title" {
		t.Errorf("TrackName = %q; want Updated Title", got[0].Track.TrackName)
	}
	if got[0].Status != scan.StatusDone {
		t.Errorf("Status = %q; want %q", got[0].Status, scan.StatusDone)
	}
}

func TestRepo_UpsertDefaultsStatus(t *testing.T) {
	ctx := context.Background()
	sqlDB := openTestDB(t)
	libRepo := library.New(sqlDB)
	scanRepo := scan.New(sqlDB)

	lib, err := libRepo.Add(ctx, "/music", "Music")
	if err != nil {
		t.Fatalf("Add library: %v", err)
	}
	if err := scanRepo.Upsert(ctx, lib.ID, []models.ScanResult{{
		FilePath: "/music/a.mp3",
		Track:    models.Track{ArtistName: "Artist", TrackName: "Title"},
	}}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := scanRepo.ListByLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatalf("ListByLibrary: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListByLibrary returned %d results; want 1", len(got))
	}
	if got[0].Status != scan.StatusPending {
		t.Fatalf("Status = %q; want %q", got[0].Status, scan.StatusPending)
	}
}

func TestRepo_ListByLibrary_IsolatedByLibrary(t *testing.T) {
	ctx := context.Background()
	sqlDB := openTestDB(t)
	libRepo := library.New(sqlDB)
	scanRepo := scan.New(sqlDB)

	libA, err := libRepo.Add(ctx, "/music/a", "A")
	if err != nil {
		t.Fatalf("Add library A: %v", err)
	}
	libB, err := libRepo.Add(ctx, "/music/b", "B")
	if err != nil {
		t.Fatalf("Add library B: %v", err)
	}
	filePath := "/shared/track.mp3"
	if err := scanRepo.Upsert(ctx, libA.ID, []models.ScanResult{{
		FilePath: filePath,
		Track:    models.Track{ArtistName: "Artist A", TrackName: "Title A"},
	}}); err != nil {
		t.Fatalf("Upsert library A: %v", err)
	}
	if err := scanRepo.Upsert(ctx, libB.ID, []models.ScanResult{{
		FilePath: filePath,
		Track:    models.Track{ArtistName: "Artist B", TrackName: "Title B"},
	}}); err != nil {
		t.Fatalf("Upsert library B: %v", err)
	}

	gotA, err := scanRepo.ListByLibrary(ctx, libA.ID)
	if err != nil {
		t.Fatalf("ListByLibrary A: %v", err)
	}
	gotB, err := scanRepo.ListByLibrary(ctx, libB.ID)
	if err != nil {
		t.Fatalf("ListByLibrary B: %v", err)
	}
	if len(gotA) != 1 || gotA[0].Track.ArtistName != "Artist A" {
		t.Fatalf("library A results = %+v; want Artist A only", gotA)
	}
	if len(gotB) != 1 || gotB[0].Track.ArtistName != "Artist B" {
		t.Fatalf("library B results = %+v; want Artist B only", gotB)
	}
}
