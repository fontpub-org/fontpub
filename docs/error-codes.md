# Error Codes — v1

This document defines canonical `error.code` values used by the Indexer API.

Implementations MAY add additional codes, but SHOULD keep these stable.

## Authentication / Authorization

- `AUTH_REQUIRED` — missing Authorization header
- `AUTH_INVALID_TOKEN` — JWT invalid or signature verification failed
- `AUTH_CLAIMS_MISSING` — required claims missing
- `AUTH_CLAIMS_MISMATCH` — request body does not match JWT claims
- `WORKFLOW_NOT_ALLOWED` — token context not allowed by policy (e.g., not a tag ref)
- `OWNERSHIP_MISMATCH` — `sub` does not match stored owner for package

## Request / Lookup

- `REQUEST_INVALID_JSON` — request body is not valid JSON
- `REQUEST_SCHEMA_INVALID` — request body is missing required fields or contains unexpected fields
- `PACKAGE_NOT_FOUND` — requested package does not exist
- `VERSION_NOT_FOUND` — requested package version does not exist

## Validation

- `MANIFEST_INVALID_JSON` — `fontpub.json` is not valid JSON
- `MANIFEST_SCHEMA_INVALID` — missing/invalid required fields
- `LICENSE_NOT_ALLOWED` — license is not `OFL-1.1`
- `VERSION_INVALID` — version does not meet Numeric Dot rules
- `TAG_VERSION_MISMATCH` — release tag version does not match the manifest version key
- `ASSET_PATH_INVALID` — invalid path (absolute, contains `..`, etc.)
- `ASSET_FORMAT_NOT_ALLOWED` — unsupported extension
- `ASSET_TOO_LARGE` — single asset exceeds size limit
- `ASSET_DUPLICATE_PATH` — manifest has duplicate file paths

## Immutability / Conflicts

- `IMMUTABLE_VERSION` — same version key already exists but differs in immutable fields
- `INDEX_CONFLICT` — root index write conflict after retries

## Upstream / Infrastructure

- `UPSTREAM_NOT_FOUND` — manifest or asset not found at SHA
- `UPSTREAM_FETCH_FAILED` — network or upstream error fetching from GitHub Raw
- `RATE_LIMITED` — too many requests
- `INTERNAL_ERROR` — unexpected server error
