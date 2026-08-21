import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BudgetModal } from "../BudgetModal";
import { userClient } from "@/lib/api";
import { BudgetPeriod } from "@/gen/candela/types/user_pb";

vi.mock("@/lib/api", () => ({
  userClient: {
    getBudget: vi.fn(),
    setBudget: vi.fn(),
  },
}));

vi.mock("@/components/Tooltip", () => ({
  HelpTip: () => <span data-testid="help-tip" />
}));

describe("BudgetModal", () => {
  const mockOnClose = vi.fn();
  const mockOnUpdated = vi.fn();
  const props = {
    userId: "user-1",
    email: "test@example.com",
    onClose: mockOnClose,
    onUpdated: mockOnUpdated,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("Shows current budget if exists", async () => {
    vi.mocked(userClient.getBudget).mockResolvedValueOnce({
      budget: { limitUsd: 100, spentUsd: 40, periodType: BudgetPeriod.DAILY } as never
    });
    
    render(<BudgetModal {...props} />);
    
    await waitFor(() => {
      expect(screen.getByText("$100.00")).toBeInTheDocument();
      expect(screen.getByText("$40.00")).toBeInTheDocument();
      expect(screen.getByText("$60.00")).toBeInTheDocument();
    });
  });

  it("Calls setBudget RPC with correct userId and amount", async () => {
    vi.mocked(userClient.getBudget).mockResolvedValueOnce({});
    vi.mocked(userClient.setBudget).mockResolvedValueOnce({});
    
    render(<BudgetModal {...props} />);
    
    await waitFor(() => expect(userClient.getBudget).toHaveBeenCalled());
    
    const input = screen.getByLabelText(/Daily Limit/i);
    fireEvent.change(input, { target: { value: "150" } });
    
    const submitBtn = screen.getByRole("button", { name: /Save Budget/i });
    fireEvent.click(submitBtn);
    
    await waitFor(() => {
      expect(userClient.setBudget).toHaveBeenCalledWith({
        userId: props.userId,
        limitUsd: 150,
        periodType: BudgetPeriod.DAILY,
      });
    });
  });

  it("Calls onUpdated + onClose on success", async () => {
    vi.mocked(userClient.getBudget).mockResolvedValueOnce({});
    vi.mocked(userClient.setBudget).mockResolvedValueOnce({});
    
    render(<BudgetModal {...props} />);
    
    await waitFor(() => expect(userClient.getBudget).toHaveBeenCalled());
    
    const submitBtn = screen.getByRole("button", { name: /Save Budget/i });
    fireEvent.click(submitBtn);
    
    await waitFor(() => {
      expect(mockOnUpdated).toHaveBeenCalled();
      expect(mockOnClose).toHaveBeenCalled();
    });
  });

  it("Shows error on failure", async () => {
    vi.mocked(userClient.getBudget).mockResolvedValueOnce({});
    vi.mocked(userClient.setBudget).mockRejectedValueOnce(new Error("Update failed"));
    
    render(<BudgetModal {...props} />);
    await waitFor(() => expect(userClient.getBudget).toHaveBeenCalled());
    
    const submitBtn = screen.getByRole("button", { name: /Save Budget/i });
    fireEvent.click(submitBtn);
    
    await waitFor(() => {
      expect(screen.getByText("Update failed")).toBeInTheDocument();
    });
  });

  it("Cancel/close works", () => {
    vi.mocked(userClient.getBudget).mockResolvedValueOnce({});
    render(<BudgetModal {...props} />);
    
    const cancelBtn = screen.getByRole("button", { name: /Cancel/i });
    fireEvent.click(cancelBtn);
    
    expect(mockOnClose).toHaveBeenCalled();
  });
});
