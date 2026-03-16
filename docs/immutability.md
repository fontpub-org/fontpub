# Immutability Rules — v1

Within a package, a version is **immutable**.

If an update request attempts to publish metadata for a version that already exists and the distributable content differs, the Indexer MUST reject the update.

## What is immutable (v1)

For a given `package_id` and `version`, the following MUST remain identical:

- The manifest fields that affect distribution:
  - `name`, `author`, `license`, `version`
  - `files[]` entries (`path`, `style`, `weight`)
- The derived asset set:
  - The set of asset `path` values (no add/remove/rename)
  - For each asset `path`:
    - `sha256`
    - `format`
    - `style`, `weight` (as in manifest)

Non-distribution repository content (README, images, CI config, etc.) is out of scope.

## How to enforce

When processing an update:
1. The Indexer generates the candidate package detail document.
2. If the Indexer already has a package detail document for the same `package_id` and `version`, it MUST compare distributable content:
   - Normalize both documents by:
     - Sorting assets by `path`
     - Removing non-essential transient fields (e.g., timestamps)
3. If any difference exists in the immutable fields, reject with `409 Conflict` and error code `IMMUTABLE_VERSION`.

## Rationale

Immutability ensures:
- Clients can rely on versions for reproducibility.
- Supply-chain attacks via “replace-in-place” are prevented.
