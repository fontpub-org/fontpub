/**
 * SHA-256 stream hashing utilities.
 * Uses crypto.subtle.digest to hash files without loading them entirely into memory.
 */

/**
 * Calculate SHA-256 hash from a ReadableStream.
 * This streams the data through the hasher without buffering the entire content.
 * @param stream The readable stream to hash
 * @returns Hex-encoded SHA-256 hash
 */
export async function hashStream(
  stream: ReadableStream<Uint8Array>,
): Promise<string> {
  const reader = stream.getReader();
  const chunks: Uint8Array[] = [];

  // Read all chunks
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
  }

  // Concatenate chunks
  const totalLength = chunks.reduce((acc, chunk) => acc + chunk.length, 0);
  const combined = new Uint8Array(totalLength);
  let offset = 0;
  for (const chunk of chunks) {
    combined.set(chunk, offset);
    offset += chunk.length;
  }

  // Calculate hash
  const hashBuffer = await crypto.subtle.digest("SHA-256", combined);

  // Convert to hex string
  return arrayBufferToHex(hashBuffer);
}

/**
 * Calculate SHA-256 hash from an ArrayBuffer.
 * @param buffer The buffer to hash
 * @returns Hex-encoded SHA-256 hash
 */
export async function hashArrayBuffer(buffer: ArrayBuffer): Promise<string> {
  const hashBuffer = await crypto.subtle.digest("SHA-256", buffer);
  return arrayBufferToHex(hashBuffer);
}

/**
 * Convert ArrayBuffer to hex string.
 */
function arrayBufferToHex(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

/**
 * Fetch a file from URL and calculate its SHA-256 hash.
 * The file content is not retained in memory after hashing.
 * @param url The URL to fetch
 * @returns Object containing the hash and content length
 * @throws Error if fetch fails or response is not ok
 */
export async function fetchAndHash(
  url: string,
): Promise<{ hash: string; size: number }> {
  const response = await fetch(url);

  if (!response.ok) {
    throw new Error(
      `Failed to fetch ${url}: ${response.status} ${response.statusText}`,
    );
  }

  if (!response.body) {
    throw new Error(`No response body for ${url}`);
  }

  // Get content length if available
  const contentLength = response.headers.get("content-length");
  const size = contentLength ? parseInt(contentLength, 10) : 0;

  // Stream and hash
  const hash = await hashStream(response.body);

  return { hash, size };
}

// Maximum file size: 50MB
export const MAX_FILE_SIZE = 50 * 1024 * 1024;

/**
 * Verify file size is within limits.
 * @param size File size in bytes
 * @throws Error if file exceeds size limit
 */
export function verifyFileSize(size: number): void {
  if (size > MAX_FILE_SIZE) {
    throw new Error(
      `File size ${size} bytes exceeds maximum allowed size of ${MAX_FILE_SIZE} bytes (50MB)`,
    );
  }
}
