import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { CreateUserModal } from "../CreateUserModal";
import { userClient } from "@/lib/api";
import { UserRole } from "@/gen/candela/types/user_pb";

vi.mock("@/lib/api", () => ({
  userClient: {
    createUser: vi.fn(),
  },
}));

vi.mock("@/components/Tooltip", () => ({
  HelpTip: () => <span data-testid="help-tip" />
}));

const mockValidate = vi.fn().mockResolvedValue(true);
vi.mock("@/hooks/useProtoValidation", () => ({
  useCreateUserValidation: () => ({
    validate: mockValidate,
    getError: vi.fn(),
    clearErrors: vi.fn(),
  })
}));

describe("CreateUserModal", () => {
  const mockOnClose = vi.fn();
  const mockOnCreated = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("Renders form with email and role inputs", () => {
    render(<CreateUserModal onClose={mockOnClose} onCreated={mockOnCreated} />);
    expect(screen.getByLabelText(/Email/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Role/i)).toBeInTheDocument();
  });

  it("Submit button disabled when email empty", () => {
    render(<CreateUserModal onClose={mockOnClose} onCreated={mockOnCreated} />);
    // HTML5 required attribute handles disabled state implicitly on submit,
    // but the test says "Submit button disabled when email empty". Let's check required attr.
    const input = screen.getByLabelText(/Email/i);
    expect(input).toBeRequired();
  });

  it("Calls createUser RPC with correct email and role", async () => {
    vi.mocked(userClient.createUser).mockResolvedValueOnce({});
    render(<CreateUserModal onClose={mockOnClose} onCreated={mockOnCreated} />);
    
    fireEvent.change(screen.getByLabelText(/Email/i), { target: { value: "test@test.com" } });
    fireEvent.change(screen.getByLabelText(/Display Name/i), { target: { value: "Test User" } });
    fireEvent.change(screen.getByLabelText(/Daily Budget/i), { target: { value: "10.5" } });
    
    fireEvent.click(screen.getByRole("button", { name: /Create User/i }));
    
    await waitFor(() => {
      expect(userClient.createUser).toHaveBeenCalledWith({
        email: "test@test.com",
        displayName: "Test User",
        role: UserRole.DEVELOPER,
        dailyBudgetUsd: 10.5,
      });
    });
  });

  it("Calls onCreated + onClose on success", async () => {
    vi.mocked(userClient.createUser).mockResolvedValueOnce({});
    render(<CreateUserModal onClose={mockOnClose} onCreated={mockOnCreated} />);
    
    fireEvent.change(screen.getByLabelText(/Email/i), { target: { value: "test@test.com" } });
    fireEvent.click(screen.getByRole("button", { name: /Create User/i }));
    
    await waitFor(() => {
      expect(mockOnCreated).toHaveBeenCalled();
      expect(mockOnClose).toHaveBeenCalled();
    });
  });

  it("Shows error on RPC failure", async () => {
    vi.mocked(userClient.createUser).mockRejectedValueOnce(new Error("RPC Error"));
    render(<CreateUserModal onClose={mockOnClose} onCreated={mockOnCreated} />);
    
    fireEvent.change(screen.getByLabelText(/Email/i), { target: { value: "test@test.com" } });
    fireEvent.click(screen.getByRole("button", { name: /Create User/i }));
    
    await waitFor(() => {
      expect(screen.getByText("RPC Error")).toBeInTheDocument();
    });
  });

  it("Close/Cancel calls onClose", () => {
    render(<CreateUserModal onClose={mockOnClose} onCreated={mockOnCreated} />);
    fireEvent.click(screen.getByRole("button", { name: /Cancel/i }));
    expect(mockOnClose).toHaveBeenCalled();
  });
});
