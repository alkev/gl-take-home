package api

import "github.com/alkev/gl_take_home/internal/store"

// insertResponse is the body returned by POST /vectors.
type insertResponse struct {
	Inserted   int               `json:"inserted"`
	Embeddings []store.Embedding `json:"embeddings"`
}

// nearestResponse is the body returned by GET /nearest.
type nearestResponse struct {
	Query   string         `json:"query"`
	Results []store.Result `json:"results"`
}

// compareResponse is the body returned by GET /compare/{metric}.
type compareResponse struct {
	Metric string  `json:"metric"`
	UUID1  string  `json:"uuid1"`
	UUID2  string  `json:"uuid2"`
	Result float32 `json:"result"`
}

// healthResponse is the body returned by GET /health.
type healthResponse struct {
	Status        string `json:"status"`
	VectorsLoaded int    `json:"vectors_loaded"`
	UptimeS       int64  `json:"uptime_s"`
}

// snapshotResponse is the body returned by POST /snapshot.
type snapshotResponse struct {
	Path       string `json:"path"`
	Vectors    int    `json:"vectors"`
	Bytes      int64  `json:"bytes"`
	DurationMs int64  `json:"duration_ms"`
}
