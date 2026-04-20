package store

// rowSlice returns the dim-length slice backing row i. Assumes the caller
// holds the appropriate lock.
func (s *Store) rowSlice(i int) []float32 {
	ci := i / s.chunkSize
	off := (i % s.chunkSize) * s.dim
	return s.chunks[ci][off : off+s.dim]
}

// appendRow places a vector at row index s.n and advances n. Assumes the
// caller holds the write lock. Grows the chunks slice by one chunk when
// the current last chunk is full.
func (s *Store) appendRow(data []float32) int {
	if s.n%s.chunkSize == 0 {
		s.chunks = append(s.chunks, make([]float32, s.chunkSize*s.dim))
	}
	row := s.rowSlice(s.n)
	copy(row, data)
	idx := s.n
	s.n++
	return idx
}
