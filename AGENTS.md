# AGENTS.md (Codex Implementation Guide)

This repository defines **Fontpub v1** as a public-artifact-first font distribution protocol.

The implementation goal is:
- an **Update API** that authenticates GitHub Actions OIDC update requests, fetches manifests and assets at SHA-pinned URLs, validates them, and publishes immutable metadata
- a **Rebuilder** that regenerates derived indexes from already-published immutable metadata
- a **CLI** that installs, verifies, and activates fonts from the public indexes
- a shared **protocol** directory containing schemas, fixtures, golden JSON, and conformance tests

This file is the implementation guide for coding agents. It is the source of truth for:
- architecture and component boundaries
- recommended repository layout
- implementation order
- TDD expectations
- acceptance criteria
- “do not guess” rules

---

## 0. Non-negotiables

### MUST
- Follow `docs/` exactly.
- Treat the public **versioned package detail** document as the authoritative record for a published package version.
- Treat the package versions index, latest package detail alias, and root index as **derived documents**.
- Ensure derived documents are rebuildable from published versioned package detail documents.
- Keep private state minimal:
  - ownership binding
  - JWT replay protection
  - transient coordination needed to publish safely
- `fontpub.json` fields are all required.
- `license` is only `"OFL-1.1"`.
- Version strings are Numeric Dot and must not contain leading zeros in any non-zero segment.
- Do not store font binaries in Fontpub-managed storage.

### MUST NOT
- Do not change on-wire JSON schemas, paths, or error codes without updating `docs/` and tests.
- Do not broaden OIDC acceptance beyond `docs/security-oidc.md`.
- Do not make private state the authoritative source for published package metadata.
- Do not require unrecoverable internal state in order to serve already-published metadata.

### SHOULD
- Prefer small, composable packages with deterministic tests.
- Add fixtures and golden files for every non-trivial edge case.
- Prefer append-only publication flows.

---

## 1. Recommended architecture

Fontpub should be implemented as a **public-artifact-first append-only system**.

### Authoritative public artifacts
- `/v1/packages/{owner}/{repo}/versions/{version_key}.json` is the authoritative public record.

### Derived public artifacts
- `/v1/packages/{owner}/{repo}/index.json`
- `/v1/packages/{owner}/{repo}.json`
- `/v1/index.json`

These are derived from the authoritative versioned package detail documents and must be reproducible.

### Private state
Private state exists only to protect publication:
- `package_id -> repository_id` ownership binding
- used `jti` values for replay prevention
- transient publish coordination if needed

Loss of private state must not invalidate already-published versioned package detail documents.

### Runtime roles
- **Update API**
  - validates GitHub OIDC
  - fetches manifest and assets at pinned GitHub Raw URLs
  - computes SHA-256
  - enforces validation, immutability, and resource limits
  - publishes authoritative versioned package detail documents
  - updates derived documents
- **Rebuilder**
  - scans published versioned package detail documents
  - regenerates package versions indexes, latest aliases, and the root index
  - is safe to run repeatedly
- **CLI**
  - reads public metadata
  - downloads assets from pinned upstream URLs
  - verifies asset hashes
  - installs and activates fonts locally

### Deployment shape
- Serve public JSON from object storage plus CDN.
- Run the Update API as a regional service, not as a read-path dependency.
- Keep the Rebuilder independent from request handling so derived documents can be repaired out of band.

---

## 2. Recommended repository layout

The repository should be organized around the protocol and the append-only publication model.

```text
.
├─ AGENTS.md
├─ README.md
├─ docs/                            # authoritative protocol/spec docs
├─ protocol/
│  ├─ schemas/                      # JSON Schemas for public documents
│  ├─ fixtures/                     # manifests, JWT claim sets, golden indexes, errors
│  ├─ golden/                       # canonical JSON outputs for conformance tests
│  └─ README.md
├─ go/
│  ├─ go.mod
│  ├─ cmd/
│  │  ├─ fontpub/                   # CLI
│  │  ├─ fontpub-indexer/           # POST /v1/update service
│  │  └─ fontpub-rebuilder/         # derived-document rebuilder
│  └─ internal/
│     ├─ cli/                       # CLI config, metadata client, lockfile, commands
│     ├─ protocol/                  # versioning, canonical JSON, validation helpers
│     └─ indexer/
│        ├─ artifacts/              # public JSON storage backends
│        ├─ derive/                 # shared derived-document generation
│        ├─ githubraw/              # pinned URL fetch logic
│        ├─ httpx/                  # HTTP response helpers
│        ├─ oidc/                   # JWT verification
│        ├─ rebuilder/              # rebuild orchestration
│        ├─ state/                  # ownership and replay state abstractions
│        └─ updateapi/              # immutable publication flow
└─ tools/
   └─ scripts/                      # release helpers, fixture generation, local checks
```

Notes:
- Use one shared Go module so the CLI, Update API, and Rebuilder can share versioning, hashing, canonicalization, and protocol logic.
- Keep `protocol/` language-neutral where possible. It should remain usable by other implementations.
- If a website or docs app is added later, it should not own protocol logic.

---

## 3. Implementation order (TDD-first)

Implement in this order:

### Phase A: Protocol assets and conformance
**Directory:** `protocol/`

Deliverables:
- JSON Schemas for:
  - manifest
  - root index
  - package versions index
  - versioned package detail
  - lockfile
  - error object
  - CLI JSON envelope
  - CLI command result objects used by `list`, `status`, `verify`, `repair`, `package init`, and `package preview`
- fixtures for:
  - valid/invalid manifests
  - valid/invalid OIDC claim sets
  - valid/invalid version strings
  - immutability comparisons
  - valid/invalid CLI JSON results
- golden canonical JSON outputs

Tests:
- fixture-driven schema validation
- version key and ordering tests
- canonical JSON serialization tests

Acceptance:
- Public documents, CLI JSON results, and error objects can be validated without referring to implementation code.

### Phase B: Shared Go protocol library
**Directory:** `go/internal/protocol`

Deliverables:
- Numeric Dot parsing and `version_key` normalization
- manifest validation helpers
- canonical JSON serializer
- immutability comparison helpers
- path validation helpers

Tests:
- table-driven version tests
- manifest/path validation tests
- golden serialization tests
- immutability comparison tests

Acceptance:
- Shared protocol logic is implementation-agnostic and deterministic.

### Phase C: Update API
**Directory:** `go/cmd/fontpub-indexer`

Deliverables:
- `POST /v1/update`
- OIDC validation per `docs/security-oidc.md`
- request validation per `docs/indexer-api.md`
- ownership binding
- `jti` replay protection
- manifest and asset fetching at SHA-pinned URLs
- SHA-256 calculation
- publication of versioned package detail documents
- update of derived documents

Tests:
- JWT claim validation tests
- replay rejection tests
- manifest and asset fetch tests with mocked upstream responses
- immutability tests
- error contract tests
- consistency tests covering partial publication and retry repair

Acceptance:
- The Update API can publish immutable versioned package detail documents and recover derived documents after retry.

### Phase D: Rebuilder
**Directory:** `go/cmd/fontpub-rebuilder`

Deliverables:
- full rebuild from published versioned package detail documents
- package-scoped rebuild
- root index regeneration
- latest alias regeneration

Tests:
- rebuild from golden published artifacts
- idempotent rerun tests
- latest-version precedence tests

Acceptance:
- Deleting derived documents and rerunning the rebuilder restores them exactly.

### Phase E: CLI
**Directory:** `go/cmd/fontpub`

Deliverables:
- `fontpub list`
- `fontpub install`
- `fontpub activate`
- `fontpub deactivate`
- `fontpub update`
- `fontpub uninstall`
- `fontpub status`
- lockfile read/write and repair-safe local state handling

Tests:
- lockfile parsing/formatting
- download verification
- activation naming and collision handling
- temp-directory integration tests for install/activate/deactivate/uninstall
- CLI JSON conformance tests against protocol schemas

Acceptance:
- CLI commands are deterministic and idempotent.
- Corrupted installs are detected and reported.
- `--json` output validates against the corresponding protocol schema.

---

## 4. TDD rules

### Shared protocol and Update API
- Every new module must ship with:
  - one happy-path test
  - one invalid-input test
  - one edge-case or fixture-driven test

### Go code
- Prefer table-driven tests.
- Use `t.TempDir()` for filesystem tests.
- Do not mutate the real home directory in tests.

### Rebuilder
- Golden tests are required.
- Rebuild outputs must be compared byte-for-byte against canonical JSON fixtures.

### Quality gates
- Run `go test ./...` for the Go module.
- Keep conformance tests for protocol fixtures separate from service tests.
- Do not weaken assertions to make tests pass.

---

## 5. Security acceptance criteria

### OIDC
- Accept only GitHub Actions OIDC tokens with:
  - `iss = https://token.actions.githubusercontent.com`
  - `aud` containing `https://fontpub.org`
  - required claims exactly as specified in `docs/security-oidc.md`
- Enforce:
  - tag-only publication
  - workflow-file restriction
  - allowed event restriction
  - `jti` replay prevention

### Ownership
- First successful publication binds `package_id` to `repository_id`.
- Subsequent publications require the same `repository_id`.

### Resource limits
- Reject:
  - manifests larger than 1 MiB
  - manifests with more than 256 assets
  - packages larger than 2 GiB total
  - assets larger than 50 MiB

### Publication safety
- A version must not become discoverable before its authoritative versioned package detail document exists.
- Retrying the same valid update must not alter the immutable versioned package detail document.

---

## 6. Determinism requirements

- Public JSON outputs must be byte-stable for the same logical document.
- Use canonical JSON serialization exactly as defined in `docs/indexes.md`.
- Sort `assets[]` by `path`.
- Sort `versions[]` by version precedence descending.
- Rebuilder output must match Update API output byte-for-byte.

---

## 7. “Do not guess” list

If any of the following are unclear, do not invent behavior. Add or update a test and then update `docs/`:
- any public JSON field name or type
- any URL path
- any error code
- any OIDC claim requirement
- any resource limit
- any `ETag` behavior
- any rule for authoritative vs derived artifacts

---

## 8. Developer ergonomics

- Provide a local command to run the Update API.
- Provide a local command to run the Rebuilder against a development object store.
- Provide a single command to run all protocol and Go tests.
- Prefer fixture-driven local development over manual endpoint testing.

Current local operator commands:
- `FONTPUB_ARTIFACTS_DIR=/path/to/artifacts go run ./cmd/fontpub-rebuilder`
- `FONTPUB_ARTIFACTS_DIR=/path/to/artifacts FONTPUB_GITHUB_JWKS_JSON='{\"keys\":[...]}' go run ./cmd/fontpub-indexer`

---

## 9. Definition of Done

A feature is done only when:
- tests cover success and failure modes
- docs remain consistent with the implementation intent
- authoritative and derived artifact behavior matches the protocol docs
- rebuildability from published versioned package detail documents is preserved
