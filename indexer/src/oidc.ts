/**
 * GitHub OIDC token verification.
 * Verifies JWT tokens from GitHub Actions using GitHub's public JWKS.
 */

import * as jose from "jose";
import type { GitHubOIDCClaims } from "./types";

// GitHub OIDC constants
const GITHUB_OIDC_ISSUER = "https://token.actions.githubusercontent.com";
const GITHUB_JWKS_URL =
  "https://token.actions.githubusercontent.com/.well-known/jwks";

// Cache JWKS for performance (refreshed periodically by jose)
let jwksCache: jose.JWTVerifyGetKey | null = null;

/**
 * Get or create cached JWKS getter.
 */
function getJWKS(): jose.JWTVerifyGetKey {
  if (!jwksCache) {
    jwksCache = jose.createRemoteJWKSet(new URL(GITHUB_JWKS_URL));
  }
  return jwksCache;
}

/**
 * Error thrown when OIDC verification fails.
 */
export class OIDCVerificationError extends Error {
  constructor(
    message: string,
    public readonly code:
      | "INVALID_TOKEN"
      | "INVALID_ISSUER"
      | "INVALID_AUDIENCE"
      | "EXPIRED"
      | "MISSING_CLAIMS",
  ) {
    super(message);
    this.name = "OIDCVerificationError";
  }
}

/**
 * Verify a GitHub OIDC token and extract claims.
 * @param token The JWT token (without "Bearer " prefix)
 * @param expectedAudience The expected audience claim
 * @returns Verified claims from the token
 * @throws OIDCVerificationError if verification fails
 */
export async function verifyGitHubOIDCToken(
  token: string,
  expectedAudience: string,
): Promise<GitHubOIDCClaims> {
  try {
    const jwks = getJWKS();

    // Verify the token
    const { payload } = await jose.jwtVerify(token, jwks, {
      issuer: GITHUB_OIDC_ISSUER,
      audience: expectedAudience,
    });

    // Validate required claims
    const claims = payload as unknown as GitHubOIDCClaims;

    if (!claims.repository) {
      throw new OIDCVerificationError(
        "Missing required claim: repository",
        "MISSING_CLAIMS",
      );
    }

    if (!claims.sha) {
      throw new OIDCVerificationError(
        "Missing required claim: sha",
        "MISSING_CLAIMS",
      );
    }

    if (!claims.sub) {
      throw new OIDCVerificationError(
        "Missing required claim: sub",
        "MISSING_CLAIMS",
      );
    }

    return claims;
  } catch (error) {
    if (error instanceof OIDCVerificationError) {
      throw error;
    }

    if (error instanceof jose.errors.JWTExpired) {
      throw new OIDCVerificationError("Token has expired", "EXPIRED");
    }

    if (error instanceof jose.errors.JWTClaimValidationFailed) {
      const message = error.message;
      if (message.includes("iss")) {
        throw new OIDCVerificationError(
          `Invalid issuer: expected ${GITHUB_OIDC_ISSUER}`,
          "INVALID_ISSUER",
        );
      }
      if (message.includes("aud")) {
        throw new OIDCVerificationError(
          `Invalid audience: expected ${expectedAudience}`,
          "INVALID_AUDIENCE",
        );
      }
    }

    throw new OIDCVerificationError(
      `Token verification failed: ${error instanceof Error ? error.message : "Unknown error"}`,
      "INVALID_TOKEN",
    );
  }
}

/**
 * Extract the Bearer token from an Authorization header.
 * @param authHeader The Authorization header value
 * @returns The token without the "Bearer " prefix, or null if invalid
 */
export function extractBearerToken(authHeader: string | null): string | null {
  if (!authHeader) {
    return null;
  }

  const parts = authHeader.split(" ");
  if (parts.length !== 2 || parts[0].toLowerCase() !== "bearer") {
    return null;
  }

  return parts[1];
}
