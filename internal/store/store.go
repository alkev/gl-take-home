package store

import (
	"sync"

	"github.com/google/uuid"
)

// Store holds vectors in a chunked slab plus indexes keyed by UUID and label.
type Store struct {
	dim       int
	chunkSize int
	chunks    [][]float32
	invNorms  []float32
	meta      []rowMeta
	byUUID    map[uuid.UUID]int
	byLabel   map[string]int
	n         int
	mu        sync.RWMutex
}

// New returns an empty store. chunkSize must be > 0. initialCap (rows) is a
// hint used to pre-allocate chunks; 0 means no pre-allocation.
func New(dim, chunkSize, initialCap int) *Store {
	if dim <= 0 {
		panic("store: dim must be > 0")
	}
	if chunkSize <= 0 {
		panic("store: chunkSize must be > 0")
	}
	s := &Store{
		dim:       dim,
		chunkSize: chunkSize,
		byUUID:    make(map[uuid.UUID]int),
		byLabel:   make(map[string]int),
	}
	if initialCap > 0 {
		needed := (initialCap + chunkSize - 1) / chunkSize
		s.chunks = make([][]float32, 0, needed)
		s.invNorms = make([]float32, 0, initialCap)
		s.meta = make([]rowMeta, 0, initialCap)
	}
	return s
}

// Len returns the number of stored embeddings.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.n
}

// Dimension returns the configured vector dimension.
func (s *Store) Dimension() int { return s.dim }
