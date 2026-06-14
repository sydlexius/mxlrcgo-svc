package secrets

import (
	"context"
	"errors"
	"sync"
)

// MemoryStore is an in-memory Store for tests and single-process use, mirroring
// auth.MemoryStore. Values are held in plaintext in a map (there is no at-rest
// surface to protect in memory), so it exercises the same Store contract as
// SQLStore without a key or database.
type MemoryStore struct {
	mu      sync.RWMutex
	secrets map[string]string
}

// NewMemoryStore returns an empty in-memory secret store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{secrets: map[string]string{}}
}

// Set stores plaintext under name, overwriting any existing value.
func (s *MemoryStore) Set(ctx context.Context, name, plaintext string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == "" {
		return errors.New("secrets: name must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[name] = plaintext
	return nil
}

// Get returns the plaintext for name; ok is false when absent.
func (s *MemoryStore) Get(ctx context.Context, name string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.secrets[name]
	return v, ok, nil
}

// Delete removes name; deleting an absent name is a no-op.
func (s *MemoryStore) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.secrets, name)
	return nil
}
