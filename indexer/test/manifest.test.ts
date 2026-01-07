import { describe, expect, it } from "vitest";
import {
  buildGitHubRawUrl,
  getFileExtension,
  isValidFontExtension,
  ManifestError,
} from "../src/manifest";

describe("manifest", () => {
  describe("buildGitHubRawUrl", () => {
    it("should build correct URL", () => {
      const url = buildGitHubRawUrl("owner/repo", "abc123", "fonts/MyFont.otf");
      expect(url).toBe(
        "https://raw.githubusercontent.com/owner/repo/abc123/fonts/MyFont.otf",
      );
    });

    it("should handle paths without leading slash", () => {
      const url = buildGitHubRawUrl("owner/repo", "sha", "file.txt");
      expect(url).toBe(
        "https://raw.githubusercontent.com/owner/repo/sha/file.txt",
      );
    });

    it("should handle nested paths", () => {
      const url = buildGitHubRawUrl(
        "user/font-repo",
        "v1.0.0",
        "dist/fonts/sub/Font-Bold.woff2",
      );
      expect(url).toBe(
        "https://raw.githubusercontent.com/user/font-repo/v1.0.0/dist/fonts/sub/Font-Bold.woff2",
      );
    });
  });

  describe("getFileExtension", () => {
    it("should extract extension correctly", () => {
      expect(getFileExtension("font.otf")).toBe("otf");
      expect(getFileExtension("path/to/font.ttf")).toBe("ttf");
      expect(getFileExtension("Font-Bold.woff2")).toBe("woff2");
    });

    it("should return lowercase extension", () => {
      expect(getFileExtension("Font.OTF")).toBe("otf");
      expect(getFileExtension("Font.TTF")).toBe("ttf");
    });

    it("should handle files without extension", () => {
      expect(getFileExtension("filename")).toBe("");
    });

    it("should handle multiple dots", () => {
      expect(getFileExtension("font.min.otf")).toBe("otf");
      expect(getFileExtension("a.b.c.ttf")).toBe("ttf");
    });
  });

  describe("isValidFontExtension", () => {
    it("should accept valid font extensions", () => {
      expect(isValidFontExtension("otf")).toBe(true);
      expect(isValidFontExtension("ttf")).toBe(true);
      expect(isValidFontExtension("woff")).toBe(true);
      expect(isValidFontExtension("woff2")).toBe(true);
    });

    it("should be case-insensitive", () => {
      expect(isValidFontExtension("OTF")).toBe(true);
      expect(isValidFontExtension("TTF")).toBe(true);
      expect(isValidFontExtension("WOFF2")).toBe(true);
    });

    it("should reject invalid extensions", () => {
      expect(isValidFontExtension("txt")).toBe(false);
      expect(isValidFontExtension("zip")).toBe(false);
      expect(isValidFontExtension("exe")).toBe(false);
      expect(isValidFontExtension("")).toBe(false);
    });
  });

  describe("ManifestError", () => {
    it("should have correct name and properties", () => {
      const error = new ManifestError("test", "FETCH_FAILED");
      expect(error.name).toBe("ManifestError");
      expect(error.code).toBe("FETCH_FAILED");
      expect(error.message).toBe("test");
    });

    it("should support all error codes", () => {
      const codes = [
        "FETCH_FAILED",
        "INVALID_JSON",
        "MISSING_FIELD",
        "INVALID_FILE",
      ] as const;

      for (const code of codes) {
        const error = new ManifestError(`Error: ${code}`, code);
        expect(error.code).toBe(code);
      }
    });
  });
});
