# Error Codes — v1

This document defines canonical `error.code` values used by:
- the Indexer API
- CLI JSON error output

Only the codes in this document are valid in v1.

## Scope

- Codes in the `Indexer API` sections are used by HTTP responses from the Indexer.
- Codes in the `CLI` section are used by `fontpub --json` failure output.

## Authentication / Authorization

- `AUTH_REQUIRED` — missing Authorization header
- `AUTH_INVALID_TOKEN` — JWT invalid or signature verification failed
- `AUTH_REPLAY_DETECTED` — JWT `jti` was already used while the token remained valid
- `AUTH_CLAIMS_MISSING` — required claims missing
- `AUTH_CLAIMS_MISMATCH` — request body does not match JWT claims
- `WORKFLOW_NOT_ALLOWED` — token context not allowed by policy (e.g., not a tag ref)
- `OWNERSHIP_MISMATCH` — `repository_id` does not match the stored owner for package

## Request / Lookup

- `REQUEST_INVALID_JSON` — request body is not valid JSON
- `REQUEST_SCHEMA_INVALID` — request body is missing required fields or contains unexpected fields
- `PACKAGE_NOT_FOUND` — requested package does not exist
- `VERSION_NOT_FOUND` — requested package version does not exist

## Validation

- `MANIFEST_INVALID_JSON` — `fontpub.json` is not valid JSON
- `MANIFEST_TOO_LARGE` — `fontpub.json` exceeds the maximum allowed size
- `MANIFEST_SCHEMA_INVALID` — missing/invalid required fields
- `LICENSE_NOT_ALLOWED` — license is not `OFL-1.1`
- `VERSION_INVALID` — version does not meet Numeric Dot rules
- `TAG_VERSION_MISMATCH` — release tag version does not match the manifest version key
- `ASSET_PATH_INVALID` — invalid path (absolute, contains `..`, etc.)
- `ASSET_FORMAT_NOT_ALLOWED` — unsupported extension
- `ASSET_COUNT_LIMIT_EXCEEDED` — manifest declares too many asset entries
- `ASSET_TOO_LARGE` — single asset exceeds size limit
- `PACKAGE_TOO_LARGE` — the total size of assets in one package exceeds the allowed limit
- `ASSET_DUPLICATE_PATH` — manifest has duplicate file paths

## Immutability / Conflicts

- `IMMUTABLE_VERSION` — same version key already exists but differs in immutable fields
- `INDEX_CONFLICT` — root index write conflict after retries

## Upstream / Infrastructure

- `UPSTREAM_NOT_FOUND` — manifest or asset not found at SHA
- `UPSTREAM_FETCH_FAILED` — network or upstream error fetching from GitHub Raw
- `RATE_LIMITED` — too many requests
- `INTERNAL_ERROR` — unexpected server error

## CLI

- `TTY_REQUIRED` — the command requires interactive input but no TTY is available
- `INPUT_REQUIRED` — required CLI input was not provided
- `LOCKFILE_INVALID` — the local lockfile is missing required fields or is malformed
- `LOCAL_FILE_MISSING` — an expected installed asset file is missing
- `LOCAL_FILE_HASH_MISMATCH` — an installed asset file does not match the expected SHA-256
- `ACTIVATION_BROKEN` — activation symlink state does not match the lockfile or expected target
- `NOT_INSTALLED` — the requested package or version is not installed locally
- `MULTIPLE_VERSIONS_INSTALLED` — an operation required an explicit version because multiple installed versions matched
- `PACKAGE_ID_REQUIRED` — the CLI could not derive a canonical GitHub package ID and requires `--package-id`
- `PACKAGE_ID_AMBIGUOUS` — the CLI found multiple distinct GitHub package IDs and requires `--package-id`
