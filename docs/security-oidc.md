# Security: GitHub OIDC Authentication — v1

Fontpub v1 authenticates Indexer updates using GitHub Actions OIDC JWTs.

This document specifies the validation rules for `POST /v1/update`.

The expected publisher workflow layout is described in `publisher-workflow.md`.

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

- `sub` (string): opaque GitHub subject value, retained for auditability
- `repository` (string): `owner/repo`
- `repository_id` (string): stable GitHub repository identifier used for ownership binding
- `repository_owner` (string): MUST equal the owner segment of `repository`
- `sha` (string): 40-hex commit SHA
- `ref` (string): git ref that triggered the workflow
- `workflow_ref` (string): identifies the workflow file that issued the token
- `workflow_sha` (string): commit SHA for the workflow file revision used to mint the token
- `jti` (string): unique token identifier
- `event_name` (string): workflow trigger event name

If any required claim is missing, reject with `401 Unauthorized` and error code `AUTH_CLAIMS_MISSING`.

## Normalization

- The Indexer MUST normalize `repository` to lowercase for storage and comparisons.
- The Indexer MUST compare `repository_owner` against the normalized owner segment of `repository`.
- The Indexer MUST treat `repository_id` as an opaque stable identifier and MUST NOT derive it from `repository`.

## Ownership binding

When a package is first registered, the Indexer MUST bind `package_id` to `repository_id`.

For subsequent updates:
- Require the same `repository_id` for the same `package_id`.
- If `repository_id` mismatches, reject with `403 Forbidden` and error code `OWNERSHIP_MISMATCH`.

`sub` MUST still be present, but v1 does not use it as the stable ownership key because GitHub subject formats may vary across workflows and repository configuration.

This binding prevents takeovers within a stable `package_id`. Repository renames are handled as new package IDs in v1 even though `repository_id` itself is stable.

## Workflow restrictions

To minimize attack surface, v1 restricts updates to release tags.

- `ref` MUST match: `refs/tags/<tag>`
- `<tag>` MUST itself be a valid Numeric Dot version string, with the same optional leading `v` or `V` support described in `versioning.md`
- The tag's version key MUST equal the manifest version key
- `workflow_ref` MUST reference `.github/workflows/fontpub.yml` in the same repository named by `repository`
- `workflow_sha` MUST be present
- `event_name` MUST be either `push` or `workflow_dispatch`

If a token fails policy restrictions, reject with `403 Forbidden` and error code `WORKFLOW_NOT_ALLOWED`.

## Replay protection

- The Indexer MUST reject reuse of the same `jti` while the corresponding token remains valid.
- A reused token MUST be rejected with `401 Unauthorized` and error code `AUTH_REPLAY_DETECTED`.
