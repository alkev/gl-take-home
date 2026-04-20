package store

import "github.com/google/uuid"

// Embedding is the public record returned by the API.
type Embedding struct {
	UUID      uuid.UUID `json:"uuid"`
	Label     string    `json:"label"`
	Dimension int       `json:"dimension"`
	Data      []float32 `json:"data"`
}

// rowMeta is stored internally per row; Label preserves original case.
type rowMeta struct {
	UUID  uuid.UUID
	Label string
}
