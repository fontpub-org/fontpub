# Fontpub v1 Overview

Fontpub is a distribution protocol and tooling ecosystem for open-source fonts.

Core idea: the canonical source of truth for a font package is a GitHub repository. The Fontpub service ("Indexer") does not mirror font binaries; instead it publishes verifiable metadata (immutable URLs pinned to a commit SHA plus SHA-256 digests) that clients can use to securely download, verify, install, and activate fonts.

Recommended implementation architecture is described in `architecture.md`.
CLI machine-readable behavior is described in `cli-json.md`.
Expected GitHub Actions publication workflow is described in `publisher-workflow.md`.
CLI JSON is part of the protocol surface and should be backed by schemas and conformance fixtures in `protocol/`.

## Components

### 1) Font repository (canonical source)
A Git repository containing:
- Font binaries (e.g., .otf, .ttf, .woff2)
- A manifest file at repository root: fontpub.json

### 2) Indexer (public metadata notary)
A web service that:
- Authenticates update requests via GitHub Actions OIDC (JWT)
- Fetches the repository’s fontpub.json and declared assets at a specific commit SHA
- Computes SHA-256 for each asset (without persisting binaries)
- Publishes:
  - Immutable versioned package detail documents as the authoritative public record
  - A lightweight root index listing packages and their latest versions
  - Per-package version indexes
  - A latest package detail alias

### 3) CLI (client)
A tool that:
- Lists packages via the root index
- Installs packages by fetching a package detail document, downloading assets from GitHub Raw, and verifying SHA-256
- Activates/deactivates fonts by linking them into a platform-defined font activation directory
- Supports both human-oriented interactive use and deterministic machine-readable automation

## Design goals (v1)
- Integrity: every installed file is verified against the Indexer-published SHA-256
- Reproducibility: every asset URL is pinned to an immutable Git commit SHA
- Minimal central trust: the Indexer stores metadata, not binaries
- Operational simplicity: release-by-tag updates and cache-friendly reads via ETag
- Rebuildability: derived indexes can be regenerated from published immutable package detail documents

## Non-goals (v1)
- Hosting/mirroring font binaries
- Supporting proprietary licenses (v1 is OFL-1.1 only)
- Supporting pre-release version semantics (e.g., -alpha)
