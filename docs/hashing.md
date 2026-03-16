# Hashing & Asset Fetching — v1

The Indexer computes SHA-256 digests for each asset listed in `fontpub.json`.

## Digest algorithm
- SHA-256
- Hex-encoded lowercase, 64 characters

## Fetching rules
- Assets MUST be fetched from an immutable GitHub Raw URL pinned to a commit SHA:

`https://raw.githubusercontent.com/<owner>/<repo>/<sha>/<path>`

- Branch refs MUST NOT be used in URLs.
- `<path>` MUST be constructed from the manifest path by percent-encoding each path segment as needed, while preserving `/` separators.

## Streaming requirement (recommended v1)
Implementations SHOULD compute SHA-256 using streaming/incremental hashing to avoid loading full files into memory.

If an implementation cannot stream:
- It MAY buffer an asset in memory provided:
  - single asset size ≤ 50 MiB
  - concurrency is limited to prevent memory exhaustion

## Size limits
- A single asset MUST be <= 50 MiB (50 * 1024 * 1024 bytes).
