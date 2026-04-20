package api

import (
	"encoding/json"
	"net/http"
)

// writeError emits a JSON error response per the project's fixed format:
//
//	{"error": "<msg>", "code": <status>}
func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": msg,
		"code":  code,
	})
}
