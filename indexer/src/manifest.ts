/**
 * Manifest (fontpub.json) fetching and parsing.
 */

import type { FontpubManifest, ManifestFile } from "./types";

// Valid font file extensions
const VALID_FONT_EXTENSIONS = new Set(["otf", "ttf", "woff", "woff2"]);

/**
 * Error thrown when manifest processing fails.
 */
export class ManifestError extends Error {
  constructor(
    message: string,
    public readonly code:
      | "FETCH_FAILED"
      | "INVALID_JSON"
      | "MISSING_FIELD"
      | "INVALID_FILE",
  ) {
    super(message);
    this.name = "ManifestError";
  }
}

/**
 * Build the GitHub raw URL for a file at a specific commit.
 * @param repository Owner/repo format
 * @param sha Commit SHA
 * @param path File path within the repository
 */
export function buildGitHubRawUrl(
  repository: string,
  sha: string,
  path: string,
): string {
  return `https://raw.githubusercontent.com/${repository}/${sha}/${path}`;
}

/**
 * Fetch the fontpub.json manifest from a GitHub repository.
 * @param repository Owner/repo format
 * @param sha Commit SHA
 * @returns Parsed manifest
 * @throws ManifestError if fetch or parsing fails
 */
export async function fetchManifest(
  repository: string,
  sha: string,
): Promise<FontpubManifest> {
  const url = buildGitHubRawUrl(repository, sha, "fontpub.json");

  let response: Response;
  try {
    response = await fetch(url);
  } catch (error) {
    throw new ManifestError(
      `Failed to fetch manifest: ${error instanceof Error ? error.message : "Unknown error"}`,
      "FETCH_FAILED",
    );
  }

  if (!response.ok) {
    throw new ManifestError(
      `Failed to fetch manifest: ${response.status} ${response.statusText}`,
      "FETCH_FAILED",
    );
  }

  let manifest: unknown;
  try {
    manifest = await response.json();
  } catch {
    throw new ManifestError("Invalid JSON in fontpub.json", "INVALID_JSON");
  }

  return validateManifest(manifest);
}

/**
 * Validate and type-check a manifest object.
 */
function validateManifest(data: unknown): FontpubManifest {
  if (!data || typeof data !== "object") {
    throw new ManifestError("Manifest must be an object", "INVALID_JSON");
  }

  const obj = data as Record<string, unknown>;

  // Required fields
  if (typeof obj.name !== "string" || !obj.name) {
    throw new ManifestError("Missing or invalid 'name' field", "MISSING_FIELD");
  }

  if (typeof obj.author !== "string" || !obj.author) {
    throw new ManifestError(
      "Missing or invalid 'author' field",
      "MISSING_FIELD",
    );
  }

  if (typeof obj.version !== "string" || !obj.version) {
    throw new ManifestError(
      "Missing or invalid 'version' field",
      "MISSING_FIELD",
    );
  }

  if (typeof obj.license !== "string" || !obj.license) {
    throw new ManifestError(
      "Missing or invalid 'license' field",
      "MISSING_FIELD",
    );
  }

  if (!Array.isArray(obj.files) || obj.files.length === 0) {
    throw new ManifestError("Missing or empty 'files' array", "MISSING_FIELD");
  }

  // Validate each file entry
  const files: ManifestFile[] = [];
  for (const file of obj.files) {
    files.push(validateManifestFile(file));
  }

  return {
    name: obj.name,
    author: obj.author,
    version: obj.version,
    license: obj.license,
    files,
  };
}

/**
 * Validate a single file entry in the manifest.
 */
function validateManifestFile(data: unknown): ManifestFile {
  if (!data || typeof data !== "object") {
    throw new ManifestError("File entry must be an object", "INVALID_FILE");
  }

  const obj = data as Record<string, unknown>;

  if (typeof obj.path !== "string" || !obj.path) {
    throw new ManifestError("File entry missing 'path' field", "INVALID_FILE");
  }

  // Validate file extension
  const extension = getFileExtension(obj.path);
  if (!isValidFontExtension(extension)) {
    throw new ManifestError(
      `Invalid font file extension: ${extension}. Allowed: ${Array.from(VALID_FONT_EXTENSIONS).join(", ")}`,
      "INVALID_FILE",
    );
  }

  const file: ManifestFile = {
    path: obj.path,
  };

  // Optional fields
  if (typeof obj.style === "string") {
    file.style = obj.style;
  }

  if (typeof obj.weight === "number") {
    file.weight = obj.weight;
  }

  return file;
}

/**
 * Get the file extension from a path.
 */
export function getFileExtension(path: string): string {
  const parts = path.split(".");
  return parts.length > 1 ? parts[parts.length - 1].toLowerCase() : "";
}

/**
 * Check if a file extension is a valid font format.
 */
export function isValidFontExtension(extension: string): boolean {
  return VALID_FONT_EXTENSIONS.has(extension.toLowerCase());
}
