/**
 * Ownership management using Cloudflare KV.
 * Tracks which GitHub repository (identified by `sub` claim) owns which package.
 */

import type { OwnershipRecord } from "./types";

// KV key prefix for ownership records
const OWNERSHIP_PREFIX = "owner:";

/**
 * Error thrown when ownership verification fails.
 */
export class OwnershipError extends Error {
  constructor(
    message: string,
    public readonly code: "NOT_OWNER" | "KV_ERROR",
  ) {
    super(message);
    this.name = "OwnershipError";
  }
}

/**
 * Get the KV key for a repository.
 */
function getOwnershipKey(repository: string): string {
  return `${OWNERSHIP_PREFIX}${repository}`;
}

/**
 * Get the ownership record for a repository.
 * @param kv KV namespace
 * @param repository Repository name (owner/repo format)
 * @returns Ownership record or null if not registered
 */
export async function getOwnership(
  kv: KVNamespace,
  repository: string,
): Promise<OwnershipRecord | null> {
  const key = getOwnershipKey(repository);
  const value = await kv.get(key, "json");
  return value as OwnershipRecord | null;
}

/**
 * Register ownership for a new repository.
 * @param kv KV namespace
 * @param repository Repository name (owner/repo format)
 * @param ownerSub The `sub` claim from the OIDC token
 */
export async function registerOwnership(
  kv: KVNamespace,
  repository: string,
  ownerSub: string,
): Promise<void> {
  const key = getOwnershipKey(repository);
  const record: OwnershipRecord = {
    owner_sub: ownerSub,
    registered_at: new Date().toISOString(),
  };

  await kv.put(key, JSON.stringify(record));
}

/**
 * Verify that the given `sub` is the owner of the repository.
 * If the repository is not registered, it will be registered with this `sub`.
 * @param kv KV namespace
 * @param repository Repository name (owner/repo format)
 * @param sub The `sub` claim from the OIDC token
 * @returns true if this is a new registration, false if existing owner
 * @throws OwnershipError if the `sub` doesn't match the registered owner
 */
export async function verifyOrRegisterOwnership(
  kv: KVNamespace,
  repository: string,
  sub: string,
): Promise<{ isNewRegistration: boolean }> {
  const existing = await getOwnership(kv, repository);

  if (!existing) {
    // New repository - register ownership
    await registerOwnership(kv, repository, sub);
    return { isNewRegistration: true };
  }

  // Existing repository - verify ownership
  if (existing.owner_sub !== sub) {
    throw new OwnershipError(
      `Repository ${repository} is owned by a different entity`,
      "NOT_OWNER",
    );
  }

  return { isNewRegistration: false };
}

/**
 * Delete ownership record (for testing or administrative purposes).
 * @param kv KV namespace
 * @param repository Repository name (owner/repo format)
 */
export async function deleteOwnership(
  kv: KVNamespace,
  repository: string,
): Promise<void> {
  const key = getOwnershipKey(repository);
  await kv.delete(key);
}
