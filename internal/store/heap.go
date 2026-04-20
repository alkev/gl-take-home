package store

import (
	"container/heap"
	"sort"
)

type scored struct {
	row   int
	score float32
}

// minHeap is a min-heap of scored by score. Implements heap.Interface.
type minHeap []scored

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i].score < h[j].score }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)        { *h = append(*h, x.(scored)) }
func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// topK maintains the top-k highest-scoring items via a min-heap of size k.
type topK struct {
	k int
	h *minHeap
}

func newTopK(k int) *topK {
	h := make(minHeap, 0, k)
	return &topK{k: k, h: &h}
}

// offer adds (row, score) if the heap has room or score beats the current min.
func (t *topK) offer(row int, score float32) {
	if t.h.Len() < t.k {
		heap.Push(t.h, scored{row: row, score: score})
		return
	}
	if score > (*t.h)[0].score {
		(*t.h)[0] = scored{row: row, score: score}
		heap.Fix(t.h, 0)
	}
}

// sorted returns items sorted descending by score.
func (t *topK) sorted() []scored {
	out := make([]scored, t.h.Len())
	copy(out, *t.h)
	sort.Slice(out, func(i, j int) bool { return out[i].score > out[j].score })
	return out
}
