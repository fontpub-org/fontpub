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
- `iat` SHOULD be within ±10 minutes of server time
- Implementations SHOULD allow up to **±60 seconds** clock skew

## Required claims (v1)

The JWT MUST include:

- `sub` (string): used for ownership binding
- `repository` (string): `owner/repo`
- `repository_owner` (string)
- `sha` (string): 40-hex commit SHA
- `ref` (string): git ref that triggered the workflow

If any required claim is missing, reject with `401 Unauthorized` and error code `AUTH_CLAIMS_MISSING`.

## Normalization

- The Indexer MUST normalize `repository` to lowercase for storage and comparisons.

## Ownership binding

When a package is first registered:
- Store `package_id -> sub` in durable storage (KV).

For subsequent updates:
- Require the same `sub` for the same `package_id`.
- If `sub` mismatches, reject with `403 Forbidden` and error code `OWNERSHIP_MISMATCH`.

This supports repository renames (same subject) while preventing takeovers.

## Workflow restrictions (recommended v1 policy)

To minimize attack surface, v1 RECOMMENDS restricting updates to release tags.

- `ref` MUST match: `refs/tags/v*`

Optional hardening (SHOULD if available in claims):
- Restrict to a specific workflow file via `workflow_ref` or `job_workflow_ref`:
  - MUST reference `.github/workflows/fontpub.yml@...`
- Reject PR-triggered contexts if an `event_name` claim is available:
  - Allow only `push` and/or `workflow_dispatch`

If a token fails policy restrictions, reject with `403 Forbidden` and error code `WORKFLOW_NOT_ALLOWED`.

## JWKS caching

Implementations SHOULD cache GitHub JWKS for 1–6 hours.

If a token fails signature verification:
- Refresh JWKS once and retry verification exactly once.
- If still failing, reject with `401 Unauthorized` and error code `AUTH_INVALID_TOKEN`.
