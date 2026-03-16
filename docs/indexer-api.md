# Indexer HTTP API — v1

Base URL: `https://fontpub.org`

All endpoints are under `/v1`.

## Common headers (recommended)

- Responses SHOULD include `Content-Type: application/json; charset=utf-8`
- Read endpoints SHOULD include `ETag`
- Read endpoints SHOULD support `If-None-Match` and return `304 Not Modified`

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

### Caching
- MUST include `ETag`
- SHOULD include `Cache-Control: public, max-age=60`

### Responses
- `200 OK` with index JSON
- `304 Not Modified` if `If-None-Match` matches

## GET /v1/packages/{owner}/{repo}.json

Returns the package detail document.

### Caching
Same as root index.

### Responses
- `200 OK` with package detail JSON
- `404 Not Found` if package does not exist
- `304 Not Modified` if `If-None-Match` matches

## POST /v1/update

Notarizes a package version by fetching its manifest and assets at a specific commit SHA and publishing updated indexes.

### Authentication
- Requires `Authorization: Bearer <GitHub OIDC JWT>`
- JWT validation rules are specified in `security-oidc.md`

### Request body (recommended)

```json
{
  "repository": "owner/repo",
  "sha": "40-hex",
  "ref": "refs/tags/v1.2.3",
  "trigger": "tag"
}
```

The Indexer MUST verify that `repository`, `sha`, and `ref` match the corresponding JWT claims.

### Processing rules (v1)
- The Indexer fetches:
  - `fontpub.json` at the pinned `sha`
  - Each file declared in `fontpub.json`
- The Indexer computes SHA-256 and (recommended) `size_bytes` for each asset.
- The Indexer MUST reject:
  - files > 50 MiB
  - non-allowed extensions (`.otf`, `.ttf`, `.woff2`)
  - invalid manifest schema
  - immutability violations (see `immutability.md`)
- The Indexer MUST update:
  - Package detail document for `{owner}/{repo}`
  - Root index document

### Concurrency

The root index update MUST be atomic under concurrency.

Implementations SHOULD use conditional writes (ETag / if-match semantics) with limited retries.

If the Indexer cannot complete a write due to repeated contention, it SHOULD return:
- `503 Service Unavailable`
- With `Retry-After: 1`
- With `error.code: INDEX_CONFLICT`

### Success response (recommended)

`200 OK`

```json
{
  "status": "ok",
  "package_id": "owner/repo",
  "version": "1.2.3",
  "github_sha": "40-hex",
  "index_etag": "string",
  "package_etag": "string"
}
```

### Error responses

- `400 Bad Request` — malformed body or body/JWT mismatch
- `401 Unauthorized` — missing/invalid JWT, signature failure
- `403 Forbidden` — ownership mismatch, policy restrictions
- `404 Not Found` — manifest or assets not found at the pinned SHA
- `409 Conflict` — immutability violation
- `413 Payload Too Large` — asset > 50 MiB
- `422 Unprocessable Entity` — manifest validation failure or unsupported asset format
- `429 Too Many Requests` — rate limited
- `502 Bad Gateway` — upstream (GitHub Raw) fetch failure
- `503 Service Unavailable` — contention after retries; includes `Retry-After`

See `error-codes.md` for canonical `error.code` values.
