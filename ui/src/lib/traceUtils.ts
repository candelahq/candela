import { SpanStatus } from "@/gen/candela/types/trace_pb";

export interface StatusBadge {
  text: string;
  cls: string;
}

/**
 * Returns the status label and badge class for a trace or span status.
 * Uses the protobuf SpanStatus enum instead of raw integers.
 */
export function statusLabel(status: SpanStatus | number): StatusBadge {
  if (status === SpanStatus.ERROR) {
    return { text: "error", cls: "badge-error" };
  }
  return { text: "ok", cls: "badge-success" };
}
