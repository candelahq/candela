/**
 * Base URL for the Candela backend API.
 * - Development: defaults to http://localhost:8181 (direct to Go server)
 * - Production: empty string = same origin (Next.js rewrites proxy to Go)
 */
export const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8181";
