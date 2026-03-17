# CLI JSON Output — v1

This document defines the machine-readable JSON contract for the Fontpub CLI.

It applies whenever a command is invoked with `--json`.

## General rules

- Stdout MUST contain exactly one JSON object.
- Human-readable tables, prompts, progress bars, and prose MUST NOT be mixed into stdout.
- The top-level object MUST contain:
  - `schema_version`
  - `ok`
  - `command`
- On success, the top-level object MUST contain `data`.
- On failure, the top-level object MUST contain `error`.
- `schema_version` MUST equal `"1"`.

## Success shape

```json
{
  "schema_version": "1",
  "ok": true,
  "command": "string",
  "data": {}
}
```

## Failure shape

```json
{
  "schema_version": "1",
  "ok": false,
  "command": "string",
  "error": {
    "code": "STRING_ENUM",
    "message": "string",
    "details": {}
  }
}
```

## Command result shapes

### `fontpub list --json`

```json
{
  "schema_version": "1",
  "ok": true,
  "command": "list",
  "data": {
    "packages": [
      {
        "package_id": "owner/repo",
        "latest_version": "1.2.3",
        "latest_version_key": "1.2.3",
        "latest_published_at": "RFC3339 timestamp"
      }
    ]
  }
}
```

### `fontpub show --json`

`data` MUST be the fetched package detail document defined in `indexes.md`.

### `fontpub status --json`

```json
{
  "schema_version": "1",
  "ok": true,
  "command": "status",
  "data": {
    "packages": {
      "owner/repo": {
        "installed_versions": ["1.2.3"],
        "active_version_key": "1.2.3"
      }
    }
  }
}
```

### `fontpub verify --json`

```json
{
  "schema_version": "1",
  "ok": true,
  "command": "verify",
  "data": {
    "packages": [
      {
        "package_id": "owner/repo",
        "ok": true,
        "findings": []
      }
    ]
  }
}
```

Each finding object MUST contain:
- `code`
- `message`
- `details`

If any package has one or more findings, the command MUST return a failure result (`ok: false`) even though package-level findings remain machine-readable in `error.details`.

On failure, `error.details` MUST include:
- `packages`: array of package verification results

Each package verification result in `error.details.packages` MUST contain:
- `package_id`
- `ok`
- `findings`

### Mutating commands with `--dry-run --json`

For mutating commands, `data` MUST include:
- `changed`: boolean
- `planned_actions`: array

Example:

```json
{
  "schema_version": "1",
  "ok": true,
  "command": "install",
  "data": {
    "changed": true,
    "planned_actions": [
      {
        "type": "download_asset",
        "package_id": "owner/repo",
        "version_key": "1.2.3",
        "path": "dist/ExampleSans-Regular.otf"
      }
    ]
  }
}
```

### `fontpub package init --json`

`data` MUST contain a candidate manifest:

```json
{
  "schema_version": "1",
  "ok": true,
  "command": "package init",
  "data": {
    "manifest": {
      "name": "string",
      "author": "string",
      "version": "string",
      "license": "OFL-1.1",
      "files": []
    }
  }
}
```

### `fontpub package preview --json`

`data` MUST contain a candidate package detail object as defined in `candidate-package-detail.md`.

Because no publication has occurred yet:
- `published_at` MUST be omitted
- preview output MUST NOT be described as byte-identical to a published package detail document

### `fontpub repair --json`

On success, `data` MUST include:
- `changed`: boolean
- `planned_actions` when `--dry-run` is set
- `repaired_packages`: array of repaired package IDs

On failure, `error.details` SHOULD include:
- `packages`: array of package repair results with findings

## Stability requirements

- Field names defined in this document are part of the CLI's machine-readable contract.
- Implementations MUST NOT add undocumented top-level fields.
- Implementations MUST NOT add undocumented fields to command-specific objects defined in this document or in `candidate-package-detail.md`.
- Future incompatible changes require a new CLI JSON schema version.
