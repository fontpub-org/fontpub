# AGENTS.md (Codex Implementation Guide)

This file is an implementation guide for coding agents.

Protocol, wire behavior, and architecture are defined in `docs/`. If this file conflicts with `docs/`, follow `docs/`.

Primary references:
- `docs/overview.md`
- `docs/architecture.md`
- `docs/indexes.md`
- `docs/indexer-api.md`
- `docs/security-oidc.md`
- `docs/manifest-fontpub-json.md`
- `docs/versioning.md`
- `docs/immutability.md`
- `docs/cli.md`
- `docs/cli-json.md`
- `docs/quickstart.md`

This file covers:
- change workflow
- TDD expectations
- acceptance criteria for code changes
- “do not guess” rules

---

## 0. Non-negotiables

### MUST
- Follow `docs/` exactly.
- Update `docs/` and tests together when changing protocol behavior, JSON schemas, paths, error codes, or architecture-affecting behavior.

### MUST NOT
- Do not invent public behavior that is not documented.
- Do not broaden OIDC acceptance beyond `docs/security-oidc.md`.
- Do not weaken tests to make them pass.

### SHOULD
- Prefer small, composable packages with deterministic tests.
- Add fixtures and golden files for every non-trivial edge case.

---

## 1. Change workflow

Before editing code, identify which surface you are changing and read the matching docs first.

- Protocol documents and artifact structure:
  `docs/overview.md`, `docs/architecture.md`, `docs/indexes.md`, `docs/versioning.md`, `docs/immutability.md`
- Update API and OIDC behavior:
  `docs/indexer-api.md`, `docs/security-oidc.md`, `docs/error-codes.md`
- Manifest and publisher-side behavior:
  `docs/manifest-fontpub-json.md`, `docs/publisher-workflow.md`, `docs/candidate-package-detail.md`
- CLI command behavior and machine-readable output:
  `docs/cli.md`, `docs/cli-json.md`
- Local usage and operator setup:
  `docs/quickstart.md`

When making changes:
- update `docs/` first or alongside code when public behavior changes
- update schemas, fixtures, and golden files under `protocol/` when the protocol surface changes
- keep shared deterministic logic in `go/internal/protocol` when possible
- keep derived-document generation consistent between the Update API and Rebuilder
- add or update tests before considering the change done

---

## 2. TDD rules

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

## 3. “Do not guess” list

If any of the following are unclear, do not invent behavior. Add or update a test and then update `docs/`:
- any public JSON field name or type
- any URL path
- any error code
- any OIDC claim requirement
- any resource limit
- any `ETag` behavior
- any rule for authoritative vs derived artifacts

---

## 4. Developer ergonomics

- Keep local operator and usage instructions in `docs/quickstart.md`.
- Provide a single command to run all protocol and Go tests.
- Prefer fixture-driven local development over manual endpoint testing.

---

## 5. Definition of Done

A feature is done only when:
- tests cover success and failure modes
- docs remain consistent with the implementation intent
- authoritative and derived artifact behavior matches the protocol docs
- rebuildability from published versioned package detail documents is preserved
