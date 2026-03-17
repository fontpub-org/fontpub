package updateapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ma/fontpub/go/internal/protocol"
)

type fakeVerifier struct {
	claims protocol.OIDCClaims
	err    error
}

func (v fakeVerifier) Verify(_ context.Context, _ string) (protocol.OIDCClaims, error) {
	return v.claims, v.err
}

type fakeProcessor struct {
	called bool
	status int
	body   any
}

func (p *fakeProcessor) Process(_ context.Context, _ UpdateRequest, _ protocol.OIDCClaims) (int, any) {
	p.called = true
	return p.status, p.body
}

func TestParseUpdateRequest(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantCode   string
		wantStatus int
	}{
		{
			name:       "valid",
			body:       `{"repository":"example/family","sha":"0123456789abcdef0123456789abcdef01234567","ref":"refs/tags/v1.2.3"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "malformed json",
			body:       `{"repository":`,
			wantCode:   "REQUEST_INVALID_JSON",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing field",
			body:       `{"repository":"example/family","sha":"0123456789abcdef0123456789abcdef01234567"}`,
			wantCode:   "REQUEST_SCHEMA_INVALID",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unexpected field",
			body:       `{"repository":"example/family","sha":"0123456789abcdef0123456789abcdef01234567","ref":"refs/tags/v1.2.3","extra":true}`,
			wantCode:   "REQUEST_SCHEMA_INVALID",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid sha",
			body:       `{"repository":"example/family","sha":"not-a-sha","ref":"refs/tags/v1.2.3"}`,
			wantCode:   "REQUEST_SCHEMA_INVALID",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, errObj, status := ParseUpdateRequest(strings.NewReader(tc.body))
			if tc.wantCode == "" {
				if errObj != nil || status != http.StatusOK {
					t.Fatalf("unexpected error: %#v status=%d", errObj, status)
				}
				return
			}
			if errObj == nil {
				t.Fatalf("expected error")
			}
			if errObj.Code != tc.wantCode || status != tc.wantStatus {
				t.Fatalf("got code=%s status=%d want code=%s status=%d", errObj.Code, status, tc.wantCode, tc.wantStatus)
			}
		})
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		wantToken  string
		wantCode   string
		wantStatus int
	}{
		{name: "valid", header: "Bearer token-1", wantToken: "token-1", wantStatus: http.StatusOK},
		{name: "missing", wantCode: "AUTH_REQUIRED", wantStatus: http.StatusUnauthorized},
		{name: "wrong scheme", header: "Basic abc", wantCode: "AUTH_INVALID_TOKEN", wantStatus: http.StatusUnauthorized},
		{name: "empty token", header: "Bearer  ", wantCode: "AUTH_INVALID_TOKEN", wantStatus: http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			header := http.Header{}
			if tc.header != "" {
				header.Set("Authorization", tc.header)
			}
			token, errObj, status := ExtractBearerToken(header)
			if tc.wantCode == "" {
				if errObj != nil || token != tc.wantToken || status != http.StatusOK {
					t.Fatalf("unexpected result token=%q err=%#v status=%d", token, errObj, status)
				}
				return
			}
			if errObj == nil {
				t.Fatalf("expected error")
			}
			if errObj.Code != tc.wantCode || status != tc.wantStatus {
				t.Fatalf("got code=%s status=%d want code=%s status=%d", errObj.Code, status, tc.wantCode, tc.wantStatus)
			}
		})
	}
}

func TestHandlerAuthFlow(t *testing.T) {
	validBody := `{"repository":"example/family","sha":"0123456789abcdef0123456789abcdef01234567","ref":"refs/tags/v1.2.3"}`
	validClaims := protocol.OIDCClaims{
		Sub:             "repo:example/family:ref:refs/tags/v1.2.3",
		Repository:      "example/family",
		RepositoryID:    "123456789",
		RepositoryOwner: "example",
		SHA:             "0123456789abcdef0123456789abcdef01234567",
		Ref:             "refs/tags/v1.2.3",
		WorkflowRef:     "example/family/.github/workflows/fontpub.yml@refs/heads/main",
		WorkflowSHA:     "89abcdef0123456789abcdef0123456789abcdef",
		JTI:             "jwt-id-1",
		EventName:       "push",
	}

	tests := []struct {
		name       string
		auth       string
		body       string
		verifier   fakeVerifier
		wantStatus int
		wantCode   string
		wantCalled bool
	}{
		{
			name:       "missing authorization",
			body:       validBody,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "AUTH_REQUIRED",
		},
		{
			name:       "verifier error",
			auth:       "Bearer token-1",
			body:       validBody,
			verifier:   fakeVerifier{err: context.DeadlineExceeded},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "AUTH_INVALID_TOKEN",
		},
		{
			name:       "invalid claims",
			auth:       "Bearer token-1",
			body:       validBody,
			verifier:   fakeVerifier{claims: protocol.OIDCClaims{Repository: "example/family"}},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "AUTH_CLAIMS_MISSING",
		},
		{
			name:       "body claim mismatch",
			auth:       "Bearer token-1",
			body:       validBody,
			verifier:   fakeVerifier{claims: mutateClaims(validClaims, func(c *protocol.OIDCClaims) { c.Ref = "refs/tags/v1.2.4" })},
			wantStatus: http.StatusBadRequest,
			wantCode:   "AUTH_CLAIMS_MISMATCH",
		},
		{
			name:       "valid request reaches processor",
			auth:       "Bearer token-1",
			body:       validBody,
			verifier:   fakeVerifier{claims: validClaims},
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			processor := &fakeProcessor{
				status: http.StatusOK,
				body:   map[string]string{"status": "next"},
			}
			server := Server{
				Verifier:  tc.verifier,
				Processor: processor,
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/update", strings.NewReader(tc.body))
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rr := httptest.NewRecorder()
			server.Handler().ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status=%d want %d body=%s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if processor.called != tc.wantCalled {
				t.Fatalf("processor called=%v want %v", processor.called, tc.wantCalled)
			}
			if tc.wantCode != "" {
				var env protocol.ErrorEnvelope
				if err := json.NewDecoder(bytes.NewReader(rr.Body.Bytes())).Decode(&env); err != nil {
					t.Fatalf("decode error: %v", err)
				}
				if env.Error.Code != tc.wantCode {
					t.Fatalf("code=%s want %s", env.Error.Code, tc.wantCode)
				}
			}
		})
	}
}

func TestHandlerRoutes(t *testing.T) {
	server := Server{}
	h := server.Handler()

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unexpected status for unknown route: %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/update", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status for wrong method: %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected healthz status: %d", rr.Code)
	}
}

func mutateClaims(in protocol.OIDCClaims, fn func(*protocol.OIDCClaims)) protocol.OIDCClaims {
	fn(&in)
	return in
}
