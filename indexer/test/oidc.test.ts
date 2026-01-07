import { describe, expect, it } from "vitest";
import { extractBearerToken, OIDCVerificationError } from "../src/oidc";

describe("extractBearerToken", () => {
  it("should extract token from valid Bearer header", () => {
    const token = extractBearerToken("Bearer abc123xyz");
    expect(token).toBe("abc123xyz");
  });

  it("should handle lowercase 'bearer'", () => {
    const token = extractBearerToken("bearer abc123xyz");
    expect(token).toBe("abc123xyz");
  });

  it("should return null for null header", () => {
    const token = extractBearerToken(null);
    expect(token).toBeNull();
  });

  it("should return null for empty string", () => {
    const token = extractBearerToken("");
    expect(token).toBeNull();
  });

  it("should return null for missing Bearer prefix", () => {
    const token = extractBearerToken("abc123xyz");
    expect(token).toBeNull();
  });

  it("should return null for wrong auth type", () => {
    const token = extractBearerToken("Basic abc123xyz");
    expect(token).toBeNull();
  });

  it("should return null for malformed header with extra parts", () => {
    const token = extractBearerToken("Bearer abc 123");
    expect(token).toBeNull();
  });
});

describe("OIDCVerificationError", () => {
  it("should have correct name and code", () => {
    const error = new OIDCVerificationError("test message", "INVALID_TOKEN");
    expect(error.name).toBe("OIDCVerificationError");
    expect(error.code).toBe("INVALID_TOKEN");
    expect(error.message).toBe("test message");
  });

  it("should work with different error codes", () => {
    const codes = [
      "INVALID_TOKEN",
      "INVALID_ISSUER",
      "INVALID_AUDIENCE",
      "EXPIRED",
      "MISSING_CLAIMS",
    ] as const;

    for (const code of codes) {
      const error = new OIDCVerificationError(`Error: ${code}`, code);
      expect(error.code).toBe(code);
    }
  });
});

// Note: Testing verifyGitHubOIDCToken requires real GitHub OIDC tokens
// which can only be obtained from GitHub Actions environment.
// Integration tests should be done in a GitHub Actions workflow.
