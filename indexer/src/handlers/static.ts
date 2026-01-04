/**
 * Static file handlers for serving index data from R2.
 */

import { Context } from "hono";
import type { Env, ErrorResponse } from "../types";
import {
  getRootIndex,
  getPackageDetail,
  createEmptyIndex,
} from "../storage";

/**
 * GET /v1/index.json
 * Returns the root index with all package summaries.
 */
export async function handleGetIndex(
  c: Context<{ Bindings: Env }>
): Promise<Response> {
  const result = await getRootIndex(c.env.INDEX_BUCKET);

  if (!result) {
    // Return empty index if none exists yet
    return c.json(createEmptyIndex());
  }

  // Set ETag header for caching
  if (result.etag) {
    c.header("ETag", result.etag);
  }

  return c.json(result.data);
}

/**
 * GET /v1/packages/:owner/:repo.json
 * Returns detailed information for a specific package.
 */
export async function handleGetPackageDetail(
  c: Context<{ Bindings: Env }>
): Promise<Response> {
  const owner = c.req.param("owner");
  let repo = c.req.param("repo");

  if (!owner || !repo) {
    const error: ErrorResponse = {
      error: "Invalid package path",
      details: "Both owner and repo are required",
    };
    return c.json(error, 400);
  }

  // Remove .json extension if present
  if (repo.endsWith(".json")) {
    repo = repo.slice(0, -5);
  }

  const result = await getPackageDetail(c.env.INDEX_BUCKET, owner, repo);

  if (!result) {
    const error: ErrorResponse = {
      error: "Package not found",
      details: `Package ${owner}/${repo} does not exist`,
    };
    return c.json(error, 404);
  }

  // Set ETag header for caching
  if (result.etag) {
    c.header("ETag", result.etag);
  }

  return c.json(result.data);
}
