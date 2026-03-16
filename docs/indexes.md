# Index Documents — v1 (Split Index)

Fontpub v1 uses a **split index**:
- Root index: `/v1/index.json` (lightweight, cache-friendly)
- Package detail: `/v1/packages/{owner}/{repo}.json` (full asset metadata)

All JSON is UTF-8.

## Package ID

Canonical package identifier is the GitHub repository: `owner/repo`.

The Indexer MUST normalize package IDs to lowercase for storage/lookup.

---

## Root index: `/v1/index.json`

### Purpose
Fast listing and update checks without downloading large metadata.

### Schema (conceptual)

```json
{
  "schema_version": "1",
  "generated_at": "RFC3339 timestamp",
  "packages": {
    "owner/repo": {
      "latest_version": "string",
      "last_updated": "RFC3339 timestamp"
    }
  }
}
```

Notes:
- `packages` keys are package IDs.
- `latest_version` follows Numeric Dot (see `versioning.md`).
- `last_updated` is when the Indexer last successfully processed an update for the package.

---

## Package detail: `/v1/packages/{owner}/{repo}.json`

### Purpose
The canonical metadata a client uses to install and verify assets.

### Schema (conceptual)

```json
{
  "schema_version": "1",
  "package_id": "owner/repo",
  "display_name": "string",
  "author": "string",
  "license": "OFL-1.1",
  "version": "string",
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
- `size_bytes` SHOULD be included; if unknown, MAY be omitted.

### Ordering
- Producers SHOULD sort `assets` by `path` ascending.
- Consumers MUST NOT treat ordering as semantically meaningful.

---

## Allowed asset formats (v1)
- `.otf` → `otf`
- `.ttf` → `ttf`
- `.woff2` → `woff2`

Other extensions MUST be rejected by the Indexer with a validation error.
