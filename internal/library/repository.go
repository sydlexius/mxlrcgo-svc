package library

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sydlexius/mxlrcgo-svc/internal/models"
)

// Repo provides CRUD access to configured library roots.
type Repo struct {
	db *sql.DB
}

// New creates a library repository backed by db.
func New(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// Add creates a new library root. Path must be unique.
func (r *Repo) Add(ctx context.Context, path, name string) (models.Library, error) {
	path, name, err := validate(path, name)
	if err != nil {
		return models.Library{}, err
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO libraries (path, name) VALUES (?, ?)`,
		path,
		name,
	)
	if err != nil {
		return models.Library{}, fmt.Errorf("library: add: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return models.Library{}, fmt.Errorf("library: add id: %w", err)
	}
	return r.Get(ctx, id)
}

// List returns all configured library roots in stable ID order.
func (r *Repo) List(ctx context.Context) (libs []models.Library, retErr error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, path, name, created_at, updated_at FROM libraries ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("library: list: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("library: close list rows: %w", err)
		}
	}()

	for rows.Next() {
		var lib models.Library
		if err := rows.Scan(&lib.ID, &lib.Path, &lib.Name, &lib.CreatedAt, &lib.UpdatedAt); err != nil {
			return nil, fmt.Errorf("library: list scan: %w", err)
		}
		libs = append(libs, lib)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("library: list rows: %w", err)
	}
	return libs, nil
}

// Get returns the library root with id. It returns sql.ErrNoRows when not found.
func (r *Repo) Get(ctx context.Context, id int64) (models.Library, error) {
	var lib models.Library
	err := r.db.QueryRowContext(ctx,
		`SELECT id, path, name, created_at, updated_at FROM libraries WHERE id = ?`,
		id,
	).Scan(&lib.ID, &lib.Path, &lib.Name, &lib.CreatedAt, &lib.UpdatedAt)
	if err != nil {
		return models.Library{}, fmt.Errorf("library: get: %w", err)
	}
	return lib, nil
}

// GetByName returns the library root whose name matches name. It returns
// sql.ErrNoRows when not found.
func (r *Repo) GetByName(ctx context.Context, name string) (models.Library, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return models.Library{}, fmt.Errorf("library: name must not be empty")
	}
	var lib models.Library
	err := r.db.QueryRowContext(ctx,
		`SELECT id, path, name, created_at, updated_at FROM libraries WHERE name = ?`,
		name,
	).Scan(&lib.ID, &lib.Path, &lib.Name, &lib.CreatedAt, &lib.UpdatedAt)
	if err != nil {
		return models.Library{}, fmt.Errorf("library: get by name: %w", err)
	}
	return lib, nil
}

// Update changes the path and name for an existing library root.
func (r *Repo) Update(ctx context.Context, id int64, path, name string) (models.Library, error) {
	path, name, err := validate(path, name)
	if err != nil {
		return models.Library{}, err
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE libraries SET path = ?, name = ? WHERE id = ?`,
		path,
		name,
		id,
	)
	if err != nil {
		return models.Library{}, fmt.Errorf("library: update: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return models.Library{}, fmt.Errorf("library: update rows affected: %w", err)
	}
	if affected == 0 {
		return models.Library{}, fmt.Errorf("library: update missing: %w", sql.ErrNoRows)
	}
	return r.Get(ctx, id)
}

// Remove deletes the library root with id.
func (r *Repo) Remove(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM libraries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("library: remove: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("library: remove rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("library: remove missing: %w", sql.ErrNoRows)
	}
	return nil
}

func validate(path, name string) (string, string, error) {
	path = strings.TrimSpace(path)
	name = strings.TrimSpace(name)
	if path == "" {
		return "", "", fmt.Errorf("library: path must not be empty")
	}
	if name == "" {
		return "", "", fmt.Errorf("library: name must not be empty")
	}
	return path, name, nil
}
