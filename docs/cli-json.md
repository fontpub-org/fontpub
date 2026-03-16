# CLI JSON Output — v1

This document defines the machine-readable JSON contract for the Fontpub CLI.

It applies whenever a command is invoked with `--json`.

## General rules

- Stdout MUST contain exactly one JSON object.
- Human-readable tables, prompts, progress bars, and prose MUST NOT be mixed into stdout.
- The top-level object MUST contain:
  - `ok`
  - `command`
- On success, the top-level object MUST contain `data`.
- On failure, the top-level object MUST contain `error`.

## Success shape

```json
{
  "ok": true,
  "command": "string",
  "data": {}
}
```

## Failure shape

```json
{
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

`data` MUST be the fetched package detail document.

### `fontpub status --json`

```json
{
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

Each finding object SHOULD include:
- `code`
- `message`
- `details`

### Mutating commands with `--dry-run --json`

For mutating commands, `data` SHOULD include:
- `changed`: boolean
- `planned_actions`: array

Example:

```json
{
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

`data` MUST contain a candidate package detail object.

Because no publication has occurred yet:
- `published_at` MUST be omitted or `null`
- preview output MUST NOT be described as byte-identical to a published package detail document

## Stability requirements

- Field names defined in this document are part of the CLI's machine-readable contract.
- Commands MAY include additional fields in `data` or `details` only if they do not change the meaning of existing fields.
