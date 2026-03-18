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

If triggered by `workflow_dispatch`, the workflow MUST define a required string input named `tag`.

## Permissions

The workflow MUST be able to request a GitHub Actions OIDC token suitable for calling `POST /v1/update`.

At minimum, the workflow MUST grant:
- `id-token: write`
- `contents: read`

## Publication behavior

The workflow MUST:
1. determine the repository name
2. determine the repository ID from the GitHub Actions context
3. determine the triggering commit SHA
4. determine the triggering ref
5. request an OIDC token whose audience is `https://fontpub.org`
6. call `POST /v1/update` with:
   - `repository`
   - `sha`
   - `ref`

Additional requirements:
- The workflow MUST be a repository workflow located at `.github/workflows/fontpub.yml`, not a reusable workflow imported from another repository.
- If triggered by `workflow_dispatch`, the workflow MUST construct `ref` as `refs/tags/<tag>` using the required `tag` input.
- If triggered by `workflow_dispatch`, the workflow MUST resolve `sha` to the commit pointed to by that exact tag ref.
- If triggered by `workflow_dispatch`, the workflow MUST fail before calling `POST /v1/update` if the tag input is missing, malformed, or not found.

## Generated workflow expectations

`fontpub workflow init` MUST generate a workflow that:
- uses the required path `.github/workflows/fontpub.yml`
- is compatible with the claim requirements in `security-oidc.md`
- requests `id-token: write` and `contents: read`
- sends the exact request body required by `indexer-api.md`
- validates the `workflow_dispatch` `tag` input before attempting publication
- fails before publication if the requested tag is missing, malformed, or does not resolve to a commit
- resolves `sha` using the exact selected tag ref rather than the default checkout branch state
- makes the publication steps readable enough that a publisher can audit what will be sent to Fontpub

## Non-goals

This document does not standardize:
- the GitHub Actions runner image
- the shell used in workflow steps
- log formatting
- retry strategy inside the workflow
