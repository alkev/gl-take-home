package api

import "github.com/alkev/gl_take_home/internal/store"

// insertRequest is the body of POST /vectors.
type insertRequest struct {
	Embeddings []insertItem `json:"embeddings"`
}

type insertItem struct {
	Label string    `json:"label"`
	Data  []float32 `json:"data"`
}

// toInputs converts the request into store-layer inputs.
func (r insertRequest) toInputs() []store.Input {
	out := make([]store.Input, len(r.Embeddings))
	for i, e := range r.Embeddings {
		out[i] = store.Input{Label: e.Label, Data: e.Data}
	}
	return out
}
