# Index Documents — v1 (Split Index)

Fontpub v1 uses a **split index**:
- Root index: `/v1/index.json` (lightweight, cache-friendly)
- Package versions index: `/v1/packages/{owner}/{repo}/index.json`
- Latest package detail alias: `/v1/packages/{owner}/{repo}.json`
- Versioned package detail: `/v1/packages/{owner}/{repo}/versions/{version_key}.json`

All JSON is UTF-8.

## Canonical JSON

To keep responses byte-stable and cache-friendly:

- Timestamps MUST be RFC3339 in UTC with `Z` and second precision.
- Producers MUST serialize JSON without insignificant whitespace.
- Object keys MUST be serialized in lexicographic order.
- Package IDs MUST be serialized in lowercase lexicographic order.
- `assets[]` MUST be sorted by `path` ascending.
- `versions[]` MUST be sorted by version precedence descending (newest first).

## Package ID

Canonical package identifier is the GitHub repository: `owner/repo`.

The Indexer MUST normalize package IDs to lowercase for storage/lookup.

Repository renames are out of scope for v1 package identity.

- A changed `owner/repo` path is a new `package_id`.
- Historical documents for the old `package_id` remain valid if retained by the Indexer.

---

## Root index: `/v1/index.json`

### Purpose
Fast listing and update checks without downloading large metadata.

This document is derived from published versioned package detail documents.

### Schema (conceptual)

```json
{
  "schema_version": "1",
  "generated_at": "RFC3339 timestamp",
  "packages": {
    "owner/repo": {
      "latest_version": "string",
      "latest_version_key": "string",
      "latest_published_at": "RFC3339 timestamp"
    }
  }
}
```

Notes:
- `packages` keys are package IDs.
- `latest_version` is the literal version string from the latest package detail document.
- `latest_version_key` is derived from `latest_version` per `versioning.md`.
- `latest_published_at` is when the current latest version was first published.

---

## Package versions index: `/v1/packages/{owner}/{repo}/index.json`

### Purpose
Discover every published immutable version for a package.

This document is derived from published versioned package detail documents for the package.

### Schema (conceptual)

```json
{
  "schema_version": "1",
  "package_id": "owner/repo",
  "latest_version": "string",
  "latest_version_key": "string",
  "versions": [
    {
      "version": "string",
      "version_key": "string",
      "published_at": "RFC3339 timestamp",
      "url": "/v1/packages/owner/repo/versions/1.2.3.json"
    }
  ]
}
```

Notes:
- `versions[]` contains one entry per immutable `version_key`.
- `version` preserves the manifest's literal version string.
- `url` is the canonical versioned package detail path for that entry.

---

## Latest package detail alias: `/v1/packages/{owner}/{repo}.json`

### Purpose
Convenience endpoint for the latest published version of a package.

Requirements:
- MUST return the same JSON document as the highest-precedence entry in the package versions index.
- MUST be byte-identical to the corresponding versioned package detail response.

This document is derived from the highest-precedence published versioned package detail document.

---

## Versioned package detail: `/v1/packages/{owner}/{repo}/versions/{version_key}.json`

### Purpose
The canonical metadata a client uses to install and verify assets.

This document is the authoritative public record for a published package version.

### Schema (conceptual)

```json
{
  "schema_version": "1",
  "package_id": "owner/repo",
  "display_name": "string",
  "author": "string",
  "license": "OFL-1.1",
  "version": "string",
  "version_key": "string",
  "published_at": "RFC3339 timestamp",
  "github": {
    "owner": "string",
    "repo": "string",
    "sha": "40-hex commit SHA"
  },
  "manifest_url": "https://raw.githubusercontent.com/<owner>/<repo>/<sha>/fontpub.json",
  "assets": [
    {
      "path": "string",
      "url": "https://raw.githubusercontent.com/<owner>/<repo>/<sha>/<path>",
      "sha256": "64-hex",
      "format": "otf | ttf | woff2",
      "style": "normal | italic | oblique",
      "weight": 1,
      "size_bytes": 0
    }
  ]
}
```

### Requirements
- `assets[]` MUST include exactly the files declared in the manifest.
- `url` MUST be pinned to the commit SHA (`.../<sha>/...`), never a branch ref.
- `sha256` MUST be computed from the asset bytes.
- `format` MUST be derived from file extension.
- `version_key` MUST be derived from `version` using `versioning.md`.
- `published_at` is when this immutable version was first published and MUST NOT change on idempotent replays.
- `size_bytes` SHOULD be included; if unknown, MAY be omitted.

### Ordering
- Producers MUST sort `assets` by `path` ascending.
- Consumers MUST NOT treat ordering as semantically meaningful.

---

## Allowed asset formats (v1)
- `.otf` → `otf`
- `.ttf` → `ttf`
- `.woff2` → `woff2`

Other extensions MUST be rejected by the Indexer with a validation error.
