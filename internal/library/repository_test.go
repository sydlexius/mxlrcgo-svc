package library_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/sydlexius/mxlrcsvc-go/internal/db"
	"github.com/sydlexius/mxlrcsvc-go/internal/library"
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

func TestAddGetListUpdateRemove(t *testing.T) {
	ctx := context.Background()
	repo := library.New(openTestDB(t))

	added, err := repo.Add(ctx, "/music/rock", "Rock")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if added.ID == 0 {
		t.Fatal("Add returned zero ID")
	}
	if added.Path != "/music/rock" || added.Name != "Rock" {
		t.Fatalf("Add returned %+v", added)
	}

	got, err := repo.Get(ctx, added.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != added {
		t.Fatalf("Get got %+v; want %+v", got, added)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0] != added {
		t.Fatalf("List got %+v; want [%+v]", list, added)
	}

	updated, err := repo.Update(ctx, added.ID, "/music/jazz", "Jazz")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ID != added.ID || updated.Path != "/music/jazz" || updated.Name != "Jazz" {
		t.Fatalf("Update returned %+v", updated)
	}

	if err := repo.Remove(ctx, added.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, err = repo.Get(ctx, added.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Get after Remove got %v; want sql.ErrNoRows", err)
	}
}

func TestAdd_RejectsDuplicatePath(t *testing.T) {
	ctx := context.Background()
	repo := library.New(openTestDB(t))

	if _, err := repo.Add(ctx, "/music", "Music"); err != nil {
		t.Fatalf("Add initial: %v", err)
	}
	if _, err := repo.Add(ctx, "/music", "Duplicate"); err == nil {
		t.Fatal("Add duplicate returned nil error; want constraint error")
	}
}

func TestAdd_ValidatesRequiredFields(t *testing.T) {
	ctx := context.Background()
	repo := library.New(openTestDB(t))

	tests := []struct {
		name string
		path string
		lib  string
	}{
		{name: "empty path", path: "", lib: "Music"},
		{name: "empty name", path: "/music", lib: ""},
		{name: "blank path", path: "  ", lib: "Music"},
		{name: "blank name", path: "/music", lib: "  "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := repo.Add(ctx, tc.path, tc.lib); err == nil {
				t.Fatal("Add returned nil error; want validation error")
			}
		})
	}
}

func TestUpdateRemove_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := library.New(openTestDB(t))

	_, err := repo.Update(ctx, 123, "/music", "Music")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Update missing got %v; want sql.ErrNoRows", err)
	}
	err = repo.Remove(ctx, 123)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Remove missing got %v; want sql.ErrNoRows", err)
	}
}
