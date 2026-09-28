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

  it("does not restart focus lifecycle when onClose callback reference changes while modal is open", () => {
    function ReRenderingModal({ count, onClose }: { count: number; onClose: () => void }) {
      return (
        <div>
          <span data-testid="count">{count}</span>
          <TestModal onClose={onClose} />
        </div>
      );
    }

    const close1 = vi.fn();
    const { rerender } = render(<ReRenderingModal count={1} onClose={close1} />);

    const input2 = screen.getByPlaceholderText("Second input");
    input2.focus();
    expect(document.activeElement).toBe(input2);

    // Re-render with brand new onClose reference (e.g. parent state change)
    const close2 = vi.fn();
    rerender(<ReRenderingModal count={2} onClose={close2} />);

    // Focus must NOT be pulled back to input1 or reset
    expect(document.activeElement).toBe(input2);

    // Escape should invoke the latest onClose callback
    fireEvent.keyDown(window, { key: "Escape" });
    expect(close2).toHaveBeenCalledTimes(1);
    expect(close1).not.toHaveBeenCalled();
  });

  it("skips disabled inputs and focuses the first enabled interactive control upon mounting", () => {
    function DisabledInputModal({ onClose }: { onClose: () => void }) {
      const { modalRef } = useModalA11y({ onClose });
      return (
        <div ref={modalRef} role="dialog">
          <input id="disabled-input" disabled placeholder="Disabled input" />
          <input id="enabled-input" placeholder="Enabled input" />
          <button type="button">Submit</button>
        </div>
      );
    }

    render(<DisabledInputModal onClose={() => {}} />);
    const enabledInput = screen.getByPlaceholderText("Enabled input");
    expect(document.activeElement).toBe(enabledInput);
  });
});
