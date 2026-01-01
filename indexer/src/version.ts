/**
 * Numeric Dot versioning logic for Fontpub.
 */

export class InvalidVersionError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "InvalidVersionError";
  }
}

/**
 * Removes the leading 'v' or 'V' prefix if present.
 */
function normalizeVersion(v: string): string {
  if (v.length > 0 && (v[0] === "v" || v[0] === "V")) {
    return v.slice(1);
  }
  return v;
}

/**
 * Splits a version string by dots and converts each segment to an integer.
 */
function parseSegments(v: string): number[] {
  if (v === "") {
    throw new InvalidVersionError("version string cannot be empty");
  }

  const parts = v.split(".");
  const segments: number[] = [];

  for (const part of parts) {
    if (part === "") {
      throw new InvalidVersionError(
        "invalid version format: must contain only digits and dots"
      );
    }

    // Check for non-digit characters
    if (!/^\d+$/.test(part)) {
      throw new InvalidVersionError(
        "invalid version format: must contain only digits and dots"
      );
    }

    const num = parseInt(part, 10);
    if (num < 0) {
      throw new InvalidVersionError(
        "invalid version format: segments must be non-negative"
      );
    }

    segments.push(num);
  }

  return segments;
}

/**
 * Checks if a version string is valid according to Numeric Dot format.
 */
export function isValidVersion(v: string): boolean {
  try {
    const normalized = normalizeVersion(v);
    parseSegments(normalized);
    return true;
  } catch {
    return false;
  }
}

/**
 * Compares two version strings using Numeric Dot algorithm.
 * @returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
 * @throws InvalidVersionError if either version string is invalid
 */
export function compareVersions(v1: string, v2: string): -1 | 0 | 1 {
  // Normalize: remove leading 'v' or 'V'
  const normalized1 = normalizeVersion(v1);
  const normalized2 = normalizeVersion(v2);

  // Parse segments
  const seg1 = parseSegments(normalized1);
  const seg2 = parseSegments(normalized2);

  // Pad shorter array with zeros
  const maxLen = Math.max(seg1.length, seg2.length);

  while (seg1.length < maxLen) {
    seg1.push(0);
  }
  while (seg2.length < maxLen) {
    seg2.push(0);
  }

  // Compare segment by segment from left to right
  for (let i = 0; i < maxLen; i++) {
    if (seg1[i] < seg2[i]) {
      return -1;
    }
    if (seg1[i] > seg2[i]) {
      return 1;
    }
  }

  return 0;
}

/**
 * Returns true if v1 is newer (greater) than v2.
 */
export function isNewer(v1: string, v2: string): boolean {
  return compareVersions(v1, v2) > 0;
}

