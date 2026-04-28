package auth

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// NewWebhookService returns an auth service populated with configured webhook keys.
func NewWebhookService(ctx context.Context, rawKeys []string) (*Service, error) {
	if len(rawKeys) == 0 {
		return nil, fmt.Errorf("at least one webhook API key is required")
	}
	store := NewMemoryStore()
	now := time.Now().UTC()
	created := 0
	for i, raw := range rawKeys {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.HasPrefix(raw, KeyPrefix) {
			return nil, fmt.Errorf("webhook API key %d: invalid format", i+1)
		}
		hash, err := HashKey(raw)
		if err != nil {
			return nil, fmt.Errorf("hash webhook API key %d: %w", i+1, err)
		}
		if len(hash) < 16 {
			return nil, fmt.Errorf("webhook API key %d: derived hash is too short", i+1)
		}
		key := Key{
			ID:        hash[:16],
			Name:      fmt.Sprintf("webhook-%d", i+1),
			Hash:      hash,
			Scopes:    []Scope{ScopeWebhook},
			CreatedAt: now,
		}
		if err := store.Create(ctx, key); err != nil {
			return nil, fmt.Errorf("create webhook API key %d: %w", i+1, err)
		}
		created++
	}
	if created == 0 {
		return nil, fmt.Errorf("at least one webhook API key is required")
	}
	return NewService(store), nil
}
