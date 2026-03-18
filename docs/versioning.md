# Versioning (Numeric Dot) — v1

Fontpub v1 uses a numeric dotted version format that is compatible with common open font versioning practices such as `1.002` and `1.500`.

## Accepted format

A version string MUST match:

- Optional leading `v` or `V`
- One or more numeric segments separated by `.`

Examples (valid):
- `1`
- `1.0`
- `1.002`
- `1.2.3`
- `v2.10.0`
- `0.100`

Examples (invalid):
- `01.2` (the major segment MUST NOT have a leading zero)
- `1.2-alpha` (pre-release not supported)
- `1..2`
- `1.2.`

## Leading zero rule

The first numeric segment (the major version) MUST be either:
- exactly `"0"`, or
- start with `[1-9]` (no leading zeros)

Subsequent numeric segments MAY contain leading zeros.

Thus:
- `0.100` is allowed
- `1.002` is allowed
- `1.02` is allowed
- `01.2` is forbidden

## Version key

Fontpub distinguishes between:

- **Version string**: the literal string stored in `fontpub.json`
- **Version key**: the canonical identifier derived from the version string for comparisons, uniqueness, and lookup

To derive a version key:

1. Remove a leading `v` or `V` if present.
2. Preserve the remaining numeric segments exactly as written.

Examples:
- `1` -> `1`
- `1.0` -> `1.0`
- `1.0.0` -> `1.0.0`
- `1.002` -> `1.002`
- `v2.10.0` -> `2.10.0`
- `0.100` -> `0.100`

## Comparison and identity

To compare versions:

1. Remove a leading `v` or `V` if present.
2. Parse every numeric segment as a base-10 integer.
3. Compare segments left-to-right as integers.
4. Missing trailing segments are treated as `0`.

Examples:
- `1` == `1.0` == `1.0.0`
- `1.500` > `1.5`
- `1.002` == `1.2`
- `1.2.3` < `1.2.10`
- `2.0` > `1.999.999`

Two version strings identify the same published package version if and only if they produce the same version key.

Two distinct version strings MAY compare equal while still producing different version keys. For example, `1.002` and `1.2` have equal precedence but different version keys.

## On-wire rules

- Producers MUST preserve the manifest's literal version string in published JSON documents.
- Implementations MUST use the version key for:
  - uniqueness within a package
  - version ordering
  - historical version lookup paths
- A package MUST NOT publish two distinct immutable documents whose version keys are equal.
- A package SHOULD avoid publishing two distinct immutable documents whose version strings compare equal, since that creates two different lookup paths with the same precedence.
