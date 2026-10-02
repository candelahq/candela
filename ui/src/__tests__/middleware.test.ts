import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { NextRequest } from "next/server";
import {
  middleware,
  parseJwtPayload,
  renderAccessDenied,
} from "@/middleware";

function createMockJwt(payload: Record<string, unknown>): string {
  const header = btoa(JSON.stringify({ alg: "HS256", typ: "JWT" }));
  const body = btoa(JSON.stringify(payload))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
  return `${header}.${body}.mock-signature`;
}

describe("middleware helper functions", () => {
  it("parseJwtPayload extracts claims correctly", () => {
    const token = createMockJwt({
      sub: "user-123",
      email: "alice@test.com",
      role: "admin",
      exp: Math.floor(Date.now() / 1000) + 3600,
    });
    const parsed = parseJwtPayload(token);
    expect(parsed).not.toBeNull();
    expect(parsed?.sub).toBe("user-123");
    expect(parsed?.email).toBe("alice@test.com");
    expect(parsed?.role).toBe("admin");
  });

  it("parseJwtPayload handles malformed tokens safely", () => {
    expect(parseJwtPayload("not-a-token")).toBeNull();
    expect(parseJwtPayload("a.b")).toBeNull();
    expect(parseJwtPayload("")).toBeNull();
  });

  it("renderAccessDenied returns 403 Forbidden with Access Denied HTML", async () => {
    const res = renderAccessDenied();
    expect(res.status).toBe(403);
    expect(res.headers.get("content-type")).toContain("text/html");
    const text = await res.text();
    expect(text).toContain("Access Denied");
    expect(text).toContain("You need admin privileges");
    expect(text).not.toContain("/_next/static/chunks/app/admin");
  });
});

describe("server-side route guard middleware", () => {
  const originalEnv = { ...process.env };

  beforeEach(() => {
    process.env = { ...originalEnv };
  });

  afterEach(() => {
    process.env = { ...originalEnv };
    vi.restoreAllMocks();
  });

  it("passes through non-admin routes untouched", async () => {
    const req = new NextRequest("http://localhost:3000/traces");
    const res = await middleware(req);
    expect(res.status).toBe(200);
    expect(res.headers.get("x-middleware-next")).toBe("1");
  });

  it("blocks non-admin user on /admin/users with 403 in test/dev mode", async () => {
    process.env.NODE_ENV = "test";
    const req = new NextRequest("http://localhost:3000/admin/users", {
      headers: {
        cookie: "candela_role=developer",
      },
    });
    const res = await middleware(req);
    expect(res.status).toBe(403);
    const body = await res.text();
    expect(body).toContain("Access Denied");
    expect(body).not.toContain("/_next/static/chunks/app/admin");
  });

  it("allows admin user with candela_role=admin cookie in test/dev mode", async () => {
    process.env.NODE_ENV = "test";
    const req = new NextRequest("http://localhost:3000/admin/users", {
      headers: {
        cookie: "candela_role=admin",
      },
    });
    const res = await middleware(req);
    expect(res.status).toBe(200);
    expect(res.headers.get("x-middleware-next")).toBe("1");
  });

  it("allows admin user with valid admin JWT in test/dev mode", async () => {
    process.env.NODE_ENV = "test";
    const token = createMockJwt({
      role: "admin",
      exp: Math.floor(Date.now() / 1000) + 3600,
    });
    const req = new NextRequest("http://localhost:3000/admin/budgets", {
      headers: {
        cookie: `__session=${token}`,
      },
    });
    const res = await middleware(req);
    expect(res.status).toBe(200);
    expect(res.headers.get("x-middleware-next")).toBe("1");
  });

  it("redirects unauthenticated user to /login in production mode", async () => {
    process.env.NODE_ENV = "production";
    process.env.NEXT_PUBLIC_FIREBASE_API_KEY = "mock-api-key";

    const req = new NextRequest("http://localhost:3000/admin/users");
    const res = await middleware(req);
    expect(res.status).toBe(307);
    expect(res.headers.get("location")).toContain("/login?from=%2Fadmin%2Fusers");
  });

  it("redirects expired session to /login in production mode", async () => {
    process.env.NODE_ENV = "production";
    process.env.NEXT_PUBLIC_FIREBASE_API_KEY = "mock-api-key";

    const expiredToken = createMockJwt({
      role: "admin",
      exp: Math.floor(Date.now() / 1000) - 60, // Expired 1 minute ago
    });

    const req = new NextRequest("http://localhost:3000/admin/users", {
      headers: {
        cookie: `__session=${expiredToken}`,
      },
    });
    const res = await middleware(req);
    expect(res.status).toBe(307);
    expect(res.headers.get("location")).toContain("/login?from=%2Fadmin%2Fusers");
  });

  it("allows valid admin JWT token in production mode", async () => {
    process.env.NODE_ENV = "production";
    process.env.NEXT_PUBLIC_FIREBASE_API_KEY = "mock-api-key";

    const validToken = createMockJwt({
      role: "admin",
      exp: Math.floor(Date.now() / 1000) + 3600,
    });

    const req = new NextRequest("http://localhost:3000/admin/users", {
      headers: {
        cookie: `__session=${validToken}`,
      },
    });
    const res = await middleware(req);
    expect(res.status).toBe(200);
    expect(res.headers.get("x-middleware-next")).toBe("1");
  });

  it("blocks non-admin JWT token with 403 in production mode when backend confirms non-admin", async () => {
    process.env.NODE_ENV = "production";
    process.env.NEXT_PUBLIC_FIREBASE_API_KEY = "mock-api-key";

    const devToken = createMockJwt({
      role: "developer",
      exp: Math.floor(Date.now() / 1000) + 3600,
    });

    // Mock fetch to backend returning developer role
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ user: { role: 1 } }),
    }) as unknown as typeof fetch;

    const req = new NextRequest("http://localhost:3000/admin/users", {
      headers: {
        cookie: `__session=${devToken}`,
      },
    });
    const res = await middleware(req);
    expect(res.status).toBe(403);
    const body = await res.text();
    expect(body).toContain("Access Denied");
  });
});
