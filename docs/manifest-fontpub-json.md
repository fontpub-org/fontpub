# Manifest: `fontpub.json` — v1

Every Fontpub repository MUST include a `fontpub.json` file at repository root.

All fields are **required** in v1.

License is restricted to **OFL-1.1**.

## Schema (conceptual)

```json
{
  "name": "string",
  "author": "string",
  "version": "string (Numeric Dot; see versioning.md)",
  "license": "OFL-1.1",
  "files": [
    {
      "path": "string",
      "style": "normal | italic | oblique",
      "weight": 1-1000
    }
  ]
}
```

## Field semantics & validation

### `name` (required)
Human-friendly font family name (display name).

### `author` (required)
Human-friendly author or organization name.

### `version` (required)
Must follow the v1 Numeric Dot rules.

### `license` (required)
Must be the literal string: `OFL-1.1`.

### `files[]` (required, non-empty)
List of font files to distribute.

Each entry:

- `path`:
  - Repository-root-relative POSIX path (use `/` separators)
  - MUST NOT start with `/`
  - MUST NOT contain `..` segments
  - SHOULD NOT contain `.` segments
- `style`:
  - One of: `normal`, `italic`, `oblique`
- `weight`:
  - Integer 1–1000 (aligned with CSS font-weight range)
  - SHOULD use common weights (100, 200, …, 900) when applicable

Additional rules:
- `files[]` MUST NOT contain duplicate `path` values.
- For a given `path`, the tuple `(style, weight)` is metadata used by clients; it does not affect integrity checks beyond immutability rules.

## Example

```json
{
  "name": "Example Sans",
  "author": "Example Studio",
  "version": "1.2.3",
  "license": "OFL-1.1",
  "files": [
    { "path": "dist/ExampleSans-Regular.otf", "style": "normal", "weight": 400 },
    { "path": "dist/ExampleSans-Italic.otf", "style": "italic", "weight": 400 },
    { "path": "dist/ExampleSans-Bold.otf", "style": "normal", "weight": 700 }
  ]
}
```
