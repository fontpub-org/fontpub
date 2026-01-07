/**
 * Handler for POST /v1/update endpoint.
 * Processes font package updates from GitHub Actions.
 */

import type { Context } from "hono";
import { processPackageUpdate } from "../assets";
import { fetchManifest, ManifestError } from "../manifest";
import {
  extractBearerToken,
  OIDCVerificationError,
  verifyGitHubOIDCToken,
} from "../oidc";
import { OwnershipError, verifyOrRegisterOwnership } from "../ownership";
import { getRootIndex, putPackageDetail, putRootIndex } from "../storage";
import type {
  Env,
  ErrorResponse,
  FontpubManifest,
  GitHubOIDCClaims,
  PackageDetail,
  PackageSummary,
  UpdateResponse,
} from "../types";
import { compareVersions } from "../version";

// Expected audience for OIDC tokens
const EXPECTED_AUDIENCE = "https://fontpub.org";

/**
 * Handle POST /v1/update request.
 * Verifies OIDC token, fetches manifest, hashes assets, and updates index.
 */
export async function handlePostUpdate(
  c: Context<{ Bindings: Env }>,
): Promise<Response> {
  // 1. Extract and verify OIDC token
  const authHeader = c.req.header("Authorization");
  const token = extractBearerToken(authHeader ?? null);

  if (!token) {
    return c.json<ErrorResponse>(
      {
        error: "Unauthorized",
        details:
          "Missing or invalid Authorization header. Expected: Bearer <token>",
      },
      401,
    );
  }

  let claims: GitHubOIDCClaims;
  try {
    claims = await verifyGitHubOIDCToken(token, EXPECTED_AUDIENCE);
  } catch (error) {
    if (error instanceof OIDCVerificationError) {
      return c.json<ErrorResponse>(
        {
          error: "Token verification failed",
          details: `${error.code}: ${error.message}`,
        },
        401,
      );
    }
    return c.json<ErrorResponse>(
      {
        error: "Token verification failed",
        details: error instanceof Error ? error.message : "Unknown error",
      },
      401,
    );
  }

  const { repository, sha, sub } = claims;

  // 2. Verify ownership
  try {
    const { isNewRegistration } = await verifyOrRegisterOwnership(
      c.env.OWNERSHIP_KV,
      repository,
      sub,
    );
    if (isNewRegistration) {
      console.log(`New package registered: ${repository}`);
    }
  } catch (error) {
    if (error instanceof OwnershipError) {
      return c.json<ErrorResponse>(
        {
          error: "Ownership verification failed",
          details: error.message,
        },
        403,
      );
    }
    throw error;
  }

  // 3. Fetch and validate manifest
  let manifest: FontpubManifest;
  try {
    manifest = await fetchManifest(repository, sha);
  } catch (error) {
    if (error instanceof ManifestError) {
      return c.json<ErrorResponse>(
        {
          error: "Manifest error",
          details: `${error.code}: ${error.message}`,
        },
        400,
      );
    }
    return c.json<ErrorResponse>(
      {
        error: "Failed to fetch manifest",
        details: error instanceof Error ? error.message : "Unknown error",
      },
      500,
    );
  }

  // 4. Check for version immutability
  const [owner, repo] = repository.split("/");
  const existingIndex = await getRootIndex(c.env.INDEX_BUCKET);
  const existingPackage = existingIndex?.data.packages[repository];

  if (existingPackage) {
    const comparison = compareVersions(
      manifest.version,
      existingPackage.latest_version,
    );

    // Same version - reject (immutability rule)
    if (comparison === 0) {
      return c.json<ErrorResponse>(
        {
          error: "Version already exists",
          details: `Version ${manifest.version} is already published. Bump the version to publish changes.`,
        },
        409,
      );
    }

    // Older version - reject
    if (comparison < 0) {
      return c.json<ErrorResponse>(
        {
          error: "Version too old",
          details: `Version ${manifest.version} is older than current version ${existingPackage.latest_version}.`,
        },
        400,
      );
    }
  }

  // 5. Process assets (fetch and hash)
  let packageDetail: PackageDetail;
  try {
    packageDetail = await processPackageUpdate(repository, sha, manifest);
  } catch (error) {
    return c.json<ErrorResponse>(
      {
        error: "Asset processing failed",
        details: error instanceof Error ? error.message : "Unknown error",
      },
      500,
    );
  }

  // 6. Write package detail to R2
  try {
    await putPackageDetail(c.env.INDEX_BUCKET, owner, repo, packageDetail);
  } catch (error) {
    return c.json<ErrorResponse>(
      {
        error: "Failed to write package detail",
        details: error instanceof Error ? error.message : "Unknown error",
      },
      500,
    );
  }

  // 7. Update root index with ETag-based conditional write
  const maxRetries = 3;
  let retryCount = 0;

  while (retryCount < maxRetries) {
    // Re-read index to get fresh ETag
    const currentIndex = await getRootIndex(c.env.INDEX_BUCKET);
    const etag = currentIndex?.etag ?? undefined;

    // Prepare updated index
    const updatedPackages = { ...(currentIndex?.data.packages ?? {}) };
    const packageSummary: PackageSummary = {
      latest_version: manifest.version,
      last_updated: new Date().toISOString(),
    };
    updatedPackages[repository] = packageSummary;

    const newIndex = { packages: updatedPackages };

    // Attempt conditional write
    const writeSuccess = await putRootIndex(c.env.INDEX_BUCKET, newIndex, etag);

    if (writeSuccess) {
      // Success!
      return c.json<UpdateResponse>({
        success: true,
        message: `Successfully published ${manifest.name} v${manifest.version}`,
        package: repository,
        version: manifest.version,
      });
    }

    // ETag mismatch - retry
    retryCount++;
    console.log(
      `Root index write conflict, retrying (${retryCount}/${maxRetries})...`,
    );

    // Small delay before retry
    await new Promise((resolve) => setTimeout(resolve, 100 * retryCount));
  }

  // All retries exhausted
  return c.json<ErrorResponse>(
    {
      error: "Concurrent update conflict",
      details:
        "Unable to update index due to concurrent modifications. Please retry.",
    },
    503,
  );
}
