/** Base URL for the Candela backend API. Configurable via NEXT_PUBLIC_API_URL env var. */
export const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8181";
