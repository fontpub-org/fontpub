/**
 * Type definitions for Fontpub Indexer.
 */

// ============================================================
// Root Index Schema (/v1/index.json)
// ============================================================

/**
 * Summary information for a package in the root index.
 */
export interface PackageSummary {
  latest_version: string;
  last_updated: string; // ISO8601
}

/**
 * Root index containing all package summaries.
 */
export interface RootIndex {
  packages: Record<string, PackageSummary>;
}

// ============================================================
// Package Detail Schema (/v1/packages/{owner}/{repo}.json)
// ============================================================

/**
 * A font file asset within a package.
 */
export interface Asset {
  path: string;
  url: string;
  sha256: string;
  style?: string;
  weight?: number;
  format?: string;
}

/**
 * Detailed information for a specific package.
 */
export interface PackageDetail {
  name: string;
  version: string;
  github_sha: string;
  assets: Asset[];
}

// ============================================================
// fontpub.json Manifest Schema (in font repositories)
// ============================================================

/**
 * A file entry in the fontpub.json manifest.
 */
export interface ManifestFile {
  path: string;
  style?: string;
  weight?: number;
}

/**
 * The fontpub.json manifest schema.
 */
export interface FontpubManifest {
  name: string;
  author: string;
  version: string;
  license: string;
  files: ManifestFile[];
}

// ============================================================
// GitHub OIDC Claims
// ============================================================

/**
 * Relevant claims from GitHub Actions OIDC token.
 */
export interface GitHubOIDCClaims {
  iss: string; // https://token.actions.githubusercontent.com
  aud: string | string[];
  sub: string; // Unique identifier for the repository
  repository: string; // owner/repo
  sha: string; // Commit SHA
  ref: string; // refs/heads/main or refs/tags/v1.0.0
  repository_owner: string;
  actor: string;
}

// ============================================================
// KV Ownership Record
// ============================================================

/**
 * Ownership record stored in KV.
 */
export interface OwnershipRecord {
  owner_sub: string;
  registered_at: string; // ISO8601
}

// ============================================================
// Cloudflare Workers Bindings
// ============================================================

/**
 * Environment bindings for the Worker.
 */
export interface Env {
  OWNERSHIP_KV: KVNamespace;
  INDEX_BUCKET: R2Bucket;
  FONTPUB_AUDIENCE: string;
}

// ============================================================
// API Response Types
// ============================================================

export interface UpdateResponse {
  success: boolean;
  message: string;
  package?: string;
  version?: string;
}

export interface ErrorResponse {
  error: string;
  details?: string;
}
