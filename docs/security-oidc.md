# Security: GitHub OIDC Authentication — v1

Fontpub v1 authenticates Indexer updates using GitHub Actions OIDC JWTs.

This document specifies the validation rules for `POST /v1/update`.

## Token source

The Indexer MUST only accept tokens issued by GitHub Actions OIDC:

- `iss` MUST equal: `https://token.actions.githubusercontent.com`

The Indexer MUST validate the JWT signature using GitHub's JWKS.

## Audience

- `aud` MUST include (or equal): `https://fontpub.org`

## Time validity

- `exp` MUST be in the future
- `iat` MUST be within ±10 minutes of server time

## Required claims (v1)

The JWT MUST include:

- `sub` (string): used for ownership binding
- `repository` (string): `owner/repo`
- `repository_owner` (string): MUST equal the owner segment of `repository`
- `sha` (string): 40-hex commit SHA
- `ref` (string): git ref that triggered the workflow

If any required claim is missing, reject with `401 Unauthorized` and error code `AUTH_CLAIMS_MISSING`.

## Normalization

- The Indexer MUST normalize `repository` to lowercase for storage and comparisons.
- The Indexer MUST compare `repository_owner` against the normalized owner segment of `repository`.

## Ownership binding

When a package is first registered, the Indexer MUST bind `package_id` to `sub`.

For subsequent updates:
- Require the same `sub` for the same `package_id`.
- If `sub` mismatches, reject with `403 Forbidden` and error code `OWNERSHIP_MISMATCH`.

This binding prevents takeovers within a stable `package_id`. Repository renames are handled as new package IDs in v1.

## Workflow restrictions

To minimize attack surface, v1 restricts updates to release tags.

- `ref` MUST match: `refs/tags/<tag>`
- `<tag>` MUST itself be a valid Numeric Dot version string, with the same optional leading `v` or `V` support described in `versioning.md`
- The tag's version key MUST equal the manifest version key

If a token fails policy restrictions, reject with `403 Forbidden` and error code `WORKFLOW_NOT_ALLOWED`.
