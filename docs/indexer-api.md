# Indexer HTTP API — v1

Base URL: `https://fontpub.org`

All endpoints are under `/v1`.

## Common headers

- Responses MUST include `Content-Type: application/json; charset=utf-8`
- Read endpoints MUST include a strong `ETag`
- Read endpoints MUST support `If-None-Match` and return `304 Not Modified`

`ETag` requirements:
- The `ETag` value MUST validate the exact response bytes.
- Producers MUST compute `ETag` values from the canonical JSON serialization defined in `indexes.md`.
- Clients MUST treat `ETag` as opaque.

## Error object

All error responses MUST use:

```json
{
  "error": {
    "code": "STRING_ENUM",
    "message": "Human readable summary",
    "details": {}
  }
}
```

## GET /v1/index.json

Returns the root index document (`indexes.md`).

### Responses
- `200 OK` with index JSON
- `304 Not Modified` if `If-None-Match` matches

## GET /v1/packages/{owner}/{repo}.json

Returns the latest package detail alias.

### Responses
- `200 OK` with package detail JSON
- `404 Not Found` with `error.code: PACKAGE_NOT_FOUND` if package does not exist
- `304 Not Modified` if `If-None-Match` matches

## GET /v1/packages/{owner}/{repo}/index.json

Returns the package versions index document.

### Responses
- `200 OK` with package versions index JSON
- `404 Not Found` with `error.code: PACKAGE_NOT_FOUND` if package does not exist
- `304 Not Modified` if `If-None-Match` matches

## GET /v1/packages/{owner}/{repo}/versions/{version_key}.json

Returns the immutable versioned package detail document for `version_key`.

### Responses
- `200 OK` with package detail JSON
- `404 Not Found` with `error.code: VERSION_NOT_FOUND` if the package exists but that version does not
- `404 Not Found` with `error.code: PACKAGE_NOT_FOUND` if package does not exist
- `304 Not Modified` if `If-None-Match` matches

## POST /v1/update

Notarizes a package version by fetching its manifest and assets at a specific commit SHA and publishing updated indexes.

### Authentication
- Requires `Authorization: Bearer <GitHub OIDC JWT>`
- JWT validation rules are specified in `security-oidc.md`

### Request body

```json
{
  "repository": "owner/repo",
  "sha": "40-hex",
  "ref": "refs/tags/v1.2.3"
}
```

Request rules:
- The request body MUST be a JSON object with exactly the fields above.
- `repository`, `sha`, and `ref` are all required.
- The Indexer MUST verify that `repository`, `sha`, and `ref` match the corresponding JWT claims.

### Processing rules (v1)
- The Indexer fetches:
  - `fontpub.json` at the pinned `sha`
  - Each file declared in `fontpub.json`
- The Indexer validates that the tag name in `ref` is a valid Numeric Dot version string and that its version key equals the manifest version key.
- The Indexer computes SHA-256 for each asset.
- The Indexer MUST reject:
  - files > 50 MiB
  - non-allowed extensions (`.otf`, `.ttf`, `.woff2`)
  - invalid manifest schema
  - tag/version mismatches
  - immutability violations (see `immutability.md`)
- The Indexer MUST update:
  - Versioned package detail document for `{owner}/{repo}` and `version_key`
  - Package versions index document for `{owner}/{repo}`
  - Latest package detail alias for `{owner}/{repo}` if the published version is now latest
  - Root index document

### Concurrency and consistency

Consistency rules:
- The versioned package detail document is the authoritative immutable record.
- The package versions index, latest package detail alias, and root index are derived documents.
- A version MUST NOT become discoverable through derived documents before its versioned package detail document is available.
- If an update attempt fails after the versioned package detail document is written, retrying the same request MUST NOT alter that immutable document and MUST be sufficient to restore derived-document consistency.

If the Indexer cannot complete an update because it cannot preserve these consistency requirements, it MUST return:
- `503 Service Unavailable`
- With `Retry-After: 1`
- With `error.code: INDEX_CONFLICT`

### Success response

`200 OK`

```json
{
  "status": "ok",
  "package_id": "owner/repo",
  "version": "1.2.3",
  "version_key": "1.2.3",
  "github_sha": "40-hex",
  "index_etag": "string",
  "package_etag": "string",
  "package_version_etag": "string"
}
```

### Error responses

- `400 Bad Request` — malformed body, invalid body schema, or body/JWT mismatch
- `401 Unauthorized` — missing/invalid JWT, signature failure
- `403 Forbidden` — ownership mismatch or workflow restrictions
- `404 Not Found` — manifest or assets not found at the pinned SHA
- `409 Conflict` — immutability violation
- `413 Payload Too Large` — asset > 50 MiB
- `422 Unprocessable Entity` — manifest validation failure, unsupported asset format, or tag/version mismatch
- `429 Too Many Requests` — rate limited
- `502 Bad Gateway` — upstream (GitHub Raw) fetch failure
- `503 Service Unavailable` — contention after retries; includes `Retry-After`

See `error-codes.md` for canonical `error.code` values.
