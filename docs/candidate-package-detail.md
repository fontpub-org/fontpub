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

If `--package-id <owner>/<repo>` is provided, the CLI MUST:
- validate that it is a canonical package ID after lowercase normalization
- use that value as `package_id`

If `--package-id` is omitted, the CLI MUST derive `package_id` from local Git metadata using this algorithm:
1. Enumerate Git remotes for the selected repository root.
2. Convert any remote URL that clearly identifies `github.com/<owner>/<repo>` to the canonical lowercase package ID `owner/repo`.
3. Ignore remotes that do not map to public GitHub repository URLs.
4. If exactly one distinct canonical package ID remains, use it.
5. If zero canonical package IDs remain, fail with CLI error code `PACKAGE_ID_REQUIRED`.
6. If more than one distinct canonical package ID remains, fail with CLI error code `PACKAGE_ID_AMBIGUOUS`.

Recognized GitHub remote URL forms are:
- `https://github.com/<owner>/<repo>`
- `https://github.com/<owner>/<repo>.git`
- `git@github.com:<owner>/<repo>.git`

No other hostnames are recognized for v1 package identity derivation.

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
- `package_id` MUST be either:
  - the validated explicit `--package-id` value, or
  - the canonical GitHub `owner/repo` identity derived from local Git metadata
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
