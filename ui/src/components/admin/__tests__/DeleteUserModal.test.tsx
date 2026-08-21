import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { DeleteUserModal } from "../DeleteUserModal";
import { userClient } from "@/lib/api";

vi.mock("@/lib/api", () => ({
  userClient: {
    deleteUser: vi.fn(),
  },
}));

describe("DeleteUserModal", () => {
  const mockOnClose = vi.fn();
  const mockOnDeleted = vi.fn();
  const props = {
    userId: "user-1",
    email: "test@example.com",
    onClose: mockOnClose,
    onDeleted: mockOnDeleted,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("Renders with user email in warning", () => {
    render(<DeleteUserModal {...props} />);
    expect(screen.getByText(/This will permanently delete/i)).toBeInTheDocument();
    expect(screen.getByText("test@example.com")).toBeInTheDocument();
  });
  
  it("Delete button disabled when confirmation empty", () => {
    render(<DeleteUserModal {...props} />);
    const button = screen.getByRole("button", { name: /Delete Permanently/i });
    expect(button).toBeDisabled();
  });

  it("Delete button disabled when confirmation doesn't match email", () => {
    render(<DeleteUserModal {...props} />);
    const input = screen.getByLabelText(/Type the user's email to confirm/i);
    fireEvent.change(input, { target: { value: "wrong@example.com" } });
    const button = screen.getByRole("button", { name: /Delete Permanently/i });
    expect(button).toBeDisabled();
  });

  it("Delete button enabled when confirmation matches email exactly", () => {
    render(<DeleteUserModal {...props} />);
    const input = screen.getByLabelText(/Type the user's email to confirm/i);
    fireEvent.change(input, { target: { value: props.email } });
    const button = screen.getByRole("button", { name: /Delete Permanently/i });
    expect(button).not.toBeDisabled();
  });

  it("Calls deleteUser RPC on submit with correct args", async () => {
    vi.mocked(userClient.deleteUser).mockResolvedValueOnce({});
    render(<DeleteUserModal {...props} />);
    const input = screen.getByLabelText(/Type the user's email to confirm/i);
    fireEvent.change(input, { target: { value: props.email } });
    const button = screen.getByRole("button", { name: /Delete Permanently/i });
    fireEvent.click(button);
    expect(userClient.deleteUser).toHaveBeenCalledWith({ id: props.userId, confirmEmail: props.email });
  });

  it("Calls onDeleted + onClose on success", async () => {
    vi.mocked(userClient.deleteUser).mockResolvedValueOnce({});
    render(<DeleteUserModal {...props} />);
    const input = screen.getByLabelText(/Type the user's email to confirm/i);
    fireEvent.change(input, { target: { value: props.email } });
    const button = screen.getByRole("button", { name: /Delete Permanently/i });
    fireEvent.click(button);
    
    await waitFor(() => {
      expect(mockOnDeleted).toHaveBeenCalled();
      expect(mockOnClose).toHaveBeenCalled();
    });
  });

  it("Shows error message on RPC failure", async () => {
    vi.mocked(userClient.deleteUser).mockRejectedValueOnce(new Error("Network error"));
    render(<DeleteUserModal {...props} />);
    const input = screen.getByLabelText(/Type the user's email to confirm/i);
    fireEvent.change(input, { target: { value: props.email } });
    const button = screen.getByRole("button", { name: /Delete Permanently/i });
    fireEvent.click(button);
    
    await waitFor(() => {
      expect(screen.getByText("Network error")).toBeInTheDocument();
    });
  });

  it("Shows 'Deleting...' loading state", async () => {
    let resolvePromise: (value: unknown) => void;
    vi.mocked(userClient.deleteUser).mockReturnValueOnce(new Promise((resolve) => {
      resolvePromise = resolve;
    }));
    
    render(<DeleteUserModal {...props} />);
    const input = screen.getByLabelText(/Type the user's email to confirm/i);
    fireEvent.change(input, { target: { value: props.email } });
    const button = screen.getByRole("button", { name: /Delete Permanently/i });
    fireEvent.click(button);
    
    expect(screen.getByRole("button", { name: /Deleting.../i })).toBeDisabled();
    resolvePromise({});
  });

  it("Close button calls onClose", () => {
    render(<DeleteUserModal {...props} />);
    const closeBtn = screen.getByText("×");
    fireEvent.click(closeBtn);
    expect(mockOnClose).toHaveBeenCalled();
  });

  it("Overlay click calls onClose", () => {
    render(<DeleteUserModal {...props} />);
    // The overlay is the root element that doesn't propagate clicks to itself from inner modal
    const overlay = screen.getByText("Delete User").closest(".modal-overlay");
    if (overlay) {
      fireEvent.click(overlay);
    }
    expect(mockOnClose).toHaveBeenCalled();
  });
});
