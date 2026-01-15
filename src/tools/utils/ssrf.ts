/**
 * SSRF Protection Utilities
 * Prevents Server-Side Request Forgery by blocking internal/private addresses
 */

// Private IP ranges (RFC 1918 + loopback + link-local + cloud metadata)
const PRIVATE_IP_PATTERNS = [
  /^127\./,                            // Loopback (127.0.0.0/8)
  /^10\./,                             // Class A private (10.0.0.0/8)
  /^172\.(1[6-9]|2[0-9]|3[01])\./,    // Class B private (172.16.0.0/12)
  /^192\.168\./,                       // Class C private (192.168.0.0/16)
  /^169\.254\./,                       // Link-local (169.254.0.0/16)
  /^0\./,                              // "This" network
  /^::1$/,                             // IPv6 loopback
  /^fe80:/i,                           // IPv6 link-local
  /^fc00:/i,                           // IPv6 unique local
  /^fd[0-9a-f]{2}:/i,                  // IPv6 unique local
];

const BLOCKED_HOSTNAMES = [
  'localhost',
  'localhost.localdomain',
  'metadata.google.internal',   // GCP metadata
  '169.254.169.254',            // AWS/GCP/Azure metadata
];

/**
 * Check if an IP address is in a private range
 */
export function isPrivateIP(ip: string): boolean {
  return PRIVATE_IP_PATTERNS.some((pattern) => pattern.test(ip));
}

/**
 * Check if a hostname is blocked
 */
export function isBlockedHostname(hostname: string): boolean {
  const lower = hostname.toLowerCase();

  // Direct match
  if (BLOCKED_HOSTNAMES.includes(lower)) {
    return true;
  }

  // Check .local suffix
  if (lower.endsWith('.local')) {
    return true;
  }

  return false;
}

/**
 * Validate a URL for SSRF protection
 * Throws an error if the URL is not allowed
 */
export function validateUrl(urlString: string): void {
  let parsed: URL;
  try {
    parsed = new URL(urlString);
  } catch {
    throw new Error('Invalid URL format');
  }

  // Only allow http/https protocols
  if (!['http:', 'https:'].includes(parsed.protocol)) {
    throw new Error('Only http:// and https:// URLs are supported');
  }

  // Check hostname blocklist
  if (isBlockedHostname(parsed.hostname)) {
    throw new Error('Access to internal/local addresses is not allowed');
  }

  // Check if hostname is a private IP
  if (isPrivateIP(parsed.hostname)) {
    throw new Error('Access to private IP addresses is not allowed');
  }
}
