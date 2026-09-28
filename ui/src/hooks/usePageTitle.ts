"use client";

import { useEffect } from "react";

const TITLE_SUFFIX = "Candela";

/**
 * Hook to set document.title in client components.
 * Formats title as: `${title} | Candela`.
 * If no title is provided, defaults to "Candela — LLM Observability".
 * Restores original document.title on unmount.
 */
export function usePageTitle(title?: string) {
  useEffect(() => {
    if (typeof document === "undefined") return;

    const previousTitle = document.title;
    if (title && title.trim().length > 0) {
      document.title = `${title.trim()} | ${TITLE_SUFFIX}`;
    } else {
      document.title = "Candela — LLM Observability";
    }

    return () => {
      document.title = previousTitle;
    };
  }, [title]);
}
