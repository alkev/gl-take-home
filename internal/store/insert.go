package store

import (
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"

	"github.com/alkev/gl_take_home/internal/vecmath"
)

// Input is a single insert request.
type Input struct {
	Label string
	Data  []float32
}

// InsertOne inserts a single embedding and returns its UUID. Validates
// dimension, finiteness, label, and case-insensitive label uniqueness.
func (s *Store) InsertOne(label string, data []float32) (uuid.UUID, error) {
	if err := s.validate(label, data); err != nil {
		return uuid.UUID{}, err
	}
	inv := vecmath.InvNorm(data)
	if inv == 0 {
		return uuid.UUID{}, ErrZeroVector
	}
	id := uuid.New()
	s.mu.Lock()
	defer s.mu.Unlock()
	lower := strings.ToLower(label)
	if _, exists := s.byLabel[lower]; exists {
		return uuid.UUID{}, fmt.Errorf("%w: %q", ErrDuplicateLabel, label)
	}
	rowIdx := s.appendRow(data)
	s.invNorms = append(s.invNorms, inv)
	s.meta = append(s.meta, rowMeta{UUID: id, Label: label})
	s.byUUID[id] = rowIdx
	s.byLabel[lower] = rowIdx
	s.dirty = true
	return id, nil
}

// InsertBatch inserts a batch atomically: either all rows succeed or none.
// Order of returned UUIDs matches order of input.
func (s *Store) InsertBatch(in []Input) ([]uuid.UUID, error) {
	if len(in) == 0 {
		return nil, nil
	}
	invNorms := make([]float32, len(in))
	lowers := make([]string, len(in))
	seen := make(map[string]int, len(in))
	for i, e := range in {
		if err := s.validate(e.Label, e.Data); err != nil {
			return nil, fmt.Errorf("embedding %d: %w", i, err)
		}
		inv := vecmath.InvNorm(e.Data)
		if inv == 0 {
			return nil, fmt.Errorf("embedding %d: %w", i, ErrZeroVector)
		}
		lower := strings.ToLower(e.Label)
		if prev, dup := seen[lower]; dup {
			return nil, fmt.Errorf("embedding %d: %w (also at %d)", i, ErrDuplicateLabel, prev)
		}
		seen[lower] = i
		invNorms[i] = inv
		lowers[i] = lower
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, lower := range lowers {
		if _, exists := s.byLabel[lower]; exists {
			return nil, fmt.Errorf("embedding %d: %w: %q", i, ErrDuplicateLabel, in[i].Label)
		}
	}

	ids := make([]uuid.UUID, len(in))
	for i, e := range in {
		id := uuid.New()
		rowIdx := s.appendRow(e.Data)
		s.invNorms = append(s.invNorms, invNorms[i])
		s.meta = append(s.meta, rowMeta{UUID: id, Label: e.Label})
		s.byUUID[id] = rowIdx
		s.byLabel[lowers[i]] = rowIdx
		ids[i] = id
	}
	s.dirty = true
	return ids, nil
}

// maxLabelBytes matches the uint16 length prefix used by the on-disk
// snapshot format. Rejecting over-long labels at insert avoids a
// delayed, confusing failure at the next Save.
const maxLabelBytes = 0xFFFF

func (s *Store) validate(label string, data []float32) error {
	if label == "" {
		return ErrEmptyLabel
	}
	if len(label) > maxLabelBytes {
		return fmt.Errorf("%w: %d bytes, max %d", ErrLabelTooLong, len(label), maxLabelBytes)
	}
	if len(data) != s.dim {
		return fmt.Errorf("%w: got %d, want %d", ErrBadDimension, len(data), s.dim)
	}
	for _, f := range data {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			return ErrNonFiniteValue
		}
	}
	return nil
}
