# Glossary

- **Package**: A distributable unit identified by a GitHub repository: `owner/repo`.
- **Package ID**: Canonical identifier for a package. v1: `owner/repo` (lowercased for storage/lookup).
- **Manifest**: `fontpub.json` in the repository root, describing version, license, and asset list.
- **Version string**: The literal version string stored in `fontpub.json`.
- **Version key**: The canonical form of a version string used for ordering, uniqueness, and lookup.
- **Asset**: A font binary file referenced by the manifest (plus computed metadata such as SHA-256).
- **Indexer**: The Fontpub service that notarizes packages by publishing metadata indexes.
- **Root index**: `/v1/index.json`, a lightweight list of known packages and their latest versions.
- **Package versions index**: `/v1/packages/{owner}/{repo}/index.json`, the list of immutable published versions for a package.
- **Latest package detail alias**: `/v1/packages/{owner}/{repo}.json`, the endpoint that returns the latest versioned package detail.
- **Versioned package detail**: `/v1/packages/{owner}/{repo}/versions/{version_key}.json`, full immutable metadata for one package version.
- **Authoritative artifact**: A published document whose contents are the canonical public record and are not derived from another public Fontpub document. In v1 this is the versioned package detail document.
- **Derived document**: A published document regenerated from authoritative versioned package detail documents, such as the root index, package versions index, and latest package detail alias.
- **Immutability**: Within a given package, a version key is immutable: if metadata, pinned source, or assets differ, the update must fail.
- **Publisher commands**: CLI commands used to create, inspect, validate, and prepare `fontpub.json` and publication workflow files.
- **End-user commands**: CLI commands used to list, install, activate, verify, repair, update, and remove packages.
