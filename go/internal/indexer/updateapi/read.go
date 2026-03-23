package updateapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/derive"
	"github.com/fontpub-org/fontpub/go/internal/indexer/httpx"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func (s Server) handleRootIndex(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	body, etag, errObj, status := s.readRootIndex(r.Context())
	writeReadResult(w, r, body, etag, errObj, status)
}

func (s Server) handlePackageRead(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/packages/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		s.writeRouteNotFound(w, r)
		return
	}
	var (
		body   []byte
		etag   string
		errObj *protocol.ErrorObject
		status int
	)
	switch {
	case len(parts) == 2 && strings.HasSuffix(parts[1], ".json"):
		packageID := strings.ToLower(parts[0] + "/" + strings.TrimSuffix(parts[1], ".json"))
		body, etag, errObj, status = s.readLatestPackageAlias(r.Context(), packageID)
	case len(parts) == 3 && parts[2] == "index.json":
		packageID := strings.ToLower(parts[0] + "/" + parts[1])
		body, etag, errObj, status = s.readPackageVersionsIndex(r.Context(), packageID)
	case len(parts) == 4 && parts[2] == "versions" && strings.HasSuffix(parts[3], ".json"):
		packageID := strings.ToLower(parts[0] + "/" + parts[1])
		versionKey := strings.TrimSuffix(parts[3], ".json")
		body, etag, errObj, status = s.readVersionedPackageDetail(r.Context(), packageID, versionKey)
	default:
		s.writeRouteNotFound(w, r)
		return
	}
	writeReadResult(w, r, body, etag, errObj, status)
}

func writeReadResult(w http.ResponseWriter, r *http.Request, body []byte, etag string, errObj *protocol.ErrorObject, status int) {
	if errObj != nil {
		httpx.WriteJSON(w, status, protocol.ErrorEnvelope{Error: *errObj})
		return
	}
	if etag != "" {
		w.Header().Set("ETag", etag)
		if matchesIfNoneMatch(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func matchesIfNoneMatch(headerValue, etag string) bool {
	if etag == "" {
		return false
	}
	for _, part := range strings.Split(headerValue, ",") {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		if candidate == "*" || candidate == etag || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}

func (s Server) readRootIndex(ctx context.Context) ([]byte, string, *protocol.ErrorObject, int) {
	path := artifacts.RootIndexPath()
	if doc, ok, err := s.readStoredDocument(ctx, path); err != nil {
		return nil, "", internalError(err.Error()), http.StatusInternalServerError
	} else if ok {
		return doc.Body, doc.ETag, nil, http.StatusOK
	}
	details, err := s.listAllDetails(ctx)
	if err != nil {
		return nil, "", internalError(err.Error()), http.StatusInternalServerError
	}
	rootIndex, err := derive.BuildRootIndex(details)
	if err != nil {
		return nil, "", internalError(err.Error()), http.StatusInternalServerError
	}
	return marshalReadDocument(rootIndex)
}

func (s Server) readLatestPackageAlias(ctx context.Context, packageID string) ([]byte, string, *protocol.ErrorObject, int) {
	path := artifacts.LatestAliasPath(packageID)
	if doc, ok, err := s.readStoredDocument(ctx, path); err != nil {
		return nil, "", internalError(err.Error()), http.StatusInternalServerError
	} else if ok {
		return doc.Body, doc.ETag, nil, http.StatusOK
	}
	details, errObj, status := s.listPackageDetails(ctx, packageID)
	if errObj != nil {
		return nil, "", errObj, status
	}
	_, latestDetail, err := derive.BuildPackageVersionsIndex(packageID, details)
	if err != nil {
		return nil, "", internalError(err.Error()), http.StatusInternalServerError
	}
	return marshalReadDocument(latestDetail)
}

func (s Server) readPackageVersionsIndex(ctx context.Context, packageID string) ([]byte, string, *protocol.ErrorObject, int) {
	path := artifacts.PackageVersionsIndexPath(packageID)
	if doc, ok, err := s.readStoredDocument(ctx, path); err != nil {
		return nil, "", internalError(err.Error()), http.StatusInternalServerError
	} else if ok {
		return doc.Body, doc.ETag, nil, http.StatusOK
	}
	details, errObj, status := s.listPackageDetails(ctx, packageID)
	if errObj != nil {
		return nil, "", errObj, status
	}
	index, _, err := derive.BuildPackageVersionsIndex(packageID, details)
	if err != nil {
		return nil, "", internalError(err.Error()), http.StatusInternalServerError
	}
	return marshalReadDocument(index)
}

func (s Server) readVersionedPackageDetail(ctx context.Context, packageID, versionKey string) ([]byte, string, *protocol.ErrorObject, int) {
	if s.ArtifactStore == nil {
		return nil, "", internalError("artifact store is not configured"), http.StatusInternalServerError
	}
	path := artifacts.VersionedPackageDetailPath(packageID, versionKey)
	if doc, ok, err := s.readStoredDocument(ctx, path); err != nil {
		return nil, "", internalError(err.Error()), http.StatusInternalServerError
	} else if ok {
		return doc.Body, doc.ETag, nil, http.StatusOK
	}
	detail, ok, err := s.ArtifactStore.GetVersionedPackageDetail(ctx, packageID, versionKey)
	if err != nil {
		return nil, "", internalError(err.Error()), http.StatusInternalServerError
	}
	if ok {
		return marshalReadDocument(detail)
	}
	details, err := s.ArtifactStore.ListPackageVersionedPackageDetails(ctx, packageID)
	if err != nil {
		return nil, "", internalError(err.Error()), http.StatusInternalServerError
	}
	if len(details) == 0 {
		return nil, "", errorObject("PACKAGE_NOT_FOUND", "package not found", nil), http.StatusNotFound
	}
	return nil, "", errorObject("VERSION_NOT_FOUND", "package version not found", nil), http.StatusNotFound
}

func (s Server) readStoredDocument(ctx context.Context, path string) (artifacts.Document, bool, error) {
	if s.ArtifactStore == nil {
		return artifacts.Document{}, false, nil
	}
	return s.ArtifactStore.GetDocument(ctx, path)
}

func (s Server) listAllDetails(ctx context.Context) ([]protocol.VersionedPackageDetail, error) {
	if s.ArtifactStore == nil {
		return nil, fmt.Errorf("artifact store is not configured")
	}
	return s.ArtifactStore.ListAllVersionedPackageDetails(ctx)
}

func (s Server) listPackageDetails(ctx context.Context, packageID string) ([]protocol.VersionedPackageDetail, *protocol.ErrorObject, int) {
	if s.ArtifactStore == nil {
		return nil, internalError("artifact store is not configured"), http.StatusInternalServerError
	}
	details, err := s.ArtifactStore.ListPackageVersionedPackageDetails(ctx, packageID)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}
	if len(details) == 0 {
		return nil, errorObject("PACKAGE_NOT_FOUND", "package not found", nil), http.StatusNotFound
	}
	return details, nil, http.StatusOK
}

func marshalReadDocument(value any) ([]byte, string, *protocol.ErrorObject, int) {
	body, err := protocol.MarshalCanonical(value)
	if err != nil {
		return nil, "", internalError(err.Error()), http.StatusInternalServerError
	}
	return body, derive.ComputeETag(body), nil, http.StatusOK
}
