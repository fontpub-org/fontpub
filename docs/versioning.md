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

## Comparison

To compare versions:

1. Remove a leading `v` or `V` if present.
2. Split by `.` into integer segments.
3. Compare segment-by-segment as integers.
4. Missing segments are treated as `0`.

Examples:
- `1` == `1.0` == `1.0.0`
- `1.500` > `1.5`
- `2.0` > `1.999.999`
