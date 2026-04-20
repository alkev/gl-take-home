package store

import (
	"github.com/google/uuid"

	"github.com/alkev/gl_take_home/internal/vecmath"
)

// Compare returns cosine_similarity(a, b) for two stored embeddings.
func (s *Store) Compare(a, b uuid.UUID) (float32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ia, ok := s.byUUID[a]
	if !ok {
		return 0, ErrNotFound
	}
	ib, ok := s.byUUID[b]
	if !ok {
		return 0, ErrNotFound
	}
	return vecmath.Dot(s.rowSlice(ia), s.rowSlice(ib)) * s.invNorms[ia] * s.invNorms[ib], nil
}
