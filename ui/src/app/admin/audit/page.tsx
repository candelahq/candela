"use client";

import { useCallback, useEffect, useReducer, useState } from "react";
import { userClient } from "@/lib/api";
import { HelpTip } from "@/components/Tooltip";
import type { AuditEntry } from "@/gen/types/user_pb";

interface AuditState {
  entries: AuditEntry[];
  isLoading: boolean;
  error: string | null;
}

type Action =
  | { type: "loading" }
  | { type: "success"; entries: AuditEntry[] }
  | { type: "error"; message: string };

function reducer(state: AuditState, action: Action): AuditState {
  switch (action.type) {
    case "loading": return { ...state, isLoading: true, error: null };
    case "success": return { entries: action.entries, isLoading: false, error: null };
    case "error": return { ...state, isLoading: false, error: action.message };
  }
}

const actionIcons: Record<string, string> = {
  create_user: "👤",
  deactivate_user: "🚫",
  reactivate_user: "✅",
  set_budget: "💰",
  reset_spend: "🔄",
  create_grant: "🎁",
  revoke_grant: "❌",
};

export default function AdminAuditPage() {
  const [userId, setUserId] = useState("");
  const [state, dispatch] = useReducer(reducer, { entries: [], isLoading: false, error: null });

  const fetchAudit = useCallback(async (uid: string) => {
    if (!uid) return;
    dispatch({ type: "loading" });
    try {
      const resp = await userClient.listAuditLog({ userId: uid, limit: 100 });
      dispatch({ type: "success", entries: resp.entries });
    } catch (err: unknown) {
      dispatch({ type: "error", message: err instanceof Error ? err.message : "Failed to load audit log" });
    }
  }, []);

  useEffect(() => {
    if (userId) fetchAudit(userId);
  }, [userId, fetchAudit]);

  return (
    <div className="admin-page">
      <div className="admin-page-header">
        <div>
          <h2 className="admin-page-title">
            Audit Log
            <HelpTip text="All admin actions are automatically recorded. Enter a user ID to view their audit trail." />
          </h2>
        </div>
      </div>

      <div className="admin-search-bar">
        <input
          type="text"
          placeholder="Enter user ID..."
          value={userId}
          onChange={(e) => setUserId(e.target.value)}
          className="form-input"
          id="audit-user-id"
        />
      </div>

      {state.error && <div className="admin-error">{state.error}</div>}

      {state.isLoading ? (
        <div className="admin-loading"><div className="admin-guard-spinner" /></div>
      ) : state.entries.length > 0 ? (
        <div className="audit-timeline">
          {state.entries.map((entry) => (
            <div key={entry.id} className="audit-entry">
              <div className="audit-entry-icon">
                {actionIcons[entry.action] || "📝"}
              </div>
              <div className="audit-entry-content">
                <div className="audit-entry-action">
                  <strong>{entry.action.replace(/_/g, " ")}</strong>
                  <span className="text-muted"> by {entry.actorEmail}</span>
                </div>
                {entry.details && (
                  <pre className="audit-entry-details">{entry.details}</pre>
                )}
                <div className="audit-entry-time text-muted">
                  {entry.timestamp
                    ? new Date(Number(entry.timestamp.seconds) * 1000).toLocaleString()
                    : "—"}
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : userId ? (
        <p className="text-muted" style={{ padding: "2rem", textAlign: "center" }}>
          No audit entries found for this user.
        </p>
      ) : null}
    </div>
  );
}
