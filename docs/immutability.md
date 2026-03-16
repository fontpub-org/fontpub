# Immutability Rules — v1

Within a package, a version is **immutable**.

If an update request attempts to publish metadata for a version key that already exists and the distributable content differs, the Indexer MUST reject the update.

## What is immutable (v1)

For a given `package_id` and `version_key`, the following MUST remain identical:

- The published package identity fields:
  - `package_id`
  - `version`
  - `version_key`
- The manifest fields that affect distribution:
  - `name`, `author`, `license`
  - `files[]` entries (`path`, `style`, `weight`)
- The pinned source revision:
  - `github.owner`
  - `github.repo`
  - `github.sha`
  - `manifest_url`
- The derived asset set:
  - The set of asset `path` values (no add/remove/rename)
  - For each asset `path`:
    - `url`
    - `sha256`
    - `format`
    - `style`, `weight` (as in manifest)

Non-distribution repository content (README, images, CI config, etc.) is out of scope.

## How to enforce

When processing an update:
1. The Indexer generates the candidate package detail document.
2. The Indexer derives the candidate `version_key` from the manifest version.
3. If the Indexer already has a package detail document for the same `package_id` and `version_key`, it MUST compare immutable content:
   - Normalize both documents by:
     - Sorting assets by `path`
     - Comparing only the immutable fields listed above
4. If any difference exists in the immutable fields, reject with `409 Conflict` and error code `IMMUTABLE_VERSION`.
5. If no difference exists, the update is idempotent:
   - the existing versioned package detail remains authoritative
   - `published_at` and immutable ETags for that version MUST NOT change
   - the Indexer MAY refresh derived documents such as the package latest alias or root index if they are missing or stale

## Rationale

Immutability ensures:
- Clients can rely on versions for reproducibility.
- Supply-chain attacks via “replace-in-place” are prevented, including swaps to a different commit SHA.
