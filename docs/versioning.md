# Versioning (Numeric Dot) — v1

Fontpub v1 uses a strict numeric dotted version format.

## Accepted format

A version string MUST match:

- Optional leading `v` or `V`
- One or more numeric segments separated by `.`

Examples (valid):
- `1`
- `1.0`
- `1.2.3`
- `v2.10`
- `0.100`

Examples (invalid):
- `01.2` (leading zeros in a non-zero segment are not allowed)
- `1.02`
- `1.2-alpha` (pre-release not supported)
- `1..2`
- `1.2.`

## Leading zero rule

Each numeric segment MUST be either:
- exactly `"0"`, or
- start with `[1-9]` (no leading zeros)

Thus:
- `0.100` is allowed (segment `"100"` has no leading zeros)
- `01.2` is forbidden (segment `"01"` violates the rule)

## Version key

Fontpub distinguishes between:

- **Version string**: the literal string stored in `fontpub.json`
- **Version key**: the canonical identifier derived from the version string for comparisons, uniqueness, and lookup

To derive a version key:

1. Remove a leading `v` or `V` if present.
2. Split by `.` into numeric segments.
3. Remove trailing zero segments until either:
   - a non-zero segment remains at the end, or
   - exactly one segment remains.
4. Join the remaining segments with `.`.

Examples:
- `1` -> `1`
- `1.0` -> `1`
- `1.0.0` -> `1`
- `v2.10.0` -> `2.10`
- `0.100` -> `0.100`

## Comparison and identity

To compare versions:

1. Convert both version strings to integer segment lists after removing a leading `v` or `V`.
2. Compare segment-by-segment as integers.
3. Missing segments are treated as `0`.

Examples:
- `1` == `1.0` == `1.0.0`
- `1.500` > `1.5`
- `2.0` > `1.999.999`

Two version strings identify the same package version if and only if they produce the same version key.

## On-wire rules

- Producers MUST preserve the manifest's literal version string in published JSON documents.
- Implementations MUST use the version key for:
  - uniqueness within a package
  - version ordering
  - historical version lookup paths
- A package MUST NOT publish two distinct immutable documents whose version strings differ but whose version keys are equal.
