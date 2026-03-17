package updateapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ma/fontpub/go/internal/indexer/githubraw"
	"github.com/ma/fontpub/go/internal/indexer/state"
	"github.com/ma/fontpub/go/internal/protocol"
)

const (
	ManifestMaxBytes = 1 << 20
	AssetMaxBytes    = 50 * 1024 * 1024
	PackageMaxBytes  = 2 * 1024 * 1024 * 1024
)

type ValidatedAsset struct {
	Path      string
	URL       string
	SHA256    string
	Format    string
	Style     string
	Weight    int
	SizeBytes int64
}

type ValidationResult struct {
	PackageID   string
	Version     string
	VersionKey  string
	ManifestURL string
	Manifest    protocol.Manifest
	Assets      []ValidatedAsset
}

type ValidationProcessor struct {
	State   state.Store
	Fetcher githubraw.Fetcher
}

func (p ValidationProcessor) Process(ctx context.Context, req UpdateRequest, claims protocol.OIDCClaims) (int, any) {
	result, errObj, status := p.Validate(ctx, req, claims)
	if errObj != nil {
		return status, protocol.ErrorEnvelope{Error: *errObj}
	}
	return http.StatusNotImplemented, protocol.ErrorEnvelope{
		Error: protocol.ErrorObject{
			Code:    "INTERNAL_ERROR",
			Message: "publish sequence is not implemented",
			Details: map[string]any{
				"package_id":   result.PackageID,
				"version":      result.Version,
				"version_key":  result.VersionKey,
				"validated_at": "pre-publish",
			},
		},
	}
}

func (p ValidationProcessor) Validate(ctx context.Context, req UpdateRequest, claims protocol.OIDCClaims) (ValidationResult, *protocol.ErrorObject, int) {
	if p.State == nil {
		return ValidationResult{}, internalError("state store is not configured"), http.StatusInternalServerError
	}
	if p.Fetcher == nil {
		return ValidationResult{}, internalError("upstream fetcher is not configured"), http.StatusInternalServerError
	}
	if err := p.State.CheckAndReserveJTI(ctx, claims.JTI); err != nil {
		if err == state.ErrReplayDetected {
			return ValidationResult{}, errorObject("AUTH_REPLAY_DETECTED", "JWT replay detected", nil), http.StatusUnauthorized
		}
		return ValidationResult{}, internalError(err.Error()), http.StatusInternalServerError
	}
	if err := p.State.CheckOrBindPackage(ctx, strings.ToLower(req.Repository), claims.RepositoryID); err != nil {
		if err == state.ErrOwnershipMismatch {
			return ValidationResult{}, errorObject("OWNERSHIP_MISMATCH", "repository ownership mismatch", nil), http.StatusForbidden
		}
		return ValidationResult{}, internalError(err.Error()), http.StatusInternalServerError
	}

	manifestURL, err := githubraw.BuildManifestURL(req.Repository, req.SHA)
	if err != nil {
		return ValidationResult{}, errorObject("REQUEST_SCHEMA_INVALID", "invalid repository", nil), http.StatusBadRequest
	}
	manifestResult, err := p.Fetcher.Fetch(ctx, manifestURL, ManifestMaxBytes)
	if err != nil {
		return ValidationResult{}, mapFetchError(err, true), mapFetchStatus(err)
	}

	var manifest protocol.Manifest
	if err := json.Unmarshal(manifestResult.Body, &manifest); err != nil {
		return ValidationResult{}, errorObject("MANIFEST_INVALID_JSON", "manifest is not valid JSON", nil), http.StatusUnprocessableEntity
	}
	if err := protocol.ValidateManifest(manifest); err != nil {
		return ValidationResult{}, mapProtocolValidationError(err), mapValidationStatus(err)
	}

	tag := strings.TrimPrefix(req.Ref, "refs/tags/")
	tagVersionKey, err := protocol.NormalizeVersionKey(tag)
	if err != nil {
		return ValidationResult{}, errorObject("TAG_VERSION_MISMATCH", "tag version is invalid", nil), http.StatusUnprocessableEntity
	}
	manifestVersionKey, err := protocol.NormalizeVersionKey(manifest.Version)
	if err != nil {
		return ValidationResult{}, errorObject("VERSION_INVALID", "manifest version is invalid", nil), http.StatusUnprocessableEntity
	}
	if tagVersionKey != manifestVersionKey {
		return ValidationResult{}, errorObject("TAG_VERSION_MISMATCH", "tag version does not match manifest version key", nil), http.StatusUnprocessableEntity
	}

	var totalBytes int64
	assets := make([]ValidatedAsset, 0, len(manifest.Files))
	for _, file := range protocol.SortedManifestFiles(manifest.Files) {
		assetURL, err := githubraw.BuildAssetURL(req.Repository, req.SHA, file.Path)
		if err != nil {
			return ValidationResult{}, mapProtocolValidationError(err), mapValidationStatus(err)
		}
		assetResult, err := p.Fetcher.Fetch(ctx, assetURL, AssetMaxBytes)
		if err != nil {
			return ValidationResult{}, mapFetchError(err, false), mapFetchStatus(err)
		}
		if assetResult.Size > AssetMaxBytes {
			return ValidationResult{}, errorObject("ASSET_TOO_LARGE", "asset exceeds size limit", map[string]any{"path": file.Path}), http.StatusRequestEntityTooLarge
		}
		totalBytes += assetResult.Size
		if totalBytes > PackageMaxBytes {
			return ValidationResult{}, errorObject("PACKAGE_TOO_LARGE", "package exceeds size limit", nil), http.StatusRequestEntityTooLarge
		}
		sum := sha256.Sum256(assetResult.Body)
		format, err := protocol.FormatFromPath(file.Path)
		if err != nil {
			return ValidationResult{}, mapProtocolValidationError(err), mapValidationStatus(err)
		}
		assets = append(assets, ValidatedAsset{
			Path:      file.Path,
			URL:       assetURL,
			SHA256:    hex.EncodeToString(sum[:]),
			Format:    format,
			Style:     file.Style,
			Weight:    file.Weight,
			SizeBytes: assetResult.Size,
		})
	}

	return ValidationResult{
		PackageID:   strings.ToLower(req.Repository),
		Version:     manifest.Version,
		VersionKey:  manifestVersionKey,
		ManifestURL: manifestURL,
		Manifest:    manifest,
		Assets:      assets,
	}, nil, http.StatusOK
}

func errorObject(code, message string, details map[string]any) *protocol.ErrorObject {
	if details == nil {
		details = map[string]any{}
	}
	return &protocol.ErrorObject{Code: code, Message: message, Details: details}
}

func internalError(message string) *protocol.ErrorObject {
	return errorObject("INTERNAL_ERROR", message, nil)
}

func mapFetchError(err error, manifest bool) *protocol.ErrorObject {
	switch err {
	case githubraw.ErrNotFound:
		return errorObject("UPSTREAM_NOT_FOUND", "upstream object not found", nil)
	case githubraw.ErrTooLarge:
		if manifest {
			return errorObject("MANIFEST_TOO_LARGE", "manifest exceeds size limit", nil)
		}
		return errorObject("ASSET_TOO_LARGE", "asset exceeds size limit", nil)
	default:
		return errorObject("UPSTREAM_FETCH_FAILED", "failed to fetch upstream object", nil)
	}
}

func mapFetchStatus(err error) int {
	switch err {
	case githubraw.ErrNotFound:
		return http.StatusNotFound
	case githubraw.ErrTooLarge:
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusBadGateway
	}
}

func mapProtocolValidationError(err error) *protocol.ErrorObject {
	message := err.Error()
	code := strings.TrimSpace(strings.SplitN(message, ":", 2)[0])
	switch code {
	case "LICENSE_NOT_ALLOWED", "VERSION_INVALID", "ASSET_PATH_INVALID", "ASSET_FORMAT_NOT_ALLOWED", "ASSET_DUPLICATE_PATH":
		return errorObject(code, message, nil)
	default:
		return errorObject("MANIFEST_SCHEMA_INVALID", message, nil)
	}
}

func mapValidationStatus(err error) int {
	code := strings.TrimSpace(strings.SplitN(err.Error(), ":", 2)[0])
	switch code {
	case "LICENSE_NOT_ALLOWED", "VERSION_INVALID", "ASSET_PATH_INVALID", "ASSET_FORMAT_NOT_ALLOWED", "ASSET_DUPLICATE_PATH":
		return http.StatusUnprocessableEntity
	default:
		return http.StatusUnprocessableEntity
	}
}
