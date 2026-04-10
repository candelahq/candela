import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Standalone mode — produces a minimal Node.js server in .next/standalone.
  // Includes only the necessary node_modules for production.
  output: "standalone",

  // Proxy API calls to the Go backend (runs as a sidecar in the same container).
  async rewrites() {
    const backendUrl = process.env.BACKEND_URL || "http://localhost:8181";
    return {
      // "beforeFiles" rewrites are checked before pages/public files.
      beforeFiles: [
        // ConnectRPC services (paths containing dots like candela.v1.*)
        {
          source: "/:path(candela\\.v1\\..+)",
          destination: `${backendUrl}/:path`,
        },
        // LLM proxy routes
        {
          source: "/proxy/:path*",
          destination: `${backendUrl}/proxy/:path*`,
        },
        // Health check
        {
          source: "/healthz",
          destination: `${backendUrl}/healthz`,
        },
      ],
    };
  },
};

export default nextConfig;
