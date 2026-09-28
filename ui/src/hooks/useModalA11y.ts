"use client";

import { useEffect, useRef } from "react";

interface UseModalA11yOptions {
  onClose: () => void;
  isOpen?: boolean;
}

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * Hook providing WCAG-compliant focus trapping, Escape key closing,
 * and focus restoration for modal dialogs.
 */
export function useModalA11y({ onClose, isOpen = true }: UseModalA11yOptions) {
  const modalRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLElement | null>(null);
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useEffect(() => {
    if (!isOpen) return;

    // Capture the trigger element that opened the modal
    if (typeof document !== "undefined") {
      triggerRef.current = document.activeElement as HTMLElement | null;
    }

    const modal = modalRef.current;
    if (modal) {
      // Find all focusable elements inside the dialog
      const focusable = modal.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR);
      if (focusable.length > 0) {
        // Focus first enabled element, giving preference to inputs over close icon if possible
        const firstInput = modal.querySelector<HTMLElement>(
          'input:not([disabled]), select:not([disabled]), textarea:not([disabled])'
        );
        if (firstInput) {
          firstInput.focus();
        } else {
          focusable[0].focus();
        }
      } else {
        modal.focus();
      }
    }

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onCloseRef.current();
        return;
      }

      if (e.key === "Tab") {
        const dialog = modalRef.current;
        if (!dialog) return;

        const focusableElements = Array.from(
          dialog.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)
        ).filter((el) => {
          if (el.hasAttribute("disabled") || el.getAttribute("aria-hidden") === "true") {
            return false;
          }
          if (typeof window !== "undefined") {
            const style = window.getComputedStyle(el);
            if (style.display === "none" || style.visibility === "hidden") {
              return false;
            }
          }
          return true;
        });

        if (focusableElements.length === 0) {
          e.preventDefault();
          return;
        }

        const firstElement = focusableElements[0];
        const lastElement = focusableElements[focusableElements.length - 1];

        if (e.shiftKey) {
          // Shift + Tab: if on first element, wrap to last
          if (
            document.activeElement === firstElement ||
            !dialog.contains(document.activeElement)
          ) {
            e.preventDefault();
            lastElement.focus();
          }
        } else {
          // Tab: if on last element, wrap to first
          if (
            document.activeElement === lastElement ||
            !dialog.contains(document.activeElement)
          ) {
            e.preventDefault();
            firstElement.focus();
          }
        }
      }
    };

    window.addEventListener("keydown", handleKeyDown);

    return () => {
      window.removeEventListener("keydown", handleKeyDown);
      // Restore focus to original trigger element upon close
      if (
        triggerRef.current &&
        typeof triggerRef.current.focus === "function" &&
        document.contains(triggerRef.current)
      ) {
        triggerRef.current.focus();
      }
    };
  }, [isOpen]);

  return { modalRef };
}
