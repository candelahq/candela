import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { GrantsModal } from "../GrantsModal";
import { userClient } from "@/lib/api";

vi.mock("@/lib/api", () => ({
  userClient: {
    listGrants: vi.fn(),
    createGrant: vi.fn(),
    revokeGrant: vi.fn(),
  },
}));

// Mock timestamp function to avoid issues with @bufbuild
vi.mock("@bufbuild/protobuf/wkt", () => ({
  timestampFromDate: vi.fn((d) => ({ seconds: BigInt(Math.floor(d.getTime() / 1000)) })),
}));

describe("GrantsModal", () => {
  const mockOnClose = vi.fn();
  const props = {
    userId: "user-1",
    email: "test@example.com",
    onClose: mockOnClose,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("Lists existing grants", async () => {
    const futureDate = new Date();
    futureDate.setDate(futureDate.getDate() + 10);
    vi.mocked(userClient.listGrants).mockResolvedValueOnce({
      grants: [
        {
          id: "grant-1",
          amountUsd: 100,
          spentUsd: 0,
          reason: "Test grant",
          expiresAt: { seconds: BigInt(Math.floor(futureDate.getTime() / 1000)) },
        }
      ]
    } as never);

    render(<GrantsModal {...props} />);

    await waitFor(() => {
      expect(screen.getByText("Test grant")).toBeInTheDocument();
      expect(screen.getByText("$100.00")).toBeInTheDocument();
    });
  });

  it("Can create new grant with amount/reason/dates", async () => {
    vi.mocked(userClient.listGrants).mockResolvedValue({ grants: [] });
    vi.mocked(userClient.createGrant).mockResolvedValueOnce({});
    
    render(<GrantsModal {...props} />);
    
    await waitFor(() => expect(userClient.listGrants).toHaveBeenCalledTimes(1));

    const addBtn = screen.getByText(/\+ Add Grant/i);
    fireEvent.click(addBtn);

    const amountInput = screen.getByLabelText(/Amount/i);
    const reasonInput = screen.getByLabelText(/Reason/i);
    
    fireEvent.change(amountInput, { target: { value: "50" } });
    fireEvent.change(reasonInput, { target: { value: "New reason" } });
    
    const submitBtn = screen.getByRole("button", { name: /Create Grant/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(userClient.createGrant).toHaveBeenCalledWith(expect.objectContaining({
        userId: props.userId,
        amountUsd: 50,
        reason: "New reason",
      }));
      // Should refetch
      expect(userClient.listGrants).toHaveBeenCalledTimes(2);
    });
  });

  it("Can revoke a grant", async () => {
    const futureDate = new Date();
    futureDate.setDate(futureDate.getDate() + 10);
    vi.mocked(userClient.listGrants).mockResolvedValue({
      grants: [
        {
          id: "grant-1",
          amountUsd: 100,
          spentUsd: 0,
          reason: "Test grant",
          expiresAt: { seconds: BigInt(Math.floor(futureDate.getTime() / 1000)) },
        }
      ]
    } as never);
    vi.mocked(userClient.revokeGrant).mockResolvedValueOnce({});

    render(<GrantsModal {...props} />);

    await waitFor(() => expect(screen.getByText("Test grant")).toBeInTheDocument());

    const revokeBtn = screen.getByRole("button", { name: /Revoke/i });
    fireEvent.click(revokeBtn);

    await waitFor(() => {
      expect(userClient.revokeGrant).toHaveBeenCalledWith({
        userId: props.userId,
        grantId: "grant-1",
      });
      expect(userClient.listGrants).toHaveBeenCalledTimes(2); // Initial + after revoke
    });
  });

  it("Shows error on failure", async () => {
    vi.mocked(userClient.listGrants).mockRejectedValueOnce(new Error("List failed"));

    render(<GrantsModal {...props} />);

    await waitFor(() => {
      expect(screen.getByText("List failed")).toBeInTheDocument();
    });
  });

  it("Close works", () => {
    vi.mocked(userClient.listGrants).mockResolvedValueOnce({ grants: [] });
    render(<GrantsModal {...props} />);
    
    const closeBtn = screen.getByText("×");
    fireEvent.click(closeBtn);
    
    expect(mockOnClose).toHaveBeenCalled();
  });
});
