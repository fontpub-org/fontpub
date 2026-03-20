# Quickstart

This document explains how to try the current Fontpub implementation locally.

It is written for two audiences:
- font publishers who want to generate and validate `fontpub.json`
- Fontpub operators who want to run the local Indexer and Rebuilder binaries

## Prerequisites

- Go installed locally
- Git installed locally
- a local clone of the Fontpub repository

Examples below assume the repository root is the current directory.

## Build Or Run The CLI

The simplest way to try the CLI is with `go run`:

```bash
cd go
go run ./cmd/fontpub --help
```

If you prefer a reusable binary:

```bash
cd go
go build -o /tmp/fontpub ./cmd/fontpub
/tmp/fontpub --help
```

## Try As A Font Publisher

Assume your font repository contains font files such as:

```text
your-font-repo/
├─ fontpub.json              # optional before init
├─ dist/
│  ├─ ExampleSans-Regular.otf
│  ├─ ExampleSans-Italic.otf
│  └─ ExampleSans-Regular.ttf
```

### 1. Generate A Candidate Manifest

`package init` scans the repository, prefers embedded metadata from `.ttf` and `.otf` when available, and falls back to filename heuristics when needed.

Without `--write`, it prints the candidate manifest to stdout:

```bash
cd go
go run ./cmd/fontpub package init /path/to/your-font-repo
```

To write `fontpub.json` into the target repository:

```bash
cd go
go run ./cmd/fontpub package init /path/to/your-font-repo --write
```

To overwrite an existing `fontpub.json`:

```bash
cd go
go run ./cmd/fontpub package init /path/to/your-font-repo --write --yes
```

To inspect machine-readable inference details:

```bash
cd go
go run ./cmd/fontpub package init /path/to/your-font-repo --json
```

### 2. Validate The Manifest

```bash
cd go
go run ./cmd/fontpub package validate /path/to/your-font-repo --json
```

This checks:
- manifest structure
- version and license rules
- declared file existence
- asset path constraints

### 3. Preview Published Metadata

`package preview` renders the candidate package detail document from local repository state.

```bash
cd go
go run ./cmd/fontpub package preview /path/to/your-font-repo --package-id owner/repo --json
```

If `--package-id` is omitted, the CLI tries to derive `owner/repo` from local Git remotes.

### 4. Inspect A Single Font File

```bash
cd go
go run ./cmd/fontpub package inspect /path/to/your-font-repo/dist/ExampleSans-Regular.otf --json
```

This is useful when you want to see what the CLI can infer before generating `fontpub.json`.

### 5. Check Publication Readiness

```bash
cd go
go run ./cmd/fontpub package check /path/to/your-font-repo --tag v1.2.3 --json
```

This verifies:
- the manifest is valid
- declared files exist
- the provided tag matches the manifest version

### 6. Generate A GitHub Actions Workflow

```bash
cd go
go run ./cmd/fontpub workflow init /path/to/your-font-repo --yes
```

This writes `.github/workflows/fontpub.yml` in the target repository.

You can inspect the generated workflow first with:

```bash
cd go
go run ./cmd/fontpub workflow init /path/to/your-font-repo --dry-run --json
```

## Try As A Font User

The CLI also supports user-facing commands such as:

```bash
cd go
go run ./cmd/fontpub --help
go run ./cmd/fontpub ls-remote --json
go run ./cmd/fontpub ls --json
go run ./cmd/fontpub show owner/repo --json
```

During a migration period, implementations may also continue to support:

```bash
go run ./cmd/fontpub list --json     # compatibility alias for ls-remote
go run ./cmd/fontpub status --json   # compatibility alias for ls
```

To exercise install and activation flows, you need a running Fontpub metadata service and published package metadata.

## Run The Local Operator Binaries

The repository also contains local operator binaries:

- `fontpub-indexer`
- `fontpub-rebuilder`

### Rebuilder

```bash
cd go
FONTPUB_ARTIFACTS_DIR=/path/to/artifacts go run ./cmd/fontpub-rebuilder
```

### Indexer

```bash
cd go
FONTPUB_ARTIFACTS_DIR=/path/to/artifacts \
FONTPUB_STATE_BACKEND=file \
FONTPUB_STATE_DIR=/path/to/state \
FONTPUB_GITHUB_JWKS_JSON='{"keys":[...]}' \
go run ./cmd/fontpub-indexer
```

The Indexer expects GitHub Actions OIDC-compatible JWT verification material and an artifacts directory for public JSON documents.

For local development, the Indexer defaults to an in-memory private-state backend. That is convenient for tests, but replay protection and package ownership bindings are lost when the process exits.

For a minimum persistent setup, use:

```bash
export FONTPUB_STATE_BACKEND=file
export FONTPUB_STATE_DIR=/path/to/state
```

This keeps JWT replay state and `package_id -> repository_id` ownership bindings across restarts.

For production-like operation, `fontpub-indexer` can fetch GitHub's JWKS remotely instead of requiring `FONTPUB_GITHUB_JWKS_JSON`.

By default it uses:

```text
https://token.actions.githubusercontent.com/.well-known/jwks
```

Optional settings:

```bash
export FONTPUB_GITHUB_JWKS_URL=https://token.actions.githubusercontent.com/.well-known/jwks
export FONTPUB_GITHUB_JWKS_TIMEOUT=5s
export FONTPUB_GITHUB_JWKS_CACHE_TTL=10m
```

If `FONTPUB_GITHUB_JWKS_JSON` is set, the static JSON value takes precedence over remote fetch mode.

### S3-Compatible Artifact Backend

Both `fontpub-indexer` and `fontpub-rebuilder` support `file`, `memory`, and `s3` artifact backends.

To use an S3-compatible backend, set:

```bash
export FONTPUB_ARTIFACTS_BACKEND=s3
export FONTPUB_S3_BUCKET=fontpub-artifacts
export FONTPUB_S3_REGION=auto
export FONTPUB_S3_ENDPOINT=https://<account>.r2.cloudflarestorage.com
export FONTPUB_S3_PREFIX=dev
export FONTPUB_S3_FORCE_PATH_STYLE=false
```

Credentials are resolved through the normal AWS SDK environment and shared-config chain, for example:

```bash
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
```

With those variables set, the binaries can be started the same way:

```bash
cd go
FONTPUB_STATE_BACKEND=file \
FONTPUB_STATE_DIR=/path/to/state \
FONTPUB_GITHUB_JWKS_JSON='{"keys":[...]}' \
go run ./cmd/fontpub-indexer
go run ./cmd/fontpub-rebuilder
```

`FONTPUB_S3_PREFIX` is optional. Use it when you want Fontpub to write under a sub-prefix instead of the bucket root.

### Local-Only End-To-End Mode

If you want to test publication and installation without pushing a font repository to GitHub, the current implementation supports a development-only local Git mode.

Set `FONTPUB_DEV_LOCAL_REPO_MAP` to map a canonical package ID to a local Git checkout:

```bash
export FONTPUB_DEV_LOCAL_REPO_MAP='owner/repo=/path/to/local/repo'
```

In this mode:

- `fontpub-indexer` still receives the normal `repository`, `sha`, and `ref` request body
- published metadata still contains SHA-pinned `raw.githubusercontent.com` URLs
- but the implementation resolves those GitHub Raw URLs from the mapped local Git checkout when the repository is present in `FONTPUB_DEV_LOCAL_REPO_MAP`

This is a development aid only. It does not change the public Fontpub protocol.

A practical local-only flow is:

1. prepare and commit `fontpub.json` in the target font repository
2. ensure the local release tag points at that commit
3. run `fontpub-indexer` with both:
   - `FONTPUB_ARTIFACTS_DIR=/path/to/artifacts`
   - `FONTPUB_DEV_LOCAL_REPO_MAP='owner/repo=/path/to/local/repo'`
4. publish into the local artifacts directory
5. serve the artifacts directory with a static file server
6. run the user CLI with:
   - `FONTPUB_BASE_URL=http://127.0.0.1:<port>`
   - `FONTPUB_DEV_LOCAL_REPO_MAP='owner/repo=/path/to/local/repo'`

Example static file server:

```bash
python3 -m http.server 18081 --bind 127.0.0.1 --directory /path/to/artifacts
```

Example user CLI invocation against the local artifacts:

```bash
cd go
FONTPUB_BASE_URL=http://127.0.0.1:18081 \
FONTPUB_STATE_DIR=/tmp/fontpub-user-state \
FONTPUB_ACTIVATION_DIR=/tmp/fontpub-user-fonts \
FONTPUB_DEV_LOCAL_REPO_MAP='owner/repo=/path/to/local/repo' \
go run ./cmd/fontpub install owner/repo --activate --json
```

If you want to run the whole local-only flow in one command, use the helper script:

```bash
tools/scripts/local-dev-e2e.sh --package-id owner/repo --repo /path/to/local/repo --tag 1.002 --keep
```

The script generates a temporary dev JWT, runs the local indexer, serves the generated artifacts, and exercises `fontpub ls-remote/show/install/ls/verify` against them.

## Run Tests

From the Go module root:

```bash
cd go
go test ./...
```

## Related Docs

- [Overview](./overview.md)
- [CLI spec](./cli.md)
- [CLI JSON](./cli-json.md)
- [Publisher workflow](./publisher-workflow.md)
- [Architecture](./architecture.md)
