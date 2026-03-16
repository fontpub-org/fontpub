# AGENTS.md (Codex Implementation Guide)

This repository is a **monorepo** managed with **pnpm workspaces**. The goal is to implement **Fontpub v1**:
- An **Indexer** (Cloudflare Workers) that notarizes packages (manifest + asset hashes) and publishes **Split Index** JSON to R2.
- A **CLI** (Go, macOS) that installs/verifies/activates fonts based on the Indexer’s public indexes.
- A shared set of **schemas** and **fixtures** to keep protocol compatibility.

This file is written for coding agents (Codex) and is the source of truth for:
- repo structure & boundaries
- implementation order
- TDD expectations
- acceptance criteria
- “do not guess” rules

---

## 0. Non-negotiables

### MUST
- Follow the specs in `docs/` exactly (v1).
- Treat **Split Index** as canonical. Ignore any legacy index description elsewhere.
- `fontpub.json` fields are **all required**.
- `license` is **only** `"OFL-1.1"`.
- Version strings must be **Numeric Dot** and **must not** contain leading zeros in any segment (e.g. `01.2` invalid; `0.100` valid).

### MUST NOT
- Do not change on-wire JSON schemas or error codes without updating `docs/` and tests.
- Do not broaden security acceptance (OIDC) beyond what `docs/security-oidc.md` specifies.
- Do not store font binaries in the Indexer (only index JSON).

### SHOULD
- Prefer small, composable modules with deterministic unit tests.
- Add fixtures for any non-trivial edge case.

---

## 1. Monorepo layout (recommended)

```
.
├─ AGENTS.md
├─ docs/                          # v1 spec (authoritative)
├─ pnpm-workspace.yaml
├─ package.json                   # workspace root
├─ packages/
│  ├─ indexer/                    # Cloudflare Workers (TypeScript)
│  │  ├─ src/
│  │  ├─ test/
│  │  ├─ wrangler.toml
│  │  └─ package.json
│  ├─ schemas/                    # JSON Schema / validators / canonical normalization
│  │  ├─ src/
│  │  ├─ test/
│  │  └─ package.json
│  ├─ fixtures/                   # shared test fixtures (manifests, indexes, jwt samples)
│  │  └─ ...
│  └─ cli/                        # Go module (macOS CLI)
│     ├─ cmd/fontpub/
│     ├─ internal/
│     ├─ testdata/
│     └─ go.mod
└─ tools/
   └─ scripts/                    # release helpers, lint wrappers, etc.
```

Notes:
- `packages/cli` is a Go module; it is not managed by pnpm. It lives in the monorepo for cohesion.
- `packages/schemas` is published/consumed by `packages/indexer` to prevent drift between docs and implementation.

---

## 2. Package management (pnpm)

### Workspace configuration
- Use a minimal workspace root:
  - `pnpm-workspace.yaml` includes `packages/indexer`, `packages/schemas`, and optionally `tools/*`.
- Keep Node dependencies pinned via lockfile (`pnpm-lock.yaml`).

### TypeScript standards
- Node packages are ESM unless there is a strong reason not to.
- Use `vitest` for unit tests where possible.

---

## 3. Implementation order (TDD-first)

Implement in this order to keep feedback loops tight:

### Phase A: Schemas & normalization (TDD)
**Package:** `packages/schemas`

Deliverables:
- Version parsing & comparison:
  - Numeric Dot parser
  - Leading-zero rejection per segment
  - Canonical formatting rules (do not rewrite the user’s string on wire; canonicalize for comparisons)
- Manifest validation:
  - All required fields
  - `license === "OFL-1.1"` only
  - `files[]` validation (path rules, style/weight ranges)
- Index canonicalization:
  - Sorting rules (`assets` sorted by `path` asc)
  - Deterministic comparison utilities for immutability checks

Tests:
- Table-driven tests for version compare edge cases.
- JSON fixtures for valid/invalid manifest examples.
- Golden tests for canonicalized asset lists.

Acceptance:
- `pnpm -C packages/schemas test` passes with >90% branch coverage on core modules.

### Phase B: Indexer API core (TDD)
**Package:** `packages/indexer`

Deliverables:
- Public GET endpoints:
  - `GET /v1/index.json`
  - `GET /v1/packages/:owner/:repo.json`
  - Must include `ETag` and support `If-None-Match` -> `304`
- Update endpoint:
  - `POST /v1/update`
  - Validates GitHub OIDC per `docs/security-oidc.md`
  - Fetches `fontpub.json` at **SHA-pinned** raw URL
  - Fetches each asset at SHA-pinned raw URL
  - Computes SHA-256 (streaming) and enforces file size limit
  - Writes package detail JSON to R2
  - Updates root index using conditional write / optimistic concurrency
  - Enforces immutability (409 on mismatch)

Tests:
- Unit tests for:
  - JWT claim validation logic (pure functions, no network)
  - Manifest retrieval URL building (SHA pinned)
  - Asset hashing pipeline (streamed hashing)
  - Immutability comparator (via schemas package)
- Integration-ish tests with mocked fetch:
  - Simulate GitHub raw responses and failures
  - Simulate R2 get/put semantics and ETag conflicts
- Error contract tests:
  - Verify HTTP status + `{ error: { code, message, details } }`

Acceptance:
- All endpoints behave per `docs/indexer-api.md` and `docs/error-codes.md`.
- 304 caching behavior verified by tests.

### Phase C: CLI (Go) (TDD)
**Package:** `packages/cli`

Deliverables:
- `fontpub list` (reads root index with ETag)
- `fontpub install owner/repo` (fetches package detail, downloads assets, verifies sha256)
- `fontpub activate owner/repo` (symlink into `~/Library/Fonts/from_fontpub/`)
- `fontpub deactivate`, `uninstall`, `update`, `status`
- Lockfile read/write with atomic updates

Tests:
- Unit tests for:
  - lockfile parsing/formatting
  - download verification
  - symlink naming and collision avoidance
- Integration tests using temp directories to simulate `~/.fontpub` and `~/Library/Fonts/from_fontpub/`

Acceptance:
- CLI commands are deterministic and idempotent.
- Corrupted installs must be detected and reported.

---

## 4. TDD rules (required)

### For Node/TS packages
- Every new module should ship with:
  - at least one “happy path” test
  - at least one “invalid input” test
  - at least one “edge case” test (fixture-based)

### For Go (CLI)
- Prefer table-driven tests.
- Use `t.TempDir()` for filesystem operations.
- Avoid tests that mutate the real home directory.

### Coverage / quality gates
- Add a CI job that runs:
  - Node: `pnpm -r test`
  - Go: `go test ./...`
- The agent should not “greenwash” by reducing assertions. Fail tests only when the spec changes.

---

## 5. Security acceptance criteria (Indexer)

### OIDC
- Accept only GitHub Actions OIDC:
  - `iss` fixed to GitHub issuer
  - `aud` must match `https://fontpub.org`
  - `exp`, `iat` checks with clock skew
  - required claims: `sub`, `repository`, `sha`, `ref` (+ recommended owner consistency)
- Enforce release-tag-only updates:
  - `ref` must match `refs/tags/v*` (v1 rule)

### Repository ownership
- First update claims ownership: store `sub` for `owner/repo`.
- Subsequent updates require exact `sub` match.

### DoS and resource limits
- Reject assets > 50MB with 413.
- Limit concurrent asset fetch+hash within an update request.

---

## 6. Error handling contract (Indexer)

All errors MUST use:

```json
{
  "error": {
    "code": "ENUM",
    "message": "string",
    "details": { }
  }
}
```

Status mapping MUST follow `docs/indexer-api.md` and `docs/error-codes.md`:
- 401: missing/invalid token
- 403: ownership/workflow/ref restriction failures
- 409: immutability violation
- 422: manifest validation
- 429: rate limit
- 502/503: upstream failures, include retry guidance where applicable

---

## 7. Determinism requirements

- Indexer JSON outputs must be stable across runs given the same inputs.
- Always sort `assets[]` by `path` ascending.
- Use canonical JSON serialization (stable key order if possible) for immutability comparisons.

---

## 8. “Do not guess” list

If any of the following are unclear in implementation, DO NOT invent behavior. Add a failing test and update `docs/`:
- Any on-wire JSON field name or type
- Any error code
- Any OIDC claim requirement
- Any cache/ETag behavior

---

## 9. Developer ergonomics (optional but recommended)

- Provide `pnpm dev` for indexer local dev (wrangler).
- Provide `pnpm test` at repo root.
- Add `tools/scripts/` for release tasks (tagging, manifest validation preflight).

---

## 10. Definition of Done

A feature is done only when:
- Tests cover it (including failure modes).
- Docs remain consistent (no schema drift).
- The implementation matches the error contract and sorting/canonicalization rules.
