# Fontpub v1 Overview

Fontpub is a distribution protocol and tooling ecosystem for open-source fonts.

Core idea: the canonical source of truth for a font package is a Git repository (initially assumed to be GitHub). The Fontpub service ("Indexer") does not mirror font binaries; instead it publishes verifiable metadata (immutable URLs pinned to a commit SHA plus SHA-256 digests) that clients can use to securely download, verify, install, and activate fonts.

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
  - A lightweight root index listing packages and their latest versions
  - Per-package detail documents listing all assets and their digests

### 3) CLI (client)
A tool (initially targeting macOS) that:
- Lists packages via the root index
- Installs packages by fetching a package detail document, downloading assets from GitHub Raw, and verifying SHA-256
- Activates/deactivates fonts by linking them into a macOS font directory

## Design goals (v1)
- Integrity: every installed file is verified against the Indexer-published SHA-256
- Reproducibility: every asset URL is pinned to an immutable Git commit SHA
- Minimal central trust: the Indexer stores metadata, not binaries
- Operational simplicity: “release by tag” for updates (recommended), cache-friendly reads via ETag

## Non-goals (v1)
- Hosting/mirroring font binaries
- Supporting proprietary licenses (v1 is OFL-1.1 only)
- Supporting pre-release version semantics (e.g., -alpha)
