import { describe, it, expect, vi, beforeEach } from "vitest";
import { ConnectError, Code, type AnyMessage } from "@connectrpc/connect";

// Mock Firebase module
const mockCurrentUser = {
  getIdToken: vi.fn(),
};

vi.mock("@/lib/firebase", () => ({
  firebaseAuth: {
    get currentUser() {
      return mockCurrentUser.getIdToken ? mockCurrentUser : null;
    },
  },
}));

// Import after mocking
import { authInterceptor } from "@/lib/connect";

describe("connect authInterceptor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  const createFakeRequest = () => ({
    stream: false,
    service: { typeName: "candela.v1.UserService" },
    method: { name: "GetCurrentUser", I: {} as unknown, O: {} as unknown, kind: 0 },
    url: "http://localhost:8080/candela.v1.UserService/GetCurrentUser",
    init: {},
    header: new Headers(),
    message: {} as AnyMessage,
    signal: new AbortController().signal,
  });

  it("passes through without Authorization header when no user is logged in", async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const user = mockCurrentUser as any;
    const originalGetIdToken = user.getIdToken;
    // Set to null to simulate logged out
    delete user.getIdToken;

    const req = createFakeRequest();
    const next = vi.fn().mockResolvedValue({ message: { ok: true } });

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const res = await authInterceptor(next)(req as any);

    expect(req.header.has("Authorization")).toBe(false);
    expect(next).toHaveBeenCalledWith(req);
    expect(res).toEqual({ message: { ok: true } });

    // Restore
    user.getIdToken = originalGetIdToken;
  });

  it("injects Authorization header when getIdToken succeeds", async () => {
    mockCurrentUser.getIdToken.mockResolvedValueOnce("valid-firebase-token-123");

    const req = createFakeRequest();
    const next = vi.fn().mockResolvedValue({ message: { ok: true } });

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const res = await authInterceptor(next)(req as any);

    expect(mockCurrentUser.getIdToken).toHaveBeenCalledTimes(1);
    expect(mockCurrentUser.getIdToken).toHaveBeenCalledWith();
    expect(req.header.get("Authorization")).toBe("Bearer valid-firebase-token-123");
    expect(next).toHaveBeenCalledWith(req);
    expect(res).toEqual({ message: { ok: true } });
  });

  it("retries with forceRefresh when getIdToken fails initially, and succeeds if retry works", async () => {
    mockCurrentUser.getIdToken
      .mockRejectedValueOnce(new Error("Token cache expired"))
      .mockResolvedValueOnce("refreshed-firebase-token-456");

    const req = createFakeRequest();
    const next = vi.fn().mockResolvedValue({ message: { ok: true } });

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const res = await authInterceptor(next)(req as any);

    expect(mockCurrentUser.getIdToken).toHaveBeenCalledTimes(2);
    expect(mockCurrentUser.getIdToken).toHaveBeenNthCalledWith(1);
    expect(mockCurrentUser.getIdToken).toHaveBeenNthCalledWith(2, true);
    expect(req.header.get("Authorization")).toBe("Bearer refreshed-firebase-token-456");
    expect(next).toHaveBeenCalledWith(req);
    expect(res).toEqual({ message: { ok: true } });
  });

  it("throws typed ConnectError with Code.Unauthenticated when both attempts fail", async () => {
    const errorCause = new Error("Network offline / Token refresh failed");
    mockCurrentUser.getIdToken
      .mockRejectedValueOnce(new Error("Token cache expired"))
      .mockRejectedValueOnce(errorCause);

    const req = createFakeRequest();
    const next = vi.fn();

    let caughtErr: unknown;
    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      await authInterceptor(next)(req as any);
    } catch (err) {
      caughtErr = err;
    }

    expect(caughtErr).toBeInstanceOf(ConnectError);
    const connErr = caughtErr as ConnectError;
    expect(connErr.code).toBe(Code.Unauthenticated);
    expect(connErr.message).toContain("Authentication failed: unable to retrieve ID token");
    expect(next).not.toHaveBeenCalled();
  });

  it("redirects to /login when error indicates expired user session in browser", async () => {
    const originalLocation = window.location;
    // Mock window.location
    delete (window as unknown as { location?: unknown }).location;
    window.location = {
      ...originalLocation,
      pathname: "/today",
      href: "http://localhost:3000/today",
    } as unknown as Location;

    const expiredError = Object.assign(new Error("User token expired"), {
      code: "auth/user-token-expired",
    });

    mockCurrentUser.getIdToken
      .mockRejectedValueOnce(new Error("Initial fail"))
      .mockRejectedValueOnce(expiredError);

    const req = createFakeRequest();
    const next = vi.fn();

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    await expect(authInterceptor(next)(req as any)).rejects.toThrow(ConnectError);
    expect(window.location.href).toBe("/login");

    window.location = originalLocation;
  });
});
