package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteJSON(rr, http.StatusCreated, map[string]any{"status": "ok"})

	if rr.Code != http.StatusCreated {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content-type: %s", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestWriteErrorNormalizesNilDetails(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteError(rr, http.StatusBadRequest, "REQUEST_INVALID", "bad request", nil)

	var env protocol.ErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if env.Error.Code != "REQUEST_INVALID" || env.Error.Message != "bad request" {
		t.Fatalf("unexpected error envelope: %#v", env)
	}
	if env.Error.Details == nil || len(env.Error.Details) != 0 {
		t.Fatalf("unexpected details: %#v", env.Error.Details)
	}
}
