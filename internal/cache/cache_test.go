package cache_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/sydlexius/mxlrcgo-svc/internal/cache"
	"github.com/sydlexius/mxlrcgo-svc/internal/db"
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

func TestStore_AndLookupExact(t *testing.T) {
	ctx := context.Background()
	repo := cache.New(openTestDB(t))

	// Store a row and retrieve it.
	if err := repo.Store(ctx, "Artist", "Title", "Album", "lyrics text"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := repo.LookupExact(ctx, "Artist", "Title", "Album")
	if err != nil {
		t.Fatalf("LookupExact: %v", err)
	}
	if got != "lyrics text" {
		t.Errorf("LookupExact got %q, want %q", got, "lyrics text")
	}
}

func TestStore_Upsert(t *testing.T) {
	ctx := context.Background()
	repo := cache.New(openTestDB(t))

	if err := repo.Store(ctx, "Artist", "Title", "Album", "original"); err != nil {
		t.Fatalf("Store initial: %v", err)
	}
	// Upsert same key with new lyrics.
	if err := repo.Store(ctx, "Artist", "Title", "Album", "updated"); err != nil {
		t.Fatalf("Store upsert: %v", err)
	}
	got, err := repo.LookupExact(ctx, "Artist", "Title", "Album")
	if err != nil {
		t.Fatalf("LookupExact after upsert: %v", err)
	}
	if got != "updated" {
		t.Errorf("after upsert: got %q, want %q", got, "updated")
	}
}

func TestLookupExact_WrongAlbum(t *testing.T) {
	ctx := context.Background()
	repo := cache.New(openTestDB(t))

	if err := repo.Store(ctx, "Artist", "Title", "Album", "lyrics"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	_, err := repo.LookupExact(ctx, "Artist", "Title", "WrongAlbum")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("LookupExact wrong album: got %v, want sql.ErrNoRows", err)
	}
}

func TestLookupFallback_IgnoresAlbum(t *testing.T) {
	ctx := context.Background()
	repo := cache.New(openTestDB(t))

	if err := repo.Store(ctx, "Artist", "Title", "SomeAlbum", "fallback lyrics"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := repo.LookupFallback(ctx, "Artist", "Title")
	if err != nil {
		t.Fatalf("LookupFallback: %v", err)
	}
	if got != "fallback lyrics" {
		t.Errorf("LookupFallback got %q, want %q", got, "fallback lyrics")
	}
}

func TestLookupFallback_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := cache.New(openTestDB(t))

	_, err := repo.LookupFallback(ctx, "Unknown", "Unknown")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("LookupFallback not found: got %v, want sql.ErrNoRows", err)
	}
}

func TestLookupExact_NormalizesKeys(t *testing.T) {
	ctx := context.Background()
	repo := cache.New(openTestDB(t))

	// Store with un-normalized casing/accents; lookup with different form.
	if err := repo.Store(ctx, "  Héllo  ", "  Wörld  ", "  Álbum  ", "normalized lyrics"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := repo.LookupExact(ctx, "hello", "world", "album")
	if err != nil {
		t.Fatalf("LookupExact normalized: %v", err)
	}
	if got != "normalized lyrics" {
		t.Errorf("LookupExact normalized: got %q, want %q", got, "normalized lyrics")
	}
}
