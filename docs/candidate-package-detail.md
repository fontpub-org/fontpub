# Candidate Package Detail — v1

This document defines the unpublished preview object emitted by `fontpub package preview`.

It exists so that publisher tooling can show a deterministic pre-publication view without pretending that a local preview is the same thing as a published authoritative artifact.

## Purpose

A candidate package detail:
- is derived from the current local repository state
- is intended for review, validation, and automation
- MUST NOT be treated as a published package record

## Source of truth

`fontpub package preview` MUST derive the candidate object from:
- the `fontpub.json` file under the selected repository root
- the asset files referenced by that manifest in the current local filesystem state

It MUST NOT require a Git tag, a published update, or network access.

The selected repository root MUST be a GitHub repository whose canonical `owner/repo` package identity can be derived from local Git metadata. If package identity cannot be derived, `fontpub package preview` MUST fail with CLI error code `INPUT_REQUIRED`.

## Relationship to published package detail

The candidate package detail is intentionally distinct from the published versioned package detail in `indexes.md`.

It differs in two important ways:
- it is local and unpublished
- it omits fields whose meaning only exists after publication

## Schema (conceptual)

```json
{
  "schema_version": "1",
  "package_id": "owner/repo",
  "display_name": "string",
  "author": "string",
  "license": "OFL-1.1",
  "version": "string",
  "version_key": "string",
  "source": {
    "kind": "local_repository",
    "root_path": "/absolute/path/to/repository"
  },
  "assets": [
    {
      "path": "string",
      "sha256": "64-hex",
      "format": "otf | ttf | woff2",
      "style": "normal | italic | oblique",
      "weight": 1,
      "size_bytes": 0
    }
  ]
}
```

## Requirements

- `schema_version` MUST equal `"1"`.
- `package_id` MUST be derived from the current repository's canonical GitHub `owner/repo` identity.
- `version_key` MUST be derived from `version` using `versioning.md`.
- `assets[]` MUST include exactly the files declared in the manifest.
- `assets[]` MUST be sorted by `path` ascending.
- `sha256` MUST be computed from local asset bytes.
- `size_bytes` MUST be included.
- The candidate object MUST NOT include:
  - `published_at`
  - `github`
  - `manifest_url`
  - `assets[].url`

## Non-goals

The candidate package detail does not guarantee that publication will succeed.

In particular, it does not prove:
- that the repository is on a valid release tag
- that OIDC claims will satisfy publication policy
- that the eventual published document will be byte-identical
