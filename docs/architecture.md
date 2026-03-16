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
- ownership binding: `package_id -> sub`
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

### Write plane

The write plane should be stateful and small:
- OIDC validation
- fetch/hash/validate logic
- publication sequencing
- ownership and replay checks

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

## Technology recommendations

These are recommendations, not protocol requirements:

- CLI: Go
- Update API: Go
- Rebuilder: Go
- Public artifact store: S3-compatible or R2-like object storage
- Private state store: a small durable store suitable for ownership and replay tracking

The key requirement is not the product choice. The key requirement is preserving the authoritative role of public immutable metadata.
