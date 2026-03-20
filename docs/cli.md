# Fontpub CLI — v1

The Fontpub CLI is used by both humans and software agents.

Accordingly, the CLI must support:
- concise interactive workflows for humans
- deterministic, non-interactive, machine-readable workflows for tools such as Codex or Claude Code

This document defines command behavior and on-disk layout.

## Interaction model

### Help output

The CLI MUST support help output via `--help`.

At minimum, the following forms MUST be supported:
- `fontpub --help`
- `fontpub <command> --help`
- `fontpub package --help`
- `fontpub package <subcommand> --help`
- `fontpub workflow --help`
- `fontpub workflow <subcommand> --help`

Help output is human-oriented usage text.

Rules:
- `--help` MUST print usage information for the selected command or command group and exit successfully without performing the command.
- `--help` MUST write human-readable help text to stdout.
- `--help` MUST take precedence over normal command execution.
- `--help` output is not part of the CLI JSON contract and MUST NOT require `--json`.
- Implementations MAY support `help` as an alias, but `--help` is the normative interface in v1.
- Help output SHOULD include short command descriptions.
- Help output SHOULD include representative examples for command groups and subcommands.
- Top-level help SHOULD mention the main environment variables that affect CLI behavior.

### Human-oriented behavior

When stdout and stderr are attached to a TTY, commands MAY present:
- concise human-readable tables
- prompts for missing required information
- confirmation prompts before destructive actions

### Agent-oriented behavior

For automation and AI use:
- every read command MUST support `--json`
- every mutating command MUST support `--dry-run`
- every mutating command that would otherwise prompt for confirmation MUST support `--yes`
- commands that require user input MUST fail instead of prompting when no TTY is available, unless all required inputs were provided explicitly
- `--json` output MUST be stable and machine-readable

### Exit behavior

- Exit code `0` means success.
- A non-zero exit code means failure.
- When `--json` is set, failures MUST still be emitted as JSON.

## Command groups

The CLI has two top-level command groups:

- end-user package management commands
- publisher workflow commands

## End-user commands

### `fontpub list`

- Fetch `/v1/index.json` using `ETag`
- Print available packages and latest versions
- MUST support `--json`
- In human-readable mode, `list` SHOULD emphasize package ID, latest version, and published date in a scannable layout

### `fontpub show <owner>/<repo> [--version <v>]`

- Fetch:
  - `/v1/packages/<owner>/<repo>.json`, or
  - `/v1/packages/<owner>/<repo>/versions/<version_key>.json` if `--version <v>` is provided
- Show package metadata and assets
- MUST support `--json`
- In human-readable mode, `show` SHOULD summarize package metadata before listing assets

### `fontpub install <owner>/<repo> [--version <v>] [--activate] [--activation-dir <path>]`

- Fetch the root index
- If `--version` is omitted, fetch `/v1/packages/<owner>/<repo>.json`
- If `--version` is provided, normalize it to a version key and fetch `/v1/packages/<owner>/<repo>/versions/<version_key>.json`
- Download each `asset.url`
- Verify SHA-256 matches `asset.sha256`
- Store files under `~/.fontpub/packages/...`
- Record the installation in the lockfile
- If `--activate` is set, activate the installed version after a successful install using the effective activation directory
- MUST support `--dry-run`
- MUST support `--json`

### `fontpub activate <owner>/<repo> [--version <v>] [--activation-dir <path>]`

- Activate the selected installed version by creating symlinks in the activation target directory
- If `--version` is omitted:
  - activate the only installed version if exactly one installed version exists
  - otherwise activate the installed highest-precedence version if exactly one installed version has that precedence
  - otherwise fail and require `--version`
- MUST support `--dry-run`
- MUST support `--json`

### `fontpub deactivate <owner>/<repo> [--activation-dir <path>]`

- Remove activation symlinks for the package's active version
- MUST support `--dry-run`
- MUST support `--json`

### `fontpub update [<owner>/<repo>] [--activate] [--activation-dir <path>]`

- If no package is specified:
  - examine all installed packages
- If a package is specified:
  - examine only that package
- Compare installed `version_key` values with the root index latest version key
- Install newer versions when available
- If `--activate` is set, activate the newly installed version using the effective activation directory
- If `--activate` is not set, preserve current activation state
- MUST support `--dry-run`
- MUST support `--json`

### `fontpub uninstall <owner>/<repo> [--version <v> | --all] [--activation-dir <path>]`

- Remove installed files and lockfile entries
- If the target version is active, deactivate it first
- If neither `--version` nor `--all` is provided:
  - remove the only installed version if exactly one installed version exists
  - otherwise fail and require explicit scope
- MUST support `--dry-run`
- MUST support `--yes`
- MUST support `--json`

### `fontpub status [<owner>/<repo>] [--activation-dir <path>]`

- Show installed packages, installed versions, active version, and activation state
- If a package is specified, limit output to that package
- MUST support `--json`
- In human-readable mode, `status` SHOULD identify the effective activation directory used for activation-state evaluation

### `fontpub verify [<owner>/<repo>] [--activation-dir <path>]`

- Verify local installation state against the lockfile
- Verify that installed asset files exist and match recorded SHA-256 values
- Verify that active symlinks exist and point to the expected installed files
- If a package is specified, limit verification to that package
- MUST support `--json`

### `fontpub repair [<owner>/<repo>] [--activation-dir <path>]`

- Repair local state without changing the selected remote package version
- Repair means reconciling:
  - lockfile entries
  - missing or stale activation symlinks
  - `assets[].active` flags
- `repair` MUST NOT silently install a different version from the network
- `repair` MUST be local-only and MUST NOT fetch package metadata or asset bytes from the network
- If an asset is marked active and the installed file exists with the recorded hash, `repair` MUST recreate or correct the expected activation symlink
- `repair` MUST remove stale or incorrect activation symlinks for the selected package
- `repair` MUST clear incorrect `assets[].active` flags
- After reconciliation, `repair` MUST set `active_version_key` to the version that still has one or more active assets, or clear it if none remain active
- `repair` MUST NOT delete installed asset files
- `repair` MUST NOT modify recorded asset hashes
- If a required installed asset file is missing or its bytes do not match the recorded hash, `repair` MUST report the package as unrepaired and exit with failure
- If a package is specified, limit repair to that package
- MUST support `--dry-run`
- MUST support `--yes`
- MUST support `--json`

## Publisher commands

### `fontpub package init [PATH]`

- If `PATH` is omitted, the selected repository root MUST default to the current working directory
- Recursively scan the selected repository root for distributable font files
- The scan MUST ignore the `.git/` directory and MUST consider only files with allowed Fontpub asset extensions
- Candidate asset paths MUST be repository-root-relative POSIX paths sorted by `path` ascending
- Infer candidate `files[]` entries from discovered assets
- Infer candidate `name`, `style`, and `weight` values when possible
- For each asset, the CLI MUST prefer embedded font metadata over filename heuristics when both are available
- If required manifest values cannot be determined unambiguously:
  - interactive mode MAY prompt
  - non-interactive mode MUST fail with `INPUT_REQUIRED`
- In human-readable mode, the CLI MUST summarize:
  - discovered asset files
  - inferred manifest fields
  - conflicting candidate values, if any
  - unresolved required fields, if any
- Ask for missing required manifest fields when running interactively
- Output a candidate `fontpub.json`
- If `--write` is set, write the candidate manifest to `fontpub.json`
- If `--write` would overwrite an existing `fontpub.json`, the CLI MUST require confirmation unless `--yes` is set
- If `--write` is not set, print the candidate manifest
- MUST support `--json`
- MUST support `--dry-run`
- MUST support `--yes`

### `fontpub package validate [PATH]`

- If `PATH` is omitted, the selected repository root MUST default to the current working directory
- Validate `fontpub.json` against the current spec
- Verify that all declared files exist
- Verify path, version, license, and file-entry constraints
- MUST support `--json`
- In human-readable mode, `validate` SHOULD summarize the manifest path, root path, checked file count, and version

### `fontpub package preview [PATH] [--package-id <owner>/<repo>]`

- Render a candidate package detail object as defined in `candidate-package-detail.md`
- If `PATH` is omitted, the selected repository root MUST default to the current working directory
- Preview is derived from the current local repository state rooted at `PATH`
- If `--package-id` is provided, the CLI MUST use it as the package identity after validation and normalization
- If `--package-id` is omitted, the CLI MUST derive the canonical GitHub `owner/repo` identity from local Git metadata using the rules in `candidate-package-detail.md`
- MUST NOT publish anything
- preview output MUST NOT be treated as byte-identical to a published versioned package detail document
- MUST support `--json`
- In human-readable mode, `preview` SHOULD summarize package identity, version, asset count, and root path before listing assets

### `fontpub package inspect <font-file>`

- Inspect a font file and print metadata useful for manifest generation
- MAY include family name, style, weight, and format inference
- MUST support `--json`

### `fontpub package check [--tag <tag>]`

- Validate that the current repository is ready for publication
- This includes:
  - manifest validity
  - file existence
  - tag/version consistency if `--tag <tag>` was provided
- MUST support `--json`
- In human-readable mode, `check` SHOULD summarize the root path, manifest path, checked file count, version, and tag when provided

### `fontpub workflow init`

- Generate a starter `.github/workflows/fontpub.yml`
- MUST support `--dry-run`
- MUST support `--yes`
- MUST support `--json`

## Output requirements

### Human-readable output

Human-readable output should be concise and directly actionable.

For mutating commands in human-readable mode:
- success output SHOULD identify the affected package ID and version when applicable
- success output SHOULD summarize the material local changes that occurred, such as assets written, symlinks created or removed, and files written
- `--dry-run` output SHOULD clearly indicate that it is a plan and SHOULD include the planned actions
- when no local change is required, the output SHOULD say why, not only that nothing changed
- `repair` output SHOULD summarize what was reconciled, including symlink creation or removal counts when relevant

For failures in human-readable mode:
- error output SHOULD include the relevant structured details when available, such as `path`, `package_id`, `version_key`, or `flag`
- error output SHOULD include a concise next step when the CLI can determine one
- `verify` and `repair` failure output SHOULD include finding-specific details such as `local_path`, `symlink_path`, and `reason` when available

### JSON output

When `--json` is set:
- output MUST be valid JSON
- output MUST be a single JSON object
- output MUST be stable enough for programmatic consumption
- commands MUST NOT mix human-readable tables or prose into stdout
- CLI JSON output is specified in `cli-json.md`

## On-disk layout

- Base directory: `~/.fontpub/`
- Packages:
  - `~/.fontpub/packages/<owner>/<repo>/<version_key>/...`
- Lockfile:
  - `~/.fontpub/fontpub.lock`

## Activation directory

- Commands that read or modify activation state (`activate`, `deactivate`, `status`, `verify`, `repair`, `uninstall`) MUST support `--activation-dir <path>`.
- Commands that can immediately activate as part of another operation (`install --activate`, `update --activate`) MUST also support `--activation-dir <path>`.
- When `--activation-dir` is provided, activation behavior is defined entirely against that directory.
- Implementations MAY provide a platform default activation directory when `--activation-dir` is omitted.

The effective activation directory is:
1. the path passed to `--activation-dir`, if provided
2. otherwise the implementation default activation directory

Activation is implemented by symlinks into installed package files.

Symlink naming:
- `{owner}--{repo}--{filename}`
- where `filename` is the basename of the asset path.

Activation safety rules:
- The CLI MUST use the validated asset basename exactly as published in the package detail.
- The CLI MUST NOT interpret asset basenames as path components, option flags, or shell fragments.
- `status`, `verify`, and `repair` MUST evaluate activation state against the effective activation directory selected by `--activation-dir` or the implementation default.

If a symlink name would collide:
- The CLI MUST make the name unique by appending `--<shortsha>` where `shortsha` is the first 8 chars of the asset SHA-256.

## Atomic activation updates

Activation updates MUST be atomic from the user's perspective.

## Lockfile schema

The lockfile is JSON.

```json
{
  "schema_version": "1",
  "generated_at": "RFC3339 timestamp",
  "packages": {
    "owner/repo": {
      "installed_versions": {
        "1.2.3": {
          "version": "v1.2.3",
          "version_key": "1.2.3",
          "installed_at": "RFC3339 timestamp",
          "assets": [
            {
              "path": "dist/ExampleSans-Regular.otf",
              "sha256": "64-hex",
              "local_path": "/Users/<you>/.fontpub/packages/owner/repo/1.2.3/dist/ExampleSans-Regular.otf",
              "active": true,
              "symlink_path": "/path/to/activation/owner--repo--ExampleSans-Regular.otf"
            }
          ]
        }
      },
      "active_version_key": "1.2.3"
    }
  }
}
```

Rules:
- `packages` keys MUST be canonical package IDs (lowercase).
- `installed_versions` keys MUST be version keys.
- Each installed version record MUST preserve both the literal `version` string and the canonical `version_key`.
- `active_version_key` MAY be null/omitted if not active.
- If `active_version_key` is present, it MUST reference an installed version key for that package.
- If `active_version_key` is present, at least one asset in that installed version MUST have `active: true`.
- If any asset in an installed version has `active: true`, `active_version_key` MUST equal that installed version's key.
- CLI flags or user inputs that name a version MUST accept any valid version string and normalize it to a version key before lookup.
- `assets[].active` MUST be `true` if and only if the expected activation symlink exists and points to the expected `local_path`.
- `symlink_path` MUST be present when `assets[].active` is `true`.
- `symlink_path` MAY be omitted or null when `assets[].active` is `false`.
- CLI MUST update the lockfile atomically.
