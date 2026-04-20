package store

import (
	"strings"

	"github.com/google/uuid"
)

// GetByUUID returns the embedding with the given UUID, or ErrNotFound.
func (s *Store) GetByUUID(id uuid.UUID) (Embedding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, ok := s.byUUID[id]
	if !ok {
		return Embedding{}, ErrNotFound
	}
	return s.readLocked(idx), nil
}

// GetByLabel returns the embedding with the given label (case-insensitive),
// or ErrNotFound.
func (s *Store) GetByLabel(label string) (Embedding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, ok := s.byLabel[strings.ToLower(label)]
	if !ok {
		return Embedding{}, ErrNotFound
	}
	return s.readLocked(idx), nil
}

// readLocked assembles the public Embedding for row idx. Caller must hold
// at least an RLock.
func (s *Store) readLocked(idx int) Embedding {
	data := make([]float32, s.dim)
	copy(data, s.rowSlice(idx))
	return Embedding{
		UUID:      s.meta[idx].UUID,
		Label:     s.meta[idx].Label,
		Dimension: s.dim,
		Data:      data,
	}
}

// WithByLabel calls fn with an Embedding whose Data is a direct view into
// store-owned memory. The store's read-lock is held for the duration of fn,
// so fn must not retain e.Data past the call.
func (s *Store) WithByLabel(label string, fn func(e Embedding) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, ok := s.byLabel[strings.ToLower(label)]
	if !ok {
		return ErrNotFound
	}
	return fn(Embedding{
		UUID:      s.meta[idx].UUID,
		Label:     s.meta[idx].Label,
		Dimension: s.dim,
		Data:      s.rowSlice(idx),
	})
}

// WithByUUID is the UUID-keyed counterpart of WithByLabel.
func (s *Store) WithByUUID(id uuid.UUID, fn func(e Embedding) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, ok := s.byUUID[id]
	if !ok {
		return ErrNotFound
	}
	return fn(Embedding{
		UUID:      s.meta[idx].UUID,
		Label:     s.meta[idx].Label,
		Dimension: s.dim,
		Data:      s.rowSlice(idx),
	})
}
