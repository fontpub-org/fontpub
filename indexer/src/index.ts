/**
 * Fontpub Indexer - Cloudflare Workers Application
 *
 * A "notary" service that records font package metadata without mirroring binaries.
 * Verifies GitHub OIDC tokens and maintains an index of font packages.
 */

import { Hono } from "hono";
import { cors } from "hono/cors";
import { handleGetIndex, handleGetPackageDetail } from "./handlers/static";
import { handlePostUpdate } from "./handlers/update";
import type { Env } from "./types";

const app = new Hono<{ Bindings: Env }>();

// Enable CORS for CLI and web access
app.use(
  "*",
  cors({
    origin: "*",
    allowMethods: ["GET", "POST", "OPTIONS"],
    allowHeaders: ["Content-Type", "Authorization"],
  }),
);

// Health check
app.get("/", (c) => {
  return c.json({
    name: "fontpub-indexer",
    version: "0.0.1",
    status: "ok",
  });
});

// ============================================================
// Read Endpoints (GET)
// ============================================================

// Root index - list all packages with latest versions
app.get("/v1/index.json", handleGetIndex);

// Package detail - full metadata for a specific package
app.get("/v1/packages/:owner/:repo", handleGetPackageDetail);

// ============================================================
// Write Endpoints (POST)
// ============================================================

// Update endpoint - receives notifications from GitHub Actions
app.post("/v1/update", handlePostUpdate);

// ============================================================
// 404 Handler
// ============================================================

app.notFound((c) => {
  return c.json(
    {
      error: "Not found",
      details: `Path ${c.req.path} does not exist`,
    },
    404,
  );
});

export default app;
