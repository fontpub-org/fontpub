import { describe, expect, it } from "vitest";
import {
  hashArrayBuffer,
  hashStream,
  MAX_FILE_SIZE,
  verifyFileSize,
} from "../src/hash";

describe("hash", () => {
  describe("hashArrayBuffer", () => {
    it("should hash an empty buffer", async () => {
      const buffer = new ArrayBuffer(0);
      const hash = await hashArrayBuffer(buffer);
      // SHA-256 of empty string
      expect(hash).toBe(
        "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      );
    });

    it("should hash a simple string", async () => {
      const encoder = new TextEncoder();
      const buffer = encoder.encode("hello").buffer as ArrayBuffer;
      const hash = await hashArrayBuffer(buffer);
      // SHA-256 of "hello"
      expect(hash).toBe(
        "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
      );
    });

    it("should produce lowercase hex output", async () => {
      const buffer = new ArrayBuffer(1);
      const hash = await hashArrayBuffer(buffer);
      expect(hash).toMatch(/^[0-9a-f]{64}$/);
    });
  });

  describe("hashStream", () => {
    it("should hash a stream correctly", async () => {
      const encoder = new TextEncoder();
      const data = encoder.encode("hello world");

      const stream = new ReadableStream({
        start(controller) {
          controller.enqueue(data);
          controller.close();
        },
      });

      const hash = await hashStream(stream);
      // SHA-256 of "hello world"
      expect(hash).toBe(
        "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
      );
    });

    it("should handle multiple chunks", async () => {
      const encoder = new TextEncoder();
      const chunk1 = encoder.encode("hello ");
      const chunk2 = encoder.encode("world");

      const stream = new ReadableStream({
        start(controller) {
          controller.enqueue(chunk1);
          controller.enqueue(chunk2);
          controller.close();
        },
      });

      const hash = await hashStream(stream);
      // Should produce same hash as single "hello world" chunk
      expect(hash).toBe(
        "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
      );
    });

    it("should handle empty stream", async () => {
      const stream = new ReadableStream({
        start(controller) {
          controller.close();
        },
      });

      const hash = await hashStream(stream);
      // SHA-256 of empty
      expect(hash).toBe(
        "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      );
    });
  });

  describe("verifyFileSize", () => {
    it("should not throw for small files", () => {
      expect(() => verifyFileSize(1024)).not.toThrow();
      expect(() => verifyFileSize(1024 * 1024)).not.toThrow();
      expect(() => verifyFileSize(MAX_FILE_SIZE - 1)).not.toThrow();
      expect(() => verifyFileSize(MAX_FILE_SIZE)).not.toThrow();
    });

    it("should throw for files exceeding limit", () => {
      expect(() => verifyFileSize(MAX_FILE_SIZE + 1)).toThrow(
        /exceeds maximum allowed size/,
      );
      expect(() => verifyFileSize(100 * 1024 * 1024)).toThrow();
    });
  });

  describe("MAX_FILE_SIZE", () => {
    it("should be 50MB", () => {
      expect(MAX_FILE_SIZE).toBe(50 * 1024 * 1024);
    });
  });
});
