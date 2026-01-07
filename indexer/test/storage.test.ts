import { beforeEach, describe, expect, it } from "vitest";
import {
  createEmptyIndex,
  getPackageDetail,
  getPackagePath,
  getRootIndex,
  INDEX_PATH,
  putPackageDetail,
  putRootIndex,
} from "../src/storage";
import type { PackageDetail, RootIndex } from "../src/types";

// Mock R2 bucket for testing
class MockR2Bucket {
  private storage = new Map<string, { body: string; etag: string }>();
  private etagCounter = 0;

  async get(key: string): Promise<R2ObjectBody | null> {
    const item = this.storage.get(key);
    if (!item) return null;

    return {
      body: new ReadableStream({
        start(controller) {
          controller.enqueue(new TextEncoder().encode(item.body));
          controller.close();
        },
      }),
      bodyUsed: false,
      arrayBuffer: async () => new TextEncoder().encode(item.body).buffer,
      text: async () => item.body,
      json: async () => JSON.parse(item.body),
      blob: async () => new Blob([item.body]),
      etag: item.etag,
      httpEtag: `"${item.etag}"`,
      key,
      version: "1",
      size: item.body.length,
      uploaded: new Date(),
      checksums: { toJSON: () => ({}) },
      writeHttpMetadata: () => {},
    } as unknown as R2ObjectBody;
  }

  async put(
    key: string,
    value: string | ReadableStream | ArrayBuffer | Blob | null,
    options?: R2PutOptions,
  ): Promise<R2Object | null> {
    // Check conditional write
    const onlyIf = options?.onlyIf as R2Conditional | undefined;
    if (onlyIf && "etagMatches" in onlyIf && onlyIf.etagMatches) {
      const existing = this.storage.get(key);
      if (!existing || existing.etag !== onlyIf.etagMatches) {
        return null; // ETag mismatch
      }
    }

    const body = typeof value === "string" ? value : "";
    const etag = `etag-${++this.etagCounter}`;
    this.storage.set(key, { body, etag });

    return {
      key,
      version: "1",
      size: body.length,
      etag,
      httpEtag: `"${etag}"`,
      uploaded: new Date(),
      checksums: { toJSON: () => ({}) },
      writeHttpMetadata: () => {},
    } as unknown as R2Object;
  }

  async delete(key: string): Promise<void> {
    this.storage.delete(key);
  }

  clear(): void {
    this.storage.clear();
    this.etagCounter = 0;
  }
}

describe("storage", () => {
  let bucket: MockR2Bucket;

  beforeEach(() => {
    bucket = new MockR2Bucket();
  });

  describe("getRootIndex", () => {
    it("returns null for empty bucket", async () => {
      const result = await getRootIndex(bucket as unknown as R2Bucket);
      expect(result).toBeNull();
    });

    it("returns index with etag when exists", async () => {
      const index: RootIndex = {
        packages: {
          "test/font": {
            latest_version: "1.0.0",
            last_updated: "2026-01-01T00:00:00Z",
          },
        },
      };
      await bucket.put(INDEX_PATH, JSON.stringify(index));

      const result = await getRootIndex(bucket as unknown as R2Bucket);

      expect(result).not.toBeNull();
      expect(result!.data.packages["test/font"].latest_version).toBe("1.0.0");
      expect(result!.etag).toBeTruthy();
    });
  });

  describe("putRootIndex", () => {
    it("creates new index", async () => {
      const index: RootIndex = {
        packages: {
          "new/font": {
            latest_version: "2.0.0",
            last_updated: "2026-01-02T00:00:00Z",
          },
        },
      };

      const success = await putRootIndex(bucket as unknown as R2Bucket, index);

      expect(success).toBe(true);

      const result = await getRootIndex(bucket as unknown as R2Bucket);
      expect(result!.data.packages["new/font"].latest_version).toBe("2.0.0");
    });

    it("succeeds with matching etag", async () => {
      // Create initial index
      const initial: RootIndex = { packages: {} };
      await putRootIndex(bucket as unknown as R2Bucket, initial);

      // Get etag
      const result = await getRootIndex(bucket as unknown as R2Bucket);
      const etag = result!.etag!;

      // Update with matching etag
      const updated: RootIndex = {
        packages: {
          "test/pkg": {
            latest_version: "1.0.0",
            last_updated: "2026-01-01T00:00:00Z",
          },
        },
      };

      const success = await putRootIndex(
        bucket as unknown as R2Bucket,
        updated,
        etag,
      );

      expect(success).toBe(true);
    });

    it("fails with mismatched etag", async () => {
      // Create initial index
      const initial: RootIndex = { packages: {} };
      await putRootIndex(bucket as unknown as R2Bucket, initial);

      // Try to update with wrong etag
      const updated: RootIndex = {
        packages: {
          "test/pkg": {
            latest_version: "1.0.0",
            last_updated: "2026-01-01T00:00:00Z",
          },
        },
      };

      const success = await putRootIndex(
        bucket as unknown as R2Bucket,
        updated,
        "wrong-etag",
      );

      expect(success).toBe(false);
    });
  });

  describe("getPackageDetail", () => {
    it("returns null for non-existent package", async () => {
      const result = await getPackageDetail(
        bucket as unknown as R2Bucket,
        "unknown",
        "package",
      );
      expect(result).toBeNull();
    });

    it("returns package detail when exists", async () => {
      const detail: PackageDetail = {
        name: "myfont",
        version: "1.0.0",
        github_sha: "abc123",
        assets: [
          {
            path: "fonts/Font.otf",
            url: "https://example.com/Font.otf",
            sha256: "hash123",
          },
        ],
      };
      const path = getPackagePath("owner", "myfont");
      await bucket.put(path, JSON.stringify(detail));

      const result = await getPackageDetail(
        bucket as unknown as R2Bucket,
        "owner",
        "myfont",
      );

      expect(result).not.toBeNull();
      expect(result!.data.name).toBe("myfont");
      expect(result!.data.assets).toHaveLength(1);
    });
  });

  describe("putPackageDetail", () => {
    it("stores package detail", async () => {
      const detail: PackageDetail = {
        name: "newfont",
        version: "2.0.0",
        github_sha: "def456",
        assets: [],
      };

      await putPackageDetail(
        bucket as unknown as R2Bucket,
        "owner",
        "newfont",
        detail,
      );

      const result = await getPackageDetail(
        bucket as unknown as R2Bucket,
        "owner",
        "newfont",
      );

      expect(result).not.toBeNull();
      expect(result!.data.version).toBe("2.0.0");
    });
  });

  describe("createEmptyIndex", () => {
    it("creates empty packages object", () => {
      const index = createEmptyIndex();
      expect(index.packages).toEqual({});
    });
  });

  describe("getPackagePath", () => {
    it("generates correct path", () => {
      expect(getPackagePath("alice", "myfont")).toBe(
        "v1/packages/alice/myfont.json",
      );
    });
  });
});
