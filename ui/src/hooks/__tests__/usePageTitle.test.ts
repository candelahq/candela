import { describe, it, expect, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { usePageTitle } from "../usePageTitle";

describe("usePageTitle", () => {
  beforeEach(() => {
    document.title = "Original Title";
  });

  it("sets document.title to formatted title with suffix", () => {
    const { unmount } = renderHook(() => usePageTitle("Dashboard"));
    expect(document.title).toBe("Dashboard | Candela");
    unmount();
  });

  it("defaults to full brand title if empty title provided", () => {
    const { unmount } = renderHook(() => usePageTitle(""));
    expect(document.title).toBe("Candela — LLM Observability");
    unmount();
  });

  it("updates document.title when title argument changes", () => {
    const { rerender, unmount } = renderHook(({ title }: { title: string }) => usePageTitle(title), {
      initialProps: { title: "Traces" },
    });
    expect(document.title).toBe("Traces | Candela");

    rerender({ title: "Trace Detail" });
    expect(document.title).toBe("Trace Detail | Candela");
    unmount();
  });

  it("restores original title when component unmounts", () => {
    document.title = "Candela — LLM Observability";
    const { unmount } = renderHook(() => usePageTitle("Settings"));
    expect(document.title).toBe("Settings | Candela");

    unmount();
    expect(document.title).toBe("Candela — LLM Observability");
  });
});
