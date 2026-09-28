import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { useModalA11y } from "../useModalA11y";
import { useState } from "react";

function TestModal({ onClose }: { onClose: () => void }) {
  const { modalRef } = useModalA11y({ onClose });

  return (
    <div
      ref={modalRef}
      role="dialog"
      aria-modal="true"
      aria-labelledby="test-modal-title"
      tabIndex={-1}
    >
      <h2 id="test-modal-title">Test Modal</h2>
      <input id="input-1" placeholder="First input" />
      <input id="input-2" placeholder="Second input" />
      <button type="button" onClick={onClose} aria-label="Close dialog">
        Close
      </button>
    </div>
  );
}

function TestContainer() {
  const [open, setOpen] = useState(false);

  return (
    <div>
      <button id="open-btn" onClick={() => setOpen(true)}>
        Open Modal
      </button>
      {open && <TestModal onClose={() => setOpen(false)} />}
    </div>
  );
}

describe("useModalA11y", () => {
  it("focuses the first input inside the modal upon mounting", () => {
    const handleClose = vi.fn();
    render(<TestModal onClose={handleClose} />);

    const input1 = screen.getByPlaceholderText("First input");
    expect(document.activeElement).toBe(input1);
  });

  it("calls onClose when the Escape key is pressed", () => {
    const handleClose = vi.fn();
    render(<TestModal onClose={handleClose} />);

    fireEvent.keyDown(window, { key: "Escape" });
    expect(handleClose).toHaveBeenCalledTimes(1);
  });

  it("traps focus between first and last elements when tabbing", () => {
    const handleClose = vi.fn();
    render(<TestModal onClose={handleClose} />);

    const input1 = screen.getByPlaceholderText("First input");
    const closeBtn = screen.getByRole("button", { name: "Close dialog" });

    // Focus last element (closeBtn)
    closeBtn.focus();
    expect(document.activeElement).toBe(closeBtn);

    // Tab on last element wraps back to first element
    fireEvent.keyDown(window, { key: "Tab", shiftKey: false });
    expect(document.activeElement).toBe(input1);

    // Shift+Tab on first element wraps back to last element
    fireEvent.keyDown(window, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(closeBtn);
  });

  it("restores focus to the trigger button when the modal closes", () => {
    render(<TestContainer />);

    const openBtn = screen.getByRole("button", { name: "Open Modal" });
    openBtn.focus();
    expect(document.activeElement).toBe(openBtn);

    // Open modal
    act(() => {
      fireEvent.click(openBtn);
    });
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    // Close modal via Escape
    act(() => {
      fireEvent.keyDown(window, { key: "Escape" });
    });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    // Focus restored
    expect(document.activeElement).toBe(openBtn);
  });
});
