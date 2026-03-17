package httpx

import (
	"encoding/json"
	"net/http"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func WriteError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	WriteJSON(w, status, protocol.ErrorEnvelope{
		Error: protocol.ErrorObject{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
