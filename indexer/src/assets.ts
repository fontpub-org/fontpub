/**
 * Asset processing: fetching files, computing hashes, and building asset metadata.
 */

import { fetchAndHash, verifyFileSize } from "./hash";
import { buildGitHubRawUrl, getFileExtension } from "./manifest";
import type { Asset, FontpubManifest, PackageDetail } from "./types";

/**
 * Error thrown when asset processing fails.
 */
export class AssetError extends Error {
  constructor(
    message: string,
    public readonly code: "FETCH_FAILED" | "SIZE_EXCEEDED" | "HASH_FAILED",
  ) {
    super(message);
    this.name = "AssetError";
  }
}

/**
 * Process all assets from a manifest and compute their hashes.
 * @param manifest The parsed fontpub.json manifest
 * @param repository Owner/repo format
 * @param sha Commit SHA
 * @returns Array of processed assets with hashes
 */
export async function processAssets(
  manifest: FontpubManifest,
  repository: string,
  sha: string,
): Promise<Asset[]> {
  const assets: Asset[] = [];

  for (const file of manifest.files) {
    const url = buildGitHubRawUrl(repository, sha, file.path);

    try {
      // Fetch and hash the file
      const { hash, size } = await fetchAndHash(url);

      // Verify size
      verifyFileSize(size);

      // Build asset metadata
      const asset: Asset = {
        path: file.path,
        url,
        sha256: hash,
      };

      // Add optional metadata from manifest
      if (file.style) {
        asset.style = file.style;
      }

      if (file.weight !== undefined) {
        asset.weight = file.weight;
      }

      // Determine format from extension
      const format = getFileExtension(file.path);
      if (format) {
        asset.format = format;
      }

      assets.push(asset);
    } catch (error) {
      if (error instanceof Error && error.message.includes("exceeds maximum")) {
        throw new AssetError(
          `File ${file.path} exceeds maximum size limit`,
          "SIZE_EXCEEDED",
        );
      }
      throw new AssetError(
        `Failed to process asset ${file.path}: ${error instanceof Error ? error.message : "Unknown error"}`,
        "FETCH_FAILED",
      );
    }
  }

  return assets;
}

/**
 * Build a complete PackageDetail from manifest and processed assets.
 * @param manifest The parsed fontpub.json manifest
 * @param sha Commit SHA
 * @param assets Processed assets with hashes
 * @returns Complete PackageDetail object
 */
export function buildPackageDetail(
  manifest: FontpubManifest,
  sha: string,
  assets: Asset[],
): PackageDetail {
  return {
    name: manifest.name,
    version: manifest.version,
    github_sha: sha,
    assets,
  };
}

/**
 * Process a complete package update: fetch manifest, hash assets, build detail.
 * @param repository Owner/repo format
 * @param sha Commit SHA
 * @param manifest Pre-fetched manifest (to avoid double fetch)
 * @returns Complete PackageDetail ready for storage
 */
export async function processPackageUpdate(
  repository: string,
  sha: string,
  manifest: FontpubManifest,
): Promise<PackageDetail> {
  const assets = await processAssets(manifest, repository, sha);
  return buildPackageDetail(manifest, sha, assets);
}
