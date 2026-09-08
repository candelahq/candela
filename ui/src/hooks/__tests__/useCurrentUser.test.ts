import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useCurrentUser } from "../useCurrentUser";
import { UserRole } from "@/gen/candela/types/user_pb";

const mockGetCurrentUser = vi.fn();

vi.mock("@/lib/api", () => ({
  userClient: {
    getCurrentUser: (...args: unknown[]) => mockGetCurrentUser(...args),
  },
}));

describe("useCurrentUser", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns initial loading state", () => {
    mockGetCurrentUser.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useCurrentUser());

    expect(result.current.isLoading).toBe(true);
    expect(result.current.user).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it("successfully resolves developer user", async () => {
    mockGetCurrentUser.mockResolvedValue({
      user: {
        id: "user-dev-1",
        email: "dev@azra-ai.com",
        role: UserRole.DEVELOPER,
      },
      budget: {
        limitUsd: 100,
        spentUsd: 25,
      },
      activeGrants: [],
      totalRemainingUsd: 75,
    });

    const { result } = renderHook(() => useCurrentUser());

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.error).toBeNull();
    expect(result.current.user).not.toBeNull();
    expect(result.current.user!.email).toBe("dev@azra-ai.com");
    expect(result.current.isAdmin).toBe(false);
    expect(result.current.totalRemainingUsd).toBe(75);
    expect(result.current.budget!.limitUsd).toBe(100);
  });

  it("identifies admin role correctly", async () => {
    mockGetCurrentUser.mockResolvedValue({
      user: {
        id: "admin-1",
        email: "admin@azra-ai.com",
        role: UserRole.ADMIN,
      },
      budget: null,
      activeGrants: [],
      totalRemainingUsd: 0,
    });

    const { result } = renderHook(() => useCurrentUser());

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.isAdmin).toBe(true);
    expect(result.current.user!.role).toBe(UserRole.ADMIN);
  });

  it("handles RPC failure with error message", async () => {
    mockGetCurrentUser.mockRejectedValue(new Error("unauthenticated session"));

    const { result } = renderHook(() => useCurrentUser());

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.error).toBe("unauthenticated session");
    expect(result.current.user).toBeNull();
    expect(result.current.isAdmin).toBe(false);
  });
});
