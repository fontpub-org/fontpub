# Fontpub CLI (macOS) — v1

The CLI installs and activates fonts on macOS using the Indexer metadata.

This document defines CLI behavior and on-disk layout, not the exact command-line parser flags.

## Commands (conceptual)

- `fontpub list`
  - Fetch `/v1/index.json` (use ETag)
  - Print packages and latest versions

- `fontpub install <owner>/<repo> [--version <v>]`
  - Fetch root index (optional, for existence/latest)
  - If `--version` is omitted, fetch package detail `/v1/packages/<owner>/<repo>.json`
  - If `--version` is provided, normalize it to a version key and fetch `/v1/packages/<owner>/<repo>/versions/<version_key>.json`
  - Download each `asset.url`
  - Verify SHA-256 matches `asset.sha256`
  - Store files under `~/.fontpub/packages/...`
  - Record in lockfile

- `fontpub activate <owner>/<repo> [--version <v>]`
  - Create symlinks into `~/Library/Fonts/from_fontpub/`

- `fontpub deactivate <owner>/<repo>`
  - Remove symlinks for that package

- `fontpub update`
  - For installed packages, compare installed `version_key` with the root index latest version key
  - Install newer versions and (optionally) re-activate if currently active

- `fontpub uninstall <owner>/<repo> [--version <v>|--all]`
  - Remove installed files and lockfile entries
  - If active, deactivate first

- `fontpub status`
  - Show installed packages and activation status

## On-disk layout

- Base directory: `~/.fontpub/`
- Packages:
  - `~/.fontpub/packages/<owner>/<repo>/<version_key>/...`
- Lockfile:
  - `~/.fontpub/fontpub.lock`

## Activation directory (macOS)

- `~/Library/Fonts/from_fontpub/`

Activation is implemented by symlinks into installed package files.

Symlink naming:
- `{owner}--{repo}--{filename}`
- where `filename` is the basename of the asset path.

If a symlink name would collide:
- The CLI MUST make the name unique by appending `--<shortsha>` where `shortsha` is the first 8 chars of the asset SHA-256.

## Atomic activation updates

To avoid transient “missing font” states:
- The CLI SHOULD create a temporary symlink and then `rename()` it into place.

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
              "symlink_path": "/Users/<you>/Library/Fonts/from_fontpub/owner--repo--ExampleSans-Regular.otf"
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
- CLI flags or user inputs that name a version MUST accept any valid version string and normalize it to a version key before lookup.
- `assets[].active` MUST reflect whether the symlink exists (or desired state if repairing).
- CLI MUST update lockfile atomically (write temp file, fsync if feasible, rename).
