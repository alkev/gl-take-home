package store

import (
	"errors"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/google/uuid"
)

func TestNew(t *testing.T) {
	s := New(100, 16384, 0)
	if s.Len() != 0 {
		t.Fatalf("new store must be empty, got Len=%d", s.Len())
	}
	if s.Dimension() != 100 {
		t.Fatalf("dimension mismatch: got %d, want 100", s.Dimension())
	}
}

func TestNewPanicsOnBadDim(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for dim=0")
		}
	}()
	_ = New(0, 16384, 0)
}

func TestRowSlice(t *testing.T) {
	s := New(4, 4, 0) // chunkSize=4 rows, dim=4
	s.chunks = [][]float32{
		make([]float32, 16),
		make([]float32, 16),
	}
	r0 := s.rowSlice(0)
	r5 := s.rowSlice(5)
	if &r0[0] != &s.chunks[0][0] {
		t.Fatal("rowSlice(0) should point to chunk[0][0]")
	}
	if &r5[0] != &s.chunks[1][4] {
		t.Fatal("rowSlice(5) should point to chunk[1][4]")
	}
}

func TestInsertOneRoundtrip(t *testing.T) {
	s := New(3, 4, 0)
	id, err := s.InsertOne("King", []float32{3, 0, 4})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == (uuid.UUID{}) {
		t.Fatal("id should not be zero")
	}
	if s.Len() != 1 {
		t.Fatalf("Len = %d, want 1", s.Len())
	}
}

func TestInsertRejectsBadDim(t *testing.T) {
	s := New(3, 4, 0)
	if _, err := s.InsertOne("x", []float32{1, 2}); !errors.Is(err, ErrBadDimension) {
		t.Fatalf("want ErrBadDimension, got %v", err)
	}
}

func TestInsertRejectsEmptyLabel(t *testing.T) {
	s := New(3, 4, 0)
	if _, err := s.InsertOne("", []float32{1, 0, 0}); !errors.Is(err, ErrEmptyLabel) {
		t.Fatalf("want ErrEmptyLabel, got %v", err)
	}
}

func TestInsertRejectsNaN(t *testing.T) {
	s := New(3, 4, 0)
	nan := float32(math.NaN())
	if _, err := s.InsertOne("x", []float32{1, nan, 0}); !errors.Is(err, ErrNonFiniteValue) {
		t.Fatalf("want ErrNonFiniteValue, got %v", err)
	}
}

func TestInsertRejectsInf(t *testing.T) {
	s := New(3, 4, 0)
	inf := float32(math.Inf(1))
	if _, err := s.InsertOne("x", []float32{inf, 0, 0}); !errors.Is(err, ErrNonFiniteValue) {
		t.Fatalf("want ErrNonFiniteValue, got %v", err)
	}
}

func TestInsertRejectsZeroVector(t *testing.T) {
	s := New(3, 4, 0)
	if _, err := s.InsertOne("x", []float32{0, 0, 0}); !errors.Is(err, ErrZeroVector) {
		t.Fatalf("want ErrZeroVector, got %v", err)
	}
}

func TestInsertRejectsDuplicateLabel(t *testing.T) {
	s := New(3, 4, 0)
	if _, err := s.InsertOne("King", []float32{1, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertOne("king", []float32{0, 1, 0}); !errors.Is(err, ErrDuplicateLabel) {
		t.Fatalf("want ErrDuplicateLabel (case-insensitive), got %v", err)
	}
}

func TestInsertBatchAllOrNothing(t *testing.T) {
	s := New(3, 4, 0)
	in := []Input{
		{Label: "a", Data: []float32{1, 0, 0}},
		{Label: "b", Data: []float32{0, 1, 0}},
		{Label: "", Data: []float32{0, 0, 1}}, // bad: empty label
	}
	if _, err := s.InsertBatch(in); err == nil {
		t.Fatal("expected error from bad batch")
	}
	if s.Len() != 0 {
		t.Fatalf("batch should be all-or-nothing, but %d rows were inserted", s.Len())
	}
}

func TestInsertBatchHappyPath(t *testing.T) {
	s := New(3, 4, 0)
	in := []Input{
		{Label: "a", Data: []float32{1, 0, 0}},
		{Label: "b", Data: []float32{0, 1, 0}},
	}
	ids, err := s.InsertBatch(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("got %d ids, want 2", len(ids))
	}
	if s.Len() != 2 {
		t.Fatalf("Len = %d, want 2", s.Len())
	}
}

func TestInsertBatchRejectsDupLabelsWithinBatch(t *testing.T) {
	s := New(3, 4, 0)
	in := []Input{
		{Label: "King", Data: []float32{1, 0, 0}},
		{Label: "king", Data: []float32{0, 1, 0}},
	}
	if _, err := s.InsertBatch(in); !errors.Is(err, ErrDuplicateLabel) {
		t.Fatalf("want ErrDuplicateLabel, got %v", err)
	}
	if s.Len() != 0 {
		t.Fatal("no rows should have been inserted")
	}
}

func TestGetByUUIDAndLabel(t *testing.T) {
	s := New(3, 4, 0)
	id, err := s.InsertOne("King", []float32{3, 0, 4})
	if err != nil {
		t.Fatal(err)
	}
	e1, err := s.GetByUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	if e1.Label != "King" {
		t.Fatalf("label = %q, want %q", e1.Label, "King")
	}
	e2, err := s.GetByLabel("king")
	if err != nil {
		t.Fatal(err)
	}
	if e2.UUID != id {
		t.Fatalf("label lookup returned wrong UUID")
	}
}

func TestGetNotFound(t *testing.T) {
	s := New(3, 4, 0)
	if _, err := s.GetByUUID(uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := s.GetByLabel("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestCompareCosineSimilarity(t *testing.T) {
	s := New(3, 4, 0)
	a, _ := s.InsertOne("a", []float32{1, 0, 0})
	b, _ := s.InsertOne("b", []float32{0, 1, 0})
	c, _ := s.InsertOne("c", []float32{-1, 0, 0})

	self, err := s.Compare(a, a)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(self-1.0)) > 1e-5 {
		t.Fatalf("self-compare = %v, want 1", self)
	}
	orth, _ := s.Compare(a, b)
	if math.Abs(float64(orth)) > 1e-5 {
		t.Fatalf("orthogonal = %v, want 0", orth)
	}
	anti, _ := s.Compare(a, c)
	if math.Abs(float64(anti+1.0)) > 1e-5 {
		t.Fatalf("anti-parallel = %v, want -1", anti)
	}
}

func TestCompareNotFound(t *testing.T) {
	s := New(3, 4, 0)
	a, _ := s.InsertOne("a", []float32{1, 0, 0})
	if _, err := s.Compare(a, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestTopKHeap(t *testing.T) {
	h := newTopK(3)
	h.offer(1, 0.9)
	h.offer(2, 0.5)
	h.offer(3, 0.8)
	h.offer(4, 0.1)
	h.offer(5, 0.95)
	results := h.sorted()
	if len(results) != 3 {
		t.Fatalf("got %d, want 3", len(results))
	}
	wantScores := []float32{0.95, 0.9, 0.8}
	for i, r := range results {
		if math.Abs(float64(r.score-wantScores[i])) > 1e-5 {
			t.Fatalf("result[%d].score = %v, want %v", i, r.score, wantScores[i])
		}
	}
}

func TestNearestBasic(t *testing.T) {
	s := New(3, 4, 0)
	for _, p := range []struct {
		label string
		data  []float32
	}{
		{"query", []float32{1, 0, 0}},
		{"near", []float32{0.99, 0.1, 0}},
		{"medium", []float32{0.5, 0.5, 0.7}},
		{"far", []float32{0, 1, 0}},
	} {
		if _, err := s.InsertOne(p.label, p.data); err != nil {
			t.Fatal(err)
		}
	}

	res, err := s.Nearest("query", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("got %d results, want 3", len(res))
	}
	wantOrder := []string{"near", "medium", "far"}
	for i, w := range wantOrder {
		if res[i].Label != w {
			t.Fatalf("position %d: got %q, want %q", i, res[i].Label, w)
		}
	}
	for i := 1; i < len(res); i++ {
		if res[i].Distance < res[i-1].Distance {
			t.Fatalf("results not ordered by distance")
		}
	}
}

func TestNearestExcludesQuery(t *testing.T) {
	s := New(3, 4, 0)
	_, _ = s.InsertOne("q", []float32{1, 0, 0})
	_, _ = s.InsertOne("other", []float32{0.9, 0.1, 0})
	res, err := s.Nearest("q", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Label != "other" {
		t.Fatalf("got %q, must exclude the query", res[0].Label)
	}
}

func TestNearestEmptyStore(t *testing.T) {
	s := New(3, 4, 0)
	if _, err := s.Nearest("x", 1); !errors.Is(err, ErrStoreEmpty) {
		t.Fatalf("want ErrStoreEmpty, got %v", err)
	}
}

func TestNearestKOutOfRange(t *testing.T) {
	s := New(3, 4, 0)
	_, _ = s.InsertOne("q", []float32{1, 0, 0})
	_, _ = s.InsertOne("a", []float32{0, 1, 0})
	if _, err := s.Nearest("q", 0); !errors.Is(err, ErrKOutOfRange) {
		t.Fatalf("want ErrKOutOfRange, got %v", err)
	}
	if _, err := s.Nearest("q", 2); !errors.Is(err, ErrKOutOfRange) {
		t.Fatalf("k > n-1 must fail, got %v", err)
	}
}

func TestNearestUnknownQuery(t *testing.T) {
	s := New(3, 4, 0)
	_, _ = s.InsertOne("a", []float32{1, 0, 0})
	if _, err := s.Nearest("missing", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestNearestParallelConsistency(t *testing.T) {
	s := New(4, 8, 0)
	for i := 0; i < 100; i++ {
		vec := []float32{float32(i), float32(i + 1), 1, 1}
		if _, err := s.InsertOne(fmt.Sprintf("w%d", i), vec); err != nil {
			t.Fatal(err)
		}
	}
	res, err := s.Nearest("w42", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 10 {
		t.Fatalf("got %d, want 10", len(res))
	}
	for i := 1; i < len(res); i++ {
		if res[i].Distance+1e-5 < res[i-1].Distance {
			t.Fatalf("distances out of order at %d", i)
		}
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	tmp := t.TempDir() + "/snap.bin"
	s := New(3, 4, 0)
	aID, _ := s.InsertOne("alpha", []float32{1, 0, 0})
	bID, _ := s.InsertOne("beta", []float32{3, 0, 4})
	if err := s.Save(tmp); err != nil {
		t.Fatal(err)
	}
	s2 := New(3, 4, 0)
	if err := s2.Load(tmp); err != nil {
		t.Fatal(err)
	}
	if s2.Len() != 2 {
		t.Fatalf("Len = %d, want 2", s2.Len())
	}
	a, err := s2.GetByUUID(aID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Label != "alpha" || a.Data[0] != 1 {
		t.Fatalf("alpha lost: %+v", a)
	}
	b, err := s2.GetByUUID(bID)
	if err != nil {
		t.Fatal(err)
	}
	if b.Label != "beta" || b.Data[0] != 3 || b.Data[2] != 4 {
		t.Fatalf("beta lost: %+v", b)
	}
}

func TestSnapshotLoadCorruptMagic(t *testing.T) {
	tmp := t.TempDir() + "/bad.bin"
	if err := os.WriteFile(tmp, []byte("NOTVALIDFILE"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(3, 4, 0)
	if err := s.Load(tmp); err == nil {
		t.Fatal("expected error for bad magic")
	}
}
