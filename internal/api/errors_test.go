package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, 418, "teapot")
	if w.Code != 418 {
		t.Fatalf("code = %d, want 418", w.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "teapot" || body["code"] != float64(418) {
		t.Fatalf("body wrong: %+v", body)
	}
	if got := w.Result().Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}
}
