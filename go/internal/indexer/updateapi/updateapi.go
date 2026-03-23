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

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
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
	Verifier      Verifier
	Processor     Processor
	ArtifactStore artifacts.Store
}

func (s Server) Handler() http.Handler {
	mux := s.routes()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isRoutablePath(r.URL.Path) {
			s.writeRouteNotFound(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s Server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/index.json", s.handleRootIndex)
	mux.HandleFunc("/v1/packages/", s.handlePackageRead)
	mux.HandleFunc("/v1/update", s.handleUpdate)
	return mux
}

func (s Server) writeRouteNotFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, http.StatusNotFound, "INTERNAL_ERROR", "route not found", map[string]any{
		"path": r.URL.Path,
	})
}

func (s Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodPost) {
		return
	}

	req, claims, errObj, status := s.authorizeUpdateRequest(r)
	if errObj != nil {
		httpx.WriteJSON(w, status, protocol.ErrorEnvelope{Error: *errObj})
		return
	}

	if s.Processor == nil {
		httpx.WriteError(w, http.StatusNotImplemented, "INTERNAL_ERROR", "update processor is not implemented", nil)
		return
	}
	status, body := s.Processor.Process(r.Context(), req, claims)
	maybeSetRetryAfter(w, status, body)
	httpx.WriteJSON(w, status, body)
}

func (s Server) authorizeUpdateRequest(r *http.Request) (UpdateRequest, protocol.OIDCClaims, *protocol.ErrorObject, int) {
	req, errObj, status := ParseUpdateRequest(r.Body)
	if errObj != nil {
		return UpdateRequest{}, protocol.OIDCClaims{}, errObj, status
	}
	rawToken, errObj, status := ExtractBearerToken(r.Header)
	if errObj != nil {
		return UpdateRequest{}, protocol.OIDCClaims{}, errObj, status
	}
	claims, errObj, status := s.verifyClaims(r.Context(), rawToken)
	if errObj != nil {
		return UpdateRequest{}, protocol.OIDCClaims{}, errObj, status
	}
	if errObj, status := ValidateRequestMatchesClaims(req, claims); errObj != nil {
		return UpdateRequest{}, protocol.OIDCClaims{}, errObj, status
	}
	return req, claims, nil, http.StatusOK
}

func (s Server) verifyClaims(ctx context.Context, rawToken string) (protocol.OIDCClaims, *protocol.ErrorObject, int) {
	if s.Verifier == nil {
		return protocol.OIDCClaims{}, internalError("token verifier is not configured"), http.StatusInternalServerError
	}
	claims, err := s.Verifier.Verify(ctx, rawToken)
	if err != nil {
		return protocol.OIDCClaims{}, errorObject("AUTH_INVALID_TOKEN", "invalid bearer token", map[string]any{
			"reason": err.Error(),
		}), http.StatusUnauthorized
	}
	if err := protocol.ValidateOIDCClaims(claims); err != nil {
		return protocol.OIDCClaims{}, protocolErrorObject(err.Error()), statusFromProtocolError(err.Error())
	}
	return claims, nil, http.StatusOK
}

func protocolErrorObject(message string) *protocol.ErrorObject {
	return &protocol.ErrorObject{
		Code:    codeFromProtocolError(message),
		Message: message,
		Details: map[string]any{},
	}
}

func methodAllowed(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method == want {
		return true
	}
	httpx.WriteError(w, http.StatusMethodNotAllowed, "INTERNAL_ERROR", "method not allowed", map[string]any{
		"method": r.Method,
	})
	return false
}

func isRoutablePath(path string) bool {
	return path == "/healthz" ||
		path == "/v1/update" ||
		path == "/v1/index.json" ||
		strings.HasPrefix(path, "/v1/packages/")
}

func maybeSetRetryAfter(w http.ResponseWriter, status int, body any) {
	if status != http.StatusServiceUnavailable {
		return
	}
	env, ok := body.(protocol.ErrorEnvelope)
	if !ok || env.Error.Code != "INDEX_CONFLICT" {
		return
	}
	w.Header().Set("Retry-After", "1")
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
