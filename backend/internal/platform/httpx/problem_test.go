package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// decodeProblem, yanıtın bir RFC 9457 problem yanıtı olduğunu doğrular ve gövdesini çözer.
func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) Problem {
	t.Helper()

	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want %q", ct, "application/problem+json")
	}

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("problem gövdesi çözülemedi: %v\n%s", err, rec.Body.String())
	}
	return p
}

func TestWriteProblemFillsDefaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/terms/42", nil)
	req = req.WithContext(context.WithValue(req.Context(), requestIDKey, "test-id"))
	rec := httptest.NewRecorder()

	err := WriteProblem(rec, req, Problem{Status: http.StatusConflict, Code: "CONFLICT"})
	if err != nil {
		t.Fatalf("WriteProblem() hata: %v", err)
	}

	if rec.Code != http.StatusConflict {
		t.Errorf("durum = %d, want %d", rec.Code, http.StatusConflict)
	}

	got := decodeProblem(t, rec)
	want := Problem{
		Type:      "about:blank",
		Title:     "Conflict",
		Status:    http.StatusConflict,
		Instance:  "/terms/42",
		Code:      "CONFLICT",
		RequestID: "test-id",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("problem =\n  %+v\nwant\n  %+v", got, want)
	}
}
