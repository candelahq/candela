"use client";

import { useState, useEffect, useCallback } from "react";
import { userClient } from "@/lib/api";
import type { BudgetGrant } from "@/gen/candela/types/user_pb";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";

function formatDate(ts?: { seconds: bigint }) {
  if (!ts) return "—";
  return new Date(Number(ts.seconds) * 1000).toLocaleDateString();
}

function defaultExpiry(): string {
  const d = new Date();
  d.setDate(d.getDate() + 30);
  return d.toISOString().split("T")[0];
}

interface GrantsModalProps {
  userId: string;
  email: string;
  onClose: () => void;
}

export function GrantsModal({ userId, email, onClose }: GrantsModalProps) {
  const [grants, setGrants] = useState<BudgetGrant[]>([]);
  const [grantsLoading, setGrantsLoading] = useState(false);
  const [grantsError, setGrantsError] = useState<string | null>(null);
  const [showAddGrant, setShowAddGrant] = useState(false);
  const [grantForm, setGrantForm] = useState({ amountUsd: 0, reason: "", expiresAt: defaultExpiry() });
  const [grantSubmitting, setGrantSubmitting] = useState(false);

  const fetchGrants = useCallback(async () => {
    setGrantsLoading(true);
    setGrantsError(null);
    try {
      const resp = await userClient.listGrants({ userId, activeOnly: false });
      setGrants(resp.grants);
    } catch (err: unknown) {
      setGrantsError(err instanceof Error ? err.message : "Failed to load grants");
    } finally {
      setGrantsLoading(false);
    }
  }, [userId]);

  useEffect(() => {
    fetchGrants();
  }, [fetchGrants]);

  const handleAddGrant = async (e: React.FormEvent) => {
    e.preventDefault();
    setGrantSubmitting(true);
    setGrantsError(null);
    try {
      await userClient.createGrant({
        userId,
        amountUsd: grantForm.amountUsd,
        reason: grantForm.reason,
        startsAt: timestampFromDate(new Date()),
        expiresAt: timestampFromDate(new Date(grantForm.expiresAt + "T23:59:59Z")),
      });
      setShowAddGrant(false);
      setGrantForm({ amountUsd: 0, reason: "", expiresAt: defaultExpiry() });
      fetchGrants();
    } catch (err: unknown) {
      setGrantsError(err instanceof Error ? err.message : "Failed to create grant");
    } finally {
      setGrantSubmitting(false);
    }
  };

  const handleRevokeGrant = async (grantId: string) => {
    setGrantsError(null);
    try {
      await userClient.revokeGrant({ userId, grantId });
      fetchGrants();
    } catch (err: unknown) {
      setGrantsError(err instanceof Error ? err.message : "Failed to revoke grant");
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal modal-wide" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Grants</h3>
          <button type="button" className="modal-close" onClick={onClose}>×</button>
        </div>
        <div className="modal-body">
          <div className="grants-header">
            <p className="modal-subtitle">{email}</p>
            {!showAddGrant && (
              <button
                className="btn btn-sm btn-primary"
                onClick={() => setShowAddGrant(true)}
                id="add-grant-btn"
              >
                + Add Grant
              </button>
            )}
          </div>

          {showAddGrant && (
            <form onSubmit={handleAddGrant} className="grant-add-form">
              <div className="grant-add-row">
                <div className="form-group">
                  <label htmlFor="grant-amount">Amount (USD)</label>
                  <input
                    id="grant-amount"
                    type="number"
                    min="0.01"
                    step="0.01"
                    required
                    value={grantForm.amountUsd || ""}
                    onChange={(e) => setGrantForm({ ...grantForm, amountUsd: Number(e.target.value) })}
                    className="form-input"
                    placeholder="50.00"
                  />
                </div>
                <div className="form-group">
                  <label htmlFor="grant-expires">Expires</label>
                  <input
                    id="grant-expires"
                    type="date"
                    required
                    value={grantForm.expiresAt}
                    onChange={(e) => setGrantForm({ ...grantForm, expiresAt: e.target.value })}
                    className="form-input"
                    min={new Date().toISOString().split("T")[0]}
                  />
                </div>
              </div>
              <div className="form-group">
                <label htmlFor="grant-reason">Reason</label>
                <input
                  id="grant-reason"
                  type="text"
                  required
                  value={grantForm.reason}
                  onChange={(e) => setGrantForm({ ...grantForm, reason: e.target.value })}
                  className="form-input"
                  placeholder="Hackathon weekend, project deadline, etc."
                />
              </div>
              <div className="modal-actions">
                <button type="button" className="btn btn-ghost" onClick={() => setShowAddGrant(false)}>
                  Cancel
                </button>
                <button type="submit" className="btn btn-primary" disabled={grantSubmitting}>
                  {grantSubmitting ? "Creating..." : "Create Grant"}
                </button>
              </div>
            </form>
          )}

          {grantsError && <div className="form-error">{grantsError}</div>}

          {grantsLoading ? (
            <div className="admin-loading"><div className="admin-guard-spinner" /></div>
          ) : grants.length === 0 ? (
            <p className="text-muted text-center admin-empty-state">No grants yet.</p>
          ) : (
            <div className="grants-list">
              {grants.map((grant) => {
                const isExpired = grant.expiresAt
                  ? new Date(Number(grant.expiresAt.seconds) * 1000) < new Date()
                  : false;
                const isFullySpent = grant.spentUsd >= grant.amountUsd;
                const isActive = !isExpired && !isFullySpent;
                return (
                  <div key={grant.id} className={`grant-card ${isActive ? "" : "grant-card-inactive"}`}>
                    <div className="grant-card-header">
                      <div>
                        <span className="grant-amount">${grant.amountUsd.toFixed(2)}</span>
                        <span className={`grant-status-badge ${isActive ? "grant-active" : "grant-expired"}`}>
                          {isExpired ? "Expired" : isFullySpent ? "Spent" : "Active"}
                        </span>
                      </div>
                      {isActive && (
                        <button
                          className="btn btn-sm btn-danger"
                          onClick={() => handleRevokeGrant(grant.id)}
                        >
                          Revoke
                        </button>
                      )}
                    </div>
                    <p className="grant-reason">{grant.reason}</p>
                    <div className="grant-meta">
                      <span>Spent: ${grant.spentUsd.toFixed(2)} / ${grant.amountUsd.toFixed(2)}</span>
                      <span>Expires: {formatDate(grant.expiresAt)}</span>
                    </div>
                    {isActive && (
                      <div className="grant-progress-bar">
                        <div
                          className="grant-progress-fill"
                          style={{ width: `${Math.min((grant.spentUsd / grant.amountUsd) * 100, 100)}%` }}
                        />
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
