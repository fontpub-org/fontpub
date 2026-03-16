# Publisher Workflow — v1

This document defines the expected GitHub Actions workflow used by Fontpub publishers.

It complements `security-oidc.md` and explains what `fontpub workflow init` must generate.

## Workflow file

- The workflow file path MUST be `.github/workflows/fontpub.yml`.

## Trigger conditions

The workflow MUST support at least one of:
- `push` for release tags
- `workflow_dispatch`

If triggered by `push`, it MUST be restricted to release tags compatible with Fontpub versioning.

## Permissions

The workflow MUST be able to request a GitHub Actions OIDC token suitable for calling `POST /v1/update`.

## Publication behavior

The workflow MUST:
1. determine the repository name
2. determine the triggering commit SHA
3. determine the triggering ref
4. request an OIDC token whose audience is `https://fontpub.org`
5. call `POST /v1/update` with:
   - `repository`
   - `sha`
   - `ref`

## Generated workflow expectations

`fontpub workflow init` MUST generate a workflow that:
- uses the required path `.github/workflows/fontpub.yml`
- is compatible with the claim requirements in `security-oidc.md`
- sends the exact request body required by `indexer-api.md`

## Non-goals

This document does not standardize:
- the GitHub Actions runner image
- the shell used in workflow steps
- log formatting
- retry strategy inside the workflow
