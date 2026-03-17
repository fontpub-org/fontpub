package updateapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/fontpub-org/fontpub/go/internal/indexer/httpx"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

var sha40Pattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type UpdateRequest struct {
	Repository string `json:"repository"`
	SHA        string `json:"sha"`
	Ref        string `json:"ref"`
}

type Verifier interface {
	Verify(ctx context.Context, rawToken string) (protocol.OIDCClaims, error)
}

type Processor interface {
	Process(ctx context.Context, req UpdateRequest, claims protocol.OIDCClaims) (status int, body any)
}

type Server struct {
	Verifier  Verifier
	Processor Processor
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/update", s.handleUpdate)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" && r.URL.Path != "/v1/update" {
			httpx.WriteError(w, http.StatusNotFound, "INTERNAL_ERROR", "route not found", map[string]any{
				"path": r.URL.Path,
			})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "INTERNAL_ERROR", "method not allowed", map[string]any{
			"method": r.Method,
		})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "INTERNAL_ERROR", "method not allowed", map[string]any{
			"method": r.Method,
		})
		return
	}

	req, errObj, status := ParseUpdateRequest(r.Body)
	if errObj != nil {
		httpx.WriteJSON(w, status, protocol.ErrorEnvelope{Error: *errObj})
		return
	}

	rawToken, errObj, status := ExtractBearerToken(r.Header)
	if errObj != nil {
		httpx.WriteJSON(w, status, protocol.ErrorEnvelope{Error: *errObj})
		return
	}

	if s.Verifier == nil {
		httpx.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "token verifier is not configured", nil)
		return
	}

	claims, err := s.Verifier.Verify(r.Context(), rawToken)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid bearer token", map[string]any{
			"reason": err.Error(),
		})
		return
	}
	if err := protocol.ValidateOIDCClaims(claims); err != nil {
		httpx.WriteJSON(w, statusFromProtocolError(err.Error()), protocol.ErrorEnvelope{
			Error: protocol.ErrorObject{
				Code:    codeFromProtocolError(err.Error()),
				Message: err.Error(),
				Details: map[string]any{},
			},
		})
		return
	}
	if errObj, status := ValidateRequestMatchesClaims(req, claims); errObj != nil {
		httpx.WriteJSON(w, status, protocol.ErrorEnvelope{Error: *errObj})
		return
	}

	if s.Processor == nil {
		httpx.WriteError(w, http.StatusNotImplemented, "INTERNAL_ERROR", "update processor is not implemented", nil)
		return
	}
	status, body := s.Processor.Process(r.Context(), req, claims)
	httpx.WriteJSON(w, status, body)
}

func ParseUpdateRequest(r io.Reader) (UpdateRequest, *protocol.ErrorObject, int) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	var req UpdateRequest
	if err := dec.Decode(&req); err != nil {
		return UpdateRequest{}, &protocol.ErrorObject{
			Code:    requestErrorCode(err),
			Message: "invalid update request body",
			Details: map[string]any{"reason": err.Error()},
		}, http.StatusBadRequest
	}
	if err := dec.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		return UpdateRequest{}, &protocol.ErrorObject{
			Code:    "REQUEST_SCHEMA_INVALID",
			Message: "invalid update request body",
			Details: map[string]any{"reason": "request body must contain exactly one JSON object"},
		}, http.StatusBadRequest
	}
	if req.Repository == "" || req.SHA == "" || req.Ref == "" {
		return UpdateRequest{}, &protocol.ErrorObject{
			Code:    "REQUEST_SCHEMA_INVALID",
			Message: "missing required request field",
			Details: map[string]any{},
		}, http.StatusBadRequest
	}
	if !sha40Pattern.MatchString(req.SHA) {
		return UpdateRequest{}, &protocol.ErrorObject{
			Code:    "REQUEST_SCHEMA_INVALID",
			Message: "invalid sha",
			Details: map[string]any{"field": "sha"},
		}, http.StatusBadRequest
	}
	return req, nil, http.StatusOK
}

func ExtractBearerToken(header http.Header) (string, *protocol.ErrorObject, int) {
	raw := header.Get("Authorization")
	if raw == "" {
		return "", &protocol.ErrorObject{
			Code:    "AUTH_REQUIRED",
			Message: "missing Authorization header",
			Details: map[string]any{},
		}, http.StatusUnauthorized
	}
	if !strings.HasPrefix(raw, "Bearer ") {
		return "", &protocol.ErrorObject{
			Code:    "AUTH_INVALID_TOKEN",
			Message: "Authorization header must use Bearer",
			Details: map[string]any{},
		}, http.StatusUnauthorized
	}
	token := strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
	if token == "" || strings.Contains(token, " ") {
		return "", &protocol.ErrorObject{
			Code:    "AUTH_INVALID_TOKEN",
			Message: "invalid bearer token",
			Details: map[string]any{},
		}, http.StatusUnauthorized
	}
	return token, nil, http.StatusOK
}

func ValidateRequestMatchesClaims(req UpdateRequest, claims protocol.OIDCClaims) (*protocol.ErrorObject, int) {
	if strings.ToLower(req.Repository) != strings.ToLower(claims.Repository) || req.SHA != claims.SHA || req.Ref != claims.Ref {
		return &protocol.ErrorObject{
			Code:    "AUTH_CLAIMS_MISMATCH",
			Message: "request body does not match JWT claims",
			Details: map[string]any{},
		}, http.StatusBadRequest
	}
	return nil, http.StatusOK
}

func requestErrorCode(err error) string {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return "REQUEST_INVALID_JSON"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return "REQUEST_INVALID_JSON"
	}
	return "REQUEST_SCHEMA_INVALID"
}

func codeFromProtocolError(message string) string {
	code, _, found := strings.Cut(message, ":")
	if !found {
		return "AUTH_INVALID_TOKEN"
	}
	return strings.TrimSpace(code)
}

func statusFromProtocolError(message string) int {
	switch codeFromProtocolError(message) {
	case "AUTH_CLAIMS_MISSING":
		return http.StatusUnauthorized
	case "AUTH_CLAIMS_MISMATCH":
		return http.StatusBadRequest
	case "WORKFLOW_NOT_ALLOWED":
		return http.StatusForbidden
	default:
		return http.StatusUnauthorized
	}
}

type NotImplementedProcessor struct{}

func (NotImplementedProcessor) Process(_ context.Context, _ UpdateRequest, _ protocol.OIDCClaims) (int, any) {
	return http.StatusNotImplemented, protocol.ErrorEnvelope{
		Error: protocol.ErrorObject{
			Code:    "INTERNAL_ERROR",
			Message: "update processor is not implemented",
			Details: map[string]any{},
		},
	}
}

type StaticVerifier struct {
	Err error
}

func (v StaticVerifier) Verify(_ context.Context, _ string) (protocol.OIDCClaims, error) {
	if v.Err != nil {
		return protocol.OIDCClaims{}, v.Err
	}
	return protocol.OIDCClaims{}, fmt.Errorf("verifier returned no claims")
}
