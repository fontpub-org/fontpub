import { describe, expect, it } from "vitest";
import { AssetError, buildPackageDetail } from "../src/assets";
import type { Asset, FontpubManifest } from "../src/types";

describe("assets", () => {
  describe("buildPackageDetail", () => {
    it("should build complete package detail", () => {
      const manifest: FontpubManifest = {
        name: "MyFont",
        author: "Test Author",
        version: "1.0.0",
        license: "MIT",
        files: [
          { path: "fonts/MyFont-Regular.otf", style: "regular", weight: 400 },
        ],
      };

      const assets: Asset[] = [
        {
          path: "fonts/MyFont-Regular.otf",
          url: "https://raw.githubusercontent.com/owner/repo/sha/fonts/MyFont-Regular.otf",
          sha256: "abc123",
          style: "regular",
          weight: 400,
          format: "otf",
        },
      ];

      const detail = buildPackageDetail(manifest, "sha123", assets);

      expect(detail.name).toBe("MyFont");
      expect(detail.version).toBe("1.0.0");
      expect(detail.github_sha).toBe("sha123");
      expect(detail.assets).toHaveLength(1);
      expect(detail.assets[0].path).toBe("fonts/MyFont-Regular.otf");
    });

    it("should handle multiple assets", () => {
      const manifest: FontpubManifest = {
        name: "FontFamily",
        author: "Author",
        version: "2.0",
        license: "OFL",
        files: [
          { path: "Regular.ttf" },
          { path: "Bold.ttf" },
          { path: "Italic.ttf" },
        ],
      };

      const assets: Asset[] = [
        { path: "Regular.ttf", url: "url1", sha256: "hash1", format: "ttf" },
        { path: "Bold.ttf", url: "url2", sha256: "hash2", format: "ttf" },
        { path: "Italic.ttf", url: "url3", sha256: "hash3", format: "ttf" },
      ];

      const detail = buildPackageDetail(manifest, "commit-sha", assets);

      expect(detail.assets).toHaveLength(3);
      expect(detail.version).toBe("2.0");
    });
  });

  describe("AssetError", () => {
    it("should have correct name and properties", () => {
      const error = new AssetError("test", "FETCH_FAILED");
      expect(error.name).toBe("AssetError");
      expect(error.code).toBe("FETCH_FAILED");
      expect(error.message).toBe("test");
    });

    it("should support all error codes", () => {
      const codes = ["FETCH_FAILED", "SIZE_EXCEEDED", "HASH_FAILED"] as const;

      for (const code of codes) {
        const error = new AssetError(`Error: ${code}`, code);
        expect(error.code).toBe(code);
      }
    });
  });
});
