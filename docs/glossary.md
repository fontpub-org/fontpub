# Glossary

- **Package**: A distributable unit identified by a GitHub repository: `owner/repo`.
- **Package ID**: Canonical identifier for a package. v1: `owner/repo` (lowercased for storage/lookup).
- **Manifest**: `fontpub.json` in the repository root, describing version, license, and asset list.
- **Asset**: A font binary file referenced by the manifest (plus computed metadata such as SHA-256).
- **Indexer**: The Fontpub service that notarizes packages by publishing metadata indexes.
- **Root index**: `/v1/index.json`, a lightweight list of known packages and their latest versions.
- **Package detail**: `/v1/packages/{owner}/{repo}.json`, full metadata for a package and its assets.
- **Immutability**: Within a given package, a version is immutable: if metadata+assets differ, the update must fail.
