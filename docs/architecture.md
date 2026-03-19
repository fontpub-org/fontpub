# Fontpub Architecture — v1

This document describes the recommended software architecture for implementing the current Fontpub protocol.

It is intentionally narrower than a generic deployment guide: it focuses on preserving Fontpub's core property that published package metadata remains publicly inspectable, verifiable, and rebuildable.

## Architectural principle

Fontpub is a **public-artifact-first append-only system**.

The authoritative published record for a package version is:

- `/v1/packages/{owner}/{repo}/versions/{version_key}.json`

Everything else in the public API is derived from that record.

## Artifact classes

### Authoritative artifact

The versioned package detail document is authoritative because it captures:
- the package identity
- the manifest-derived metadata
- the pinned Git commit SHA
- the asset URLs
- the asset SHA-256 digests

Once published, it is immutable.

### Derived artifacts

The following are derived documents:
- `/v1/packages/{owner}/{repo}/index.json`
- `/v1/packages/{owner}/{repo}.json`
- `/v1/index.json`

They exist for discoverability and efficient reads, not as the canonical record.

## Components

### 1. Font repository

The canonical source of package contents is a GitHub repository containing:
- `fontpub.json`
- font binaries

### 2. Update API

The Update API is the only component that accepts publication requests.

Its responsibilities are:
- validate GitHub Actions OIDC
- enforce workflow and replay restrictions
- fetch the manifest and assets from SHA-pinned GitHub Raw URLs
- validate manifest and path constraints
- compute SHA-256 digests
- enforce immutability and resource limits
- publish authoritative versioned package detail documents
- update derived documents

### 3. Public artifact store

Public JSON documents should be served from object storage and CDN-compatible static hosting.

The public artifact store contains:
- authoritative versioned package detail documents
- derived package versions indexes
- derived latest aliases
- the derived root index

### 4. Rebuilder

The Rebuilder scans authoritative versioned package detail documents and regenerates all derived documents.

It is required because:
- derived documents are not authoritative
- partial update failures must be repairable
- public state should remain reproducible from published immutable artifacts

### 5. Private state

Private state must be minimal and must not be the authoritative source of package metadata.

Private state is limited to:
- ownership binding: `package_id -> repository_id`
- replay prevention for JWT `jti`
- transient publication coordination if needed

Loss of this private state may interrupt future publication, but must not invalidate already-published package metadata.

## Recommended deployment split

### Read plane

The read plane should be static:
- object storage
- CDN
- immutable JSON documents
- strong `ETag`

For v1 implementations, a single S3-compatible artifact backend is a good default because it can target:
- AWS S3
- Cloudflare R2
- MinIO
- other S3-compatible stores

### Write plane

The write plane should be stateful and small:
- OIDC validation
- fetch/hash/validate logic
- publication sequencing
- ownership and replay checks

For local development, an in-memory private-state backend is acceptable. For any operator workflow that expects restart-safe replay protection and ownership binding, use a durable private-state backend.

The read plane must not depend on the write plane to serve already-published metadata.

## Publication model

Publication should behave like this:

1. Validate the request and OIDC token.
2. Fetch and validate `fontpub.json`.
3. Fetch and hash all declared assets.
4. Build the candidate versioned package detail document.
5. Compare against any already-published document for the same `package_id` and `version_key`.
6. Publish the authoritative versioned package detail document.
7. Update derived documents.

If step 7 fails, retrying the same request must be sufficient to restore derived-document consistency without mutating the authoritative versioned package detail document.

## Rebuildability requirement

A conforming implementation must be able to rebuild:
- each package versions index
- each latest package detail alias
- the root index

using only:
- the set of published versioned package detail documents
- the protocol rules in `docs/`

No unpublished database rows should be required to reconstruct public metadata.

For derived documents that include timestamps such as `generated_at`, the value must itself be derivable from authoritative published artifacts under the protocol rules.

## Technology recommendations

These are recommendations, not protocol requirements:

- CLI: Go
- Update API: Go
- Rebuilder: Go
- Public artifact store: S3-compatible or R2-like object storage
- Private state store: a small durable store suitable for ownership and replay tracking
  - a file-backed store is a reasonable minimum implementation
  - a memory-backed store is appropriate for tests and ephemeral local runs

The key requirement is not the product choice. The key requirement is preserving the authoritative role of public immutable metadata.

## Recommended repository organization

The repository should be organized around the protocol assets and the append-only publication model so that the CLI, Update API, and Rebuilder share deterministic logic.

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

Repository organization guidelines:
- Use one shared Go module so the CLI, Update API, and Rebuilder can share versioning, hashing, canonicalization, and protocol logic.
- Keep `protocol/` language-neutral where possible so it remains usable by other implementations.
- If a website or docs app is added later, it should not own protocol logic.
