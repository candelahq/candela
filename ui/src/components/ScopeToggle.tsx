"use client";

import { useScope, type ScopeMode } from "@/components/UserScopeProvider";

/**
 * Pill toggle for switching between "My Data" and "All Data" views.
 * Designed to sit in page headers alongside TimeRangeSelector.
 */
export function ScopeToggle() {
  const { mode, setMode } = useScope();

  const options: { value: ScopeMode; label: string; icon: string }[] = [
    { value: "personal", label: "My Data", icon: "👤" },
    { value: "global", label: "All Data", icon: "🌐" },
  ];

  return (
    <div className="scope-toggle" role="radiogroup" aria-label="Data scope">
      {options.map((opt) => (
        <button
          key={opt.value}
          role="radio"
          aria-checked={mode === opt.value}
          className={`scope-toggle-option ${mode === opt.value ? "scope-toggle-active" : ""}`}
          onClick={() => setMode(opt.value)}
        >
          <span className="scope-toggle-icon">{opt.icon}</span>
          {opt.label}
        </button>
      ))}
    </div>
  );
}
