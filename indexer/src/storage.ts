/**
 * R2 Storage operations for index data.
 * Handles reading and writing JSON files with ETag support for conditional writes.
 */

import type { RootIndex, PackageDetail } from "./types";

// R2 object paths
export const INDEX_PATH = "v1/index.json";
export const PACKAGES_PREFIX = "v1/packages/";

/**
 * Result of a read operation with ETag for conditional writes.
 */
export interface ReadResult<T> {
  data: T;
  etag: string | null;
}

/**
 * Get the root index from R2.
 * Returns null if the index doesn't exist yet.
 */
export async function getRootIndex(
  bucket: R2Bucket
): Promise<ReadResult<RootIndex> | null> {
  const object = await bucket.get(INDEX_PATH);

  if (!object) {
    return null;
  }

  const data = (await object.json()) as RootIndex;
  return {
    data,
    etag: object.etag,
  };
}

/**
 * Put the root index to R2 with optional conditional write.
 * @param bucket R2 bucket
 * @param index Index data to write
 * @param expectedEtag If provided, only write if ETag matches (for concurrent updates)
 * @returns true if write succeeded, false if ETag mismatch
 */
export async function putRootIndex(
  bucket: R2Bucket,
  index: RootIndex,
  expectedEtag?: string
): Promise<boolean> {
  const body = JSON.stringify(index, null, 2);

  const options: R2PutOptions = {
    httpMetadata: {
      contentType: "application/json",
    },
  };

  // Conditional write based on ETag
  if (expectedEtag) {
    options.onlyIf = {
      etagMatches: expectedEtag,
    };
  }

  const result = await bucket.put(INDEX_PATH, body, options);

  // If result is null, the condition failed (ETag mismatch)
  return result !== null;
}

/**
 * Create a new empty root index.
 */
export function createEmptyIndex(): RootIndex {
  return {
    packages: {},
  };
}

/**
 * Get the R2 path for a package detail file.
 */
export function getPackagePath(owner: string, repo: string): string {
  return `${PACKAGES_PREFIX}${owner}/${repo}.json`;
}

/**
 * Get a package detail from R2.
 * Returns null if the package doesn't exist.
 */
export async function getPackageDetail(
  bucket: R2Bucket,
  owner: string,
  repo: string
): Promise<ReadResult<PackageDetail> | null> {
  const path = getPackagePath(owner, repo);
  const object = await bucket.get(path);

  if (!object) {
    return null;
  }

  const data = (await object.json()) as PackageDetail;
  return {
    data,
    etag: object.etag,
  };
}

/**
 * Put a package detail to R2.
 */
export async function putPackageDetail(
  bucket: R2Bucket,
  owner: string,
  repo: string,
  detail: PackageDetail
): Promise<void> {
  const path = getPackagePath(owner, repo);
  const body = JSON.stringify(detail, null, 2);

  await bucket.put(path, body, {
    httpMetadata: {
      contentType: "application/json",
    },
  });
}

/**
 * Delete a package detail from R2.
 */
export async function deletePackageDetail(
  bucket: R2Bucket,
  owner: string,
  repo: string
): Promise<void> {
  const path = getPackagePath(owner, repo);
  await bucket.delete(path);
}

