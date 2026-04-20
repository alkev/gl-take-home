package store

import (
	"runtime"
	"strings"
	"sync"

	"github.com/alkev/gl_take_home/internal/vecmath"
)

// Result is a single hit from a nearest-neighbour search.
type Result struct {
	Embedding
	Distance float32 `json:"distance"` // 1 - cosine_similarity
}

// Nearest returns the k most similar embeddings to the named word, excluding
// the query itself. Results are sorted most-similar-first.
func (s *Store) Nearest(word string, k int) ([]Result, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.n == 0 {
		return nil, ErrStoreEmpty
	}
	qIdx, ok := s.byLabel[strings.ToLower(word)]
	if !ok {
		return nil, ErrNotFound
	}
	if k < 1 || k > s.n-1 {
		return nil, ErrKOutOfRange
	}
	q := s.rowSlice(qIdx)
	invQ := s.invNorms[qIdx]

	numChunks := len(s.chunks)
	workers := runtime.GOMAXPROCS(0)
	if workers > numChunks {
		workers = numChunks
	}
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	local := make([]*topK, workers)
	for w := 0; w < workers; w++ {
		local[w] = newTopK(k)
	}
	chunksPerWorker := (numChunks + workers - 1) / workers
	for w := 0; w < workers; w++ {
		firstChunk := w * chunksPerWorker
		if firstChunk >= numChunks {
			break
		}
		lastChunk := firstChunk + chunksPerWorker
		if lastChunk > numChunks {
			lastChunk = numChunks
		}
		wg.Add(1)
		go func(w, firstChunk, lastChunk int) {
			defer wg.Done()
			tk := local[w]
			for ci := firstChunk; ci < lastChunk; ci++ {
				chunk := s.chunks[ci]
				rowsInChunk := s.chunkSize
				firstRow := ci * s.chunkSize
				if firstRow+rowsInChunk > s.n {
					rowsInChunk = s.n - firstRow
				}
				for r := 0; r < rowsInChunk; r++ {
					rowIdx := firstRow + r
					if rowIdx == qIdx {
						continue
					}
					row := chunk[r*s.dim : (r+1)*s.dim]
					score := vecmath.Dot(q, row) * s.invNorms[rowIdx]
					tk.offer(rowIdx, score)
				}
			}
		}(w, firstChunk, lastChunk)
	}
	wg.Wait()

	merged := newTopK(k)
	for _, tk := range local {
		for _, item := range tk.sorted() {
			merged.offer(item.row, item.score)
		}
	}

	hits := merged.sorted()
	out := make([]Result, len(hits))
	for i, h := range hits {
		cosine := h.score * invQ
		out[i] = Result{
			Embedding: s.readLocked(h.row),
			Distance:  1 - cosine,
		}
	}
	return out, nil
}
