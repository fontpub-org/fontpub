import { beforeEach, describe, expect, it } from "vitest";
import {
  deleteOwnership,
  getOwnership,
  OwnershipError,
  registerOwnership,
  verifyOrRegisterOwnership,
} from "../src/ownership";
import type { OwnershipRecord } from "../src/types";

// Mock KV Namespace
class MockKVNamespace {
  private storage = new Map<string, string>();

  async get(
    key: string,
    format?: "text" | "json",
  ): Promise<string | null | unknown> {
    const value = this.storage.get(key);
    if (value === undefined) return null;
    if (format === "json") {
      return JSON.parse(value);
    }
    return value;
  }

  async put(key: string, value: string): Promise<void> {
    this.storage.set(key, value);
  }

  async delete(key: string): Promise<void> {
    this.storage.delete(key);
  }

  clear(): void {
    this.storage.clear();
  }
}

describe("ownership", () => {
  let mockKV: MockKVNamespace;

  beforeEach(() => {
    mockKV = new MockKVNamespace();
  });

  describe("getOwnership", () => {
    it("should return null for non-existent repository", async () => {
      const result = await getOwnership(
        mockKV as unknown as KVNamespace,
        "owner/repo",
      );
      expect(result).toBeNull();
    });

    it("should return ownership record for existing repository", async () => {
      const record: OwnershipRecord = {
        owner_sub: "repo:owner/repo:ref:refs/heads/main",
        registered_at: "2024-01-01T00:00:00.000Z",
      };
      await mockKV.put("owner:owner/repo", JSON.stringify(record));

      const result = await getOwnership(
        mockKV as unknown as KVNamespace,
        "owner/repo",
      );
      expect(result).toEqual(record);
    });
  });

  describe("registerOwnership", () => {
    it("should register new ownership", async () => {
      await registerOwnership(
        mockKV as unknown as KVNamespace,
        "owner/repo",
        "repo:owner/repo:ref:refs/heads/main",
      );

      const stored = (await mockKV.get(
        "owner:owner/repo",
        "json",
      )) as OwnershipRecord;
      expect(stored.owner_sub).toBe("repo:owner/repo:ref:refs/heads/main");
      expect(stored.registered_at).toBeDefined();
    });
  });

  describe("verifyOrRegisterOwnership", () => {
    it("should register new repository and return isNewRegistration=true", async () => {
      const result = await verifyOrRegisterOwnership(
        mockKV as unknown as KVNamespace,
        "owner/repo",
        "repo:owner/repo:ref:refs/heads/main",
      );

      expect(result.isNewRegistration).toBe(true);

      const stored = await getOwnership(
        mockKV as unknown as KVNamespace,
        "owner/repo",
      );
      expect(stored).not.toBeNull();
      expect(stored?.owner_sub).toBe("repo:owner/repo:ref:refs/heads/main");
    });

    it("should verify existing owner and return isNewRegistration=false", async () => {
      // First registration
      await registerOwnership(
        mockKV as unknown as KVNamespace,
        "owner/repo",
        "repo:owner/repo:ref:refs/heads/main",
      );

      // Second verification with same sub
      const result = await verifyOrRegisterOwnership(
        mockKV as unknown as KVNamespace,
        "owner/repo",
        "repo:owner/repo:ref:refs/heads/main",
      );

      expect(result.isNewRegistration).toBe(false);
    });

    it("should throw OwnershipError for different owner", async () => {
      // First registration
      await registerOwnership(
        mockKV as unknown as KVNamespace,
        "owner/repo",
        "repo:owner/repo:ref:refs/heads/main",
      );

      // Attempt by different sub
      await expect(
        verifyOrRegisterOwnership(
          mockKV as unknown as KVNamespace,
          "owner/repo",
          "repo:attacker/repo:ref:refs/heads/main",
        ),
      ).rejects.toThrow(OwnershipError);
    });

    it("should throw OwnershipError with correct code", async () => {
      await registerOwnership(
        mockKV as unknown as KVNamespace,
        "owner/repo",
        "repo:owner/repo:ref:refs/heads/main",
      );

      try {
        await verifyOrRegisterOwnership(
          mockKV as unknown as KVNamespace,
          "owner/repo",
          "repo:attacker/repo:ref:refs/heads/main",
        );
        expect.fail("Should have thrown");
      } catch (error) {
        expect(error).toBeInstanceOf(OwnershipError);
        expect((error as OwnershipError).code).toBe("NOT_OWNER");
      }
    });
  });

  describe("deleteOwnership", () => {
    it("should delete ownership record", async () => {
      await registerOwnership(
        mockKV as unknown as KVNamespace,
        "owner/repo",
        "repo:owner/repo:ref:refs/heads/main",
      );

      await deleteOwnership(mockKV as unknown as KVNamespace, "owner/repo");

      const result = await getOwnership(
        mockKV as unknown as KVNamespace,
        "owner/repo",
      );
      expect(result).toBeNull();
    });

    it("should not throw when deleting non-existent record", async () => {
      await expect(
        deleteOwnership(mockKV as unknown as KVNamespace, "nonexistent/repo"),
      ).resolves.not.toThrow();
    });
  });
});

describe("OwnershipError", () => {
  it("should have correct name and properties", () => {
    const error = new OwnershipError("test message", "NOT_OWNER");
    expect(error.name).toBe("OwnershipError");
    expect(error.code).toBe("NOT_OWNER");
    expect(error.message).toBe("test message");
  });
});
