import { describe, expect, it } from "vitest";
import {
  compareVersions,
  InvalidVersionError,
  isNewer,
  isValidVersion,
} from "../src/version";

describe("isValidVersion", () => {
  // Valid versions
  it.each([
    ["1.0", true],
    ["1.0.0", true],
    ["1.500", true],
    ["v1.0", true],
    ["V1.0", true],
    ["0.0.1", true],
    ["10.20.30", true],
    ["1", true],
    ["1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0", true], // Long but valid
  ])("isValidVersion(%s) should return %s", (version, expected) => {
    expect(isValidVersion(version)).toBe(expected);
  });

  // Invalid versions
  it.each([
    ["", false],
    ["v", false],
    ["1.0.0-alpha", false],
    ["1.0.0+build", false],
    ["1.0.0a", false],
    ["a.b.c", false],
    ["1..0", false],
    [".1.0", false],
    ["1.0.", false],
    ["-1.0", false],
  ])("isValidVersion(%s) should return %s", (version, expected) => {
    expect(isValidVersion(version)).toBe(expected);
  });
});

describe("compareVersions", () => {
  // Spec examples
  it("1.500 > 1.5", () => {
    expect(compareVersions("1.500", "1.5")).toBe(1);
  });

  it("2.0.1 > 2.0", () => {
    expect(compareVersions("2.0.1", "2.0")).toBe(1);
  });

  it("v1.2 == 1.2", () => {
    expect(compareVersions("v1.2", "1.2")).toBe(0);
  });

  // Equal versions
  it.each([
    ["1.0.0", "1.0.0", 0],
    ["1.0", "1.0.0", 0],
    ["1", "1.0.0", 0],
    ["V1.0", "v1.0", 0],
  ])("compareVersions(%s, %s) should return %d", (v1, v2, expected) => {
    expect(compareVersions(v1, v2)).toBe(expected);
  });

  // Less than
  it.each([
    ["1.0", "1.1", -1],
    ["1.0", "2.0", -1],
    ["1.9", "1.10", -1],
    ["1.0.0", "1.0.1", -1],
  ])("compareVersions(%s, %s) should return %d", (v1, v2, expected) => {
    expect(compareVersions(v1, v2)).toBe(expected);
  });

  // Greater than
  it.each([
    ["2.0", "1.0", 1],
    ["1.1", "1.0", 1],
    ["1.10", "1.9", 1],
    ["1.0.1", "1.0.0", 1],
  ])("compareVersions(%s, %s) should return %d", (v1, v2, expected) => {
    expect(compareVersions(v1, v2)).toBe(expected);
  });

  // Edge cases with zeros
  it.each([
    ["0.0.0", "0.0.0", 0],
    ["0.0.1", "0.0.0", 1],
    ["0.1.0", "0.0.1", 1],
  ])("compareVersions(%s, %s) should return %d", (v1, v2, expected) => {
    expect(compareVersions(v1, v2)).toBe(expected);
  });

  // Different segment counts
  it.each([
    ["1.2.3.4", "1.2.3", 1],
    ["1.2.3", "1.2.3.4", -1],
    ["1.2.3.0", "1.2.3", 0],
  ])("compareVersions(%s, %s) should return %d", (v1, v2, expected) => {
    expect(compareVersions(v1, v2)).toBe(expected);
  });

  // Invalid versions should throw
  it("should throw for invalid version with alpha suffix", () => {
    expect(() => compareVersions("1.0.0-alpha", "1.0.0")).toThrow(
      InvalidVersionError,
    );
  });

  it("should throw for invalid version string", () => {
    expect(() => compareVersions("1.0.0", "invalid")).toThrow(
      InvalidVersionError,
    );
  });

  it("should throw for empty version string (v1)", () => {
    expect(() => compareVersions("", "1.0.0")).toThrow(InvalidVersionError);
  });

  it("should throw for empty version string (v2)", () => {
    expect(() => compareVersions("1.0.0", "")).toThrow(InvalidVersionError);
  });
});

describe("isNewer", () => {
  it.each([
    ["2.0", "1.0", true],
    ["1.0", "2.0", false],
    ["1.0", "1.0", false],
    ["1.500", "1.5", true],
  ])("isNewer(%s, %s) should return %s", (v1, v2, expected) => {
    expect(isNewer(v1, v2)).toBe(expected);
  });

  it("should throw for invalid version", () => {
    expect(() => isNewer("invalid", "1.0")).toThrow(InvalidVersionError);
  });
});
