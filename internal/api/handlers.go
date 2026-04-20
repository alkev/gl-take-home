package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/alkev/gl_take_home/internal/store"
)

type handlers struct {
	store        *store.Store
	startTime    time.Time
	snapshotFn   func() (path string, bytes int64, err error)
	maxBatchSize int
}

func (h *handlers) handleInsert(w http.ResponseWriter, r *http.Request) {
	var req insertRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if len(req.Embeddings) == 0 {
		writeError(w, http.StatusBadRequest, "embeddings must be non-empty")
		return
	}
	if len(req.Embeddings) > h.maxBatchSize {
		writeError(w, http.StatusBadRequest, "batch size exceeds MAX_BATCH_SIZE")
		return
	}
	ids, err := h.store.InsertBatch(req.toInputs())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	embs := make([]store.Embedding, len(ids))
	for i, id := range ids {
		e, err := h.store.GetByUUID(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "post-insert read failed")
			return
		}
		embs[i] = e
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(insertResponse{Inserted: len(ids), Embeddings: embs})
}

func (h *handlers) handleGet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	word := q.Get("word")
	idStr := q.Get("uuid")
	if (word == "") == (idStr == "") {
		writeError(w, http.StatusBadRequest, "exactly one of 'word' or 'uuid' is required")
		return
	}
	// Per the technical standards table, retrieval against an empty store is 503.
	if h.store.Len() == 0 {
		writeError(w, http.StatusServiceUnavailable, "store is empty")
		return
	}
	encode := func(e store.Embedding) error {
		w.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(w).Encode(e)
	}
	var err error
	if word != "" {
		err = h.store.WithByLabel(word, encode)
	} else {
		id, parseErr := uuid.Parse(idStr)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid uuid: "+parseErr.Error())
			return
		}
		err = h.store.WithByUUID(id, encode)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *handlers) handleCompare(w http.ResponseWriter, r *http.Request) {
	metric := r.PathValue("metric")
	if metric != "cosine_similarity" {
		writeError(w, http.StatusNotFound, "unknown metric")
		return
	}
	q := r.URL.Query()
	id1Str, id2Str := q.Get("uuid1"), q.Get("uuid2")
	if id1Str == "" || id2Str == "" {
		writeError(w, http.StatusBadRequest, "uuid1 and uuid2 are required")
		return
	}
	id1, err := uuid.Parse(id1Str)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid uuid1")
		return
	}
	id2, err := uuid.Parse(id2Str)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid uuid2")
		return
	}
	// Per the technical standards table, retrieval against an empty store is 503.
	if h.store.Len() == 0 {
		writeError(w, http.StatusServiceUnavailable, "store is empty")
		return
	}
	sim, err := h.store.Compare(id1, id2)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(compareResponse{
		Metric: metric, UUID1: id1Str, UUID2: id2Str, Result: sim,
	})
}

func (h *handlers) handleNearest(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	word := q.Get("word")
	if word == "" {
		writeError(w, http.StatusBadRequest, "word is required")
		return
	}
	k := 1
	if ks := q.Get("k"); ks != "" {
		n, err := strconv.Atoi(ks)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "k must be a positive integer")
			return
		}
		k = n
	}
	res, err := h.store.Nearest(word, k)
	switch {
	case errors.Is(err, store.ErrStoreEmpty):
		writeError(w, http.StatusServiceUnavailable, "store is empty")
		return
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "query word not found")
		return
	case errors.Is(err, store.ErrKOutOfRange):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(nearestResponse{Query: word, Results: res})
}

func (h *handlers) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthResponse{
		Status:        "ok",
		VectorsLoaded: h.store.Len(),
		UptimeS:       int64(time.Since(h.startTime).Seconds()),
	})
}

func (h *handlers) handleSnapshot(w http.ResponseWriter, _ *http.Request) {
	if h.snapshotFn == nil {
		writeError(w, http.StatusBadRequest, "SNAPSHOT_PATH not configured")
		return
	}
	start := time.Now()
	path, bytesWritten, err := h.snapshotFn()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snapshotResponse{
		Path:       path,
		Vectors:    h.store.Len(),
		Bytes:      bytesWritten,
		DurationMs: time.Since(start).Milliseconds(),
	})
}
