"use client";

import { useCallback, useEffect, useReducer, useState } from "react";
import { userClient } from "@/lib/api";
import { HelpTip } from "@/components/Tooltip";
import { useCreateUserValidation } from "@/hooks/useProtoValidation";
import type { User, UserBudget, BudgetGrant } from "@/gen/candela/types/user_pb";
import { UserRole, UserStatus, BudgetPeriod } from "@/gen/candela/types/user_pb";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";

interface UsersState {
  users: User[];
  total: number;
  isLoading: boolean;
  error: string | null;
}

type Action =
  | { type: "loading" }
  | { type: "success"; users: User[]; total: number }
  | { type: "error"; message: string };

function reducer(state: UsersState, action: Action): UsersState {
  switch (action.type) {
    case "loading":
      return { ...state, isLoading: true, error: null };
    case "success":
      return { users: action.users, total: action.total, isLoading: false, error: null };
    case "error":
      return { ...state, isLoading: false, error: action.message };
  }
}

const roleLabel = (role: UserRole) => {
  switch (role) {
    case UserRole.DEVELOPER: return "Developer";
    case UserRole.ADMIN: return "Admin";
    default: return "Unknown";
  }
};

const statusLabel = (status: UserStatus) => {
  switch (status) {
    case UserStatus.PROVISIONED: return { label: "Provisioned", className: "status-badge status-provisioned" };
    case UserStatus.ACTIVE: return { label: "Active", className: "status-badge status-active" };
    case UserStatus.INACTIVE: return { label: "Inactive", className: "status-badge status-inactive" };
    default: return { label: "Unknown", className: "status-badge" };
  }
};

function formatDate(ts?: { seconds: bigint }) {
  if (!ts) return "—";
  return new Date(Number(ts.seconds) * 1000).toLocaleDateString();
}

function defaultExpiry(): string {
  const d = new Date();
  d.setDate(d.getDate() + 30);
  return d.toISOString().split("T")[0];
}

export default function AdminUsersPage() {
  const [state, dispatch] = useReducer(reducer, {
    users: [], total: 0, isLoading: true, error: null,
  });
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [createForm, setCreateForm] = useState({ email: "", displayName: "", role: UserRole.DEVELOPER, budget: 0 });
  const [createError, setCreateError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const { validate, getError, clearErrors } = useCreateUserValidation();

  // Budget modal state
  const [budgetModal, setBudgetModal] = useState<{ userId: string; email: string } | null>(null);
  const [budgetForm, setBudgetForm] = useState({ limitUsd: 0 });
  const [currentBudget, setCurrentBudget] = useState<UserBudget | null>(null);
  const [budgetLoading, setBudgetLoading] = useState(false);
  const [budgetError, setBudgetError] = useState<string | null>(null);

  // Grants modal state
  const [grantsModal, setGrantsModal] = useState<{ userId: string; email: string } | null>(null);
  const [grants, setGrants] = useState<BudgetGrant[]>([]);
  const [grantsLoading, setGrantsLoading] = useState(false);
  const [grantsError, setGrantsError] = useState<string | null>(null);
  const [showAddGrant, setShowAddGrant] = useState(false);
  const [grantForm, setGrantForm] = useState({ amountUsd: 0, reason: "", expiresAt: defaultExpiry() });
  const [grantSubmitting, setGrantSubmitting] = useState(false);

  // Delete modal state
  const [deleteModal, setDeleteModal] = useState<{ userId: string; email: string } | null>(null);
  const [deleteConfirmEmail, setDeleteConfirmEmail] = useState("");
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const fetchUsers = useCallback(async () => {
    dispatch({ type: "loading" });
    try {
      const resp = await userClient.listUsers({});
      dispatch({ type: "success", users: resp.users, total: resp.pagination?.totalCount ?? 0 });
    } catch (err: unknown) {
      dispatch({ type: "error", message: err instanceof Error ? err.message : "Failed to load users" });
    }
  }, []);

  useEffect(() => { fetchUsers(); }, [fetchUsers]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreateError(null);

    const valid = await validate({
      email: createForm.email,
      displayName: createForm.displayName,
      role: createForm.role,
      dailyBudgetUsd: createForm.budget,
    });
    if (!valid) return;

    try {
      await userClient.createUser({
        email: createForm.email,
        displayName: createForm.displayName,
        role: createForm.role,
        dailyBudgetUsd: createForm.budget,
      });
      setShowCreateModal(false);
      setCreateForm({ email: "", displayName: "", role: UserRole.DEVELOPER, budget: 0 });
      clearErrors();
      fetchUsers();
    } catch (err: unknown) {
      setCreateError(err instanceof Error ? err.message : "Failed to create user");
    }
  };

  const handleDeactivate = async (userId: string) => {
    setActionLoading(userId);
    try {
      await userClient.deactivateUser({ id: userId });
      fetchUsers();
    } catch (err: unknown) {
      alert(err instanceof Error ? err.message : "Failed to deactivate user");
    } finally {
      setActionLoading(null);
    }
  };

  const handleReactivate = async (userId: string) => {
    setActionLoading(userId);
    try {
      await userClient.reactivateUser({ id: userId });
      fetchUsers();
    } catch (err: unknown) {
      alert(err instanceof Error ? err.message : "Failed to reactivate user");
    } finally {
      setActionLoading(null);
    }
  };

  const openBudgetModal = async (userId: string, email: string) => {
    setBudgetModal({ userId, email });
    setBudgetError(null);
    setBudgetLoading(true);
    try {
      const resp = await userClient.getBudget({ userId });
      setCurrentBudget(resp.budget ?? null);
      setBudgetForm({ limitUsd: resp.budget?.limitUsd ?? 0 });
    } catch {
      setCurrentBudget(null);
      setBudgetForm({ limitUsd: 0 });
    } finally {
      setBudgetLoading(false);
    }
  };

  const handleSetBudget = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!budgetModal) return;
    setBudgetError(null);
    setBudgetLoading(true);
    try {
      await userClient.setBudget({
        userId: budgetModal.userId,
        limitUsd: budgetForm.limitUsd,
        periodType: BudgetPeriod.DAILY,
      });
      setBudgetModal(null);
      fetchUsers();
    } catch (err: unknown) {
      setBudgetError(err instanceof Error ? err.message : "Failed to set budget");
    } finally {
      setBudgetLoading(false);
    }
  };

  // Grants handlers
  const fetchGrants = async (userId: string) => {
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
  };

  const openGrantsModal = (userId: string, email: string) => {
    setGrantsModal({ userId, email });
    setShowAddGrant(false);
    setGrantForm({ amountUsd: 0, reason: "", expiresAt: defaultExpiry() });
    fetchGrants(userId);
  };

  const handleAddGrant = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!grantsModal) return;
    setGrantSubmitting(true);
    setGrantsError(null);
    try {
      await userClient.createGrant({
        userId: grantsModal.userId,
        amountUsd: grantForm.amountUsd,
        reason: grantForm.reason,
        startsAt: timestampFromDate(new Date()),
        expiresAt: timestampFromDate(new Date(grantForm.expiresAt + "T23:59:59Z")),
      });
      setShowAddGrant(false);
      setGrantForm({ amountUsd: 0, reason: "", expiresAt: defaultExpiry() });
      fetchGrants(grantsModal.userId);
    } catch (err: unknown) {
      setGrantsError(err instanceof Error ? err.message : "Failed to create grant");
    } finally {
      setGrantSubmitting(false);
    }
  };

  const handleRevokeGrant = async (grantId: string) => {
    if (!grantsModal) return;
    setGrantsError(null);
    try {
      await userClient.revokeGrant({ userId: grantsModal.userId, grantId });
      fetchGrants(grantsModal.userId);
    } catch (err: unknown) {
      setGrantsError(err instanceof Error ? err.message : "Failed to revoke grant");
    }
  };

  const openDeleteModal = (userId: string, email: string) => {
    setDeleteModal({ userId, email });
    setDeleteConfirmEmail("");
    setDeleteError(null);
  };

  const handleDelete = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!deleteModal) return;
    setDeleteError(null);
    setDeleteLoading(true);
    try {
      await userClient.deleteUser({
        id: deleteModal.userId,
        confirmEmail: deleteConfirmEmail,
      });
      setDeleteModal(null);
      fetchUsers();
    } catch (err: unknown) {
      setDeleteError(err instanceof Error ? err.message : "Failed to delete user");
    } finally {
      setDeleteLoading(false);
    }
  };

  return (
    <div className="admin-page">
      <div className="admin-page-header">
        <div>
          <h2 className="admin-page-title">
            Users
            <HelpTip text="Manage platform users. Users are auto-provisioned on first login via IAP." />
          </h2>
          <p className="admin-page-subtitle">{state.total} users total</p>
        </div>
        <button
          className="btn btn-primary"
          onClick={() => setShowCreateModal(true)}
          id="create-user-btn"
        >
          + Create User
        </button>
      </div>

      {state.error && (
        <div className="admin-error">{state.error}</div>
      )}

      {state.isLoading ? (
        <div className="admin-loading">
          <div className="admin-guard-spinner" />
        </div>
      ) : (
        <div className="admin-table-container">
          <table className="admin-table" id="users-table">
            <thead>
              <tr>
                <th>Email</th>
                <th>Display Name</th>
                <th>Role</th>
                <th>Status</th>
                <th>Last Seen</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {state.users.map((user) => {
                const status = statusLabel(user.status);
                const isInactive = user.status === UserStatus.INACTIVE;
                return (
                  <tr key={user.id}>
                    <td className="mono">{user.email}</td>
                    <td>{user.displayName || "—"}</td>
                    <td>{roleLabel(user.role)}</td>
                    <td><span className={status.className}>{status.label}</span></td>
                    <td className="text-muted">
                      {user.lastSeenAt
                        ? new Date(Number(user.lastSeenAt.seconds) * 1000).toLocaleDateString()
                        : "Never"}
                    </td>
                    <td>
                      <div className="action-btn-group">
                        <button
                          className="btn btn-sm btn-ghost"
                          onClick={() => openBudgetModal(user.id, user.email)}
                          title="Manage budget"
                        >
                          Budget
                        </button>
                        <button
                          className="btn btn-sm btn-ghost"
                          onClick={() => openGrantsModal(user.id, user.email)}
                          title="Manage grants"
                        >
                          Grants
                        </button>
                        {isInactive ? (
                          <>
                            <button
                              className="btn btn-sm btn-success"
                              onClick={() => handleReactivate(user.id)}
                              disabled={actionLoading === user.id}
                            >
                              {actionLoading === user.id ? "..." : "Reactivate"}
                            </button>
                            <button
                              className="btn btn-sm btn-danger"
                              onClick={() => openDeleteModal(user.id, user.email)}
                              title="Permanently delete user"
                            >
                              Delete
                            </button>
                          </>
                        ) : (
                          <button
                            className="btn btn-sm btn-danger"
                            onClick={() => handleDeactivate(user.id)}
                            disabled={actionLoading === user.id}
                          >
                            {actionLoading === user.id ? "..." : "Deactivate"}
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
              {state.users.length === 0 && (
                <tr>
                  <td colSpan={6} className="text-center text-muted admin-empty-state">
                    No users yet. Create one to get started.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}

      {/* Create User Modal */}
      {showCreateModal && (
        <div className="modal-overlay" onClick={() => setShowCreateModal(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Create User</h3>
              <button className="modal-close" onClick={() => setShowCreateModal(false)}>×</button>
            </div>
            <form onSubmit={handleCreate} className="modal-body">
              <div className="form-group">
                <label htmlFor="create-email">
                  Email
                  <HelpTip text="Must be a valid email. User will be pre-provisioned and activated on first login." />
                </label>
                <input
                  id="create-email"
                  type="email"
                  required
                  value={createForm.email}
                  onChange={(e) => setCreateForm({ ...createForm, email: e.target.value })}
                  placeholder="user@company.com"
                  className="form-input"
                />
                {getError("email") && <div className="form-field-error">{getError("email")}</div>}
              </div>
              <div className="form-group">
                <label htmlFor="create-name">Display Name</label>
                <input
                  id="create-name"
                  type="text"
                  value={createForm.displayName}
                  onChange={(e) => setCreateForm({ ...createForm, displayName: e.target.value })}
                  placeholder="Alice Smith"
                  className="form-input"
                />
              </div>
              <div className="form-group">
                <label htmlFor="create-role">Role</label>
                <select
                  id="create-role"
                  value={createForm.role}
                  onChange={(e) => setCreateForm({ ...createForm, role: Number(e.target.value) })}
                  className="form-input"
                >
                  <option value={UserRole.DEVELOPER}>Developer</option>
                  <option value={UserRole.ADMIN}>Admin</option>
                </select>
              </div>
              <div className="form-group">
                <label htmlFor="create-budget">
                  Daily Budget (USD)
                  <HelpTip text="Optional daily spending limit. Set to 0 for no budget. Resets at midnight UTC." />
                </label>
                <input
                  id="create-budget"
                  type="number"
                  min="0"
                  step="0.01"
                  value={createForm.budget}
                  onChange={(e) => setCreateForm({ ...createForm, budget: Number(e.target.value) })}
                  className="form-input"
                />
                {getError("daily_budget_usd") && <div className="form-field-error">{getError("daily_budget_usd")}</div>}
              </div>
              {createError && <div className="form-error">{createError}</div>}
              <div className="modal-actions">
                <button type="button" className="btn btn-ghost" onClick={() => setShowCreateModal(false)}>
                  Cancel
                </button>
                <button type="submit" className="btn btn-primary" id="submit-create-user">
                  Create User
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Budget Modal — daily budgets only */}
      {budgetModal && (
        <div className="modal-overlay" onClick={() => setBudgetModal(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Daily Budget</h3>
              <button className="modal-close" onClick={() => setBudgetModal(null)}>×</button>
            </div>
            <form onSubmit={handleSetBudget} className="modal-body">
              <p className="modal-subtitle">{budgetModal.email}</p>

              {currentBudget && (
                <div className="budget-summary-card">
                  <div className="budget-summary-row">
                    <span className="text-muted">Current limit</span>
                    <span>{currentBudget.limitUsd > 0 ? `$${currentBudget.limitUsd.toFixed(2)}` : "Unlimited"}</span>
                  </div>
                  <div className="budget-summary-row">
                    <span className="text-muted">Spent today</span>
                    <span>${currentBudget.spentUsd.toFixed(2)}</span>
                  </div>
                  <div className="budget-summary-row">
                    <span className="text-muted">Remaining</span>
                    <span className="budget-remaining">
                      {currentBudget.limitUsd > 0
                        ? `$${(currentBudget.limitUsd - currentBudget.spentUsd).toFixed(2)}`
                        : "∞"}
                    </span>
                  </div>
                </div>
              )}

              <div className="form-group">
                <label htmlFor="budget-limit">
                  Daily Limit (USD)
                  <HelpTip text="Spending cap per day. Resets at midnight UTC." />
                </label>
                <input
                  id="budget-limit"
                  type="number"
                  min="0"
                  step="0.01"
                  value={budgetForm.limitUsd}
                  onChange={(e) => setBudgetForm({ limitUsd: Number(e.target.value) })}
                  className="form-input"
                  disabled={budgetLoading}
                />
              </div>
              {budgetError && <div className="form-error">{budgetError}</div>}
              <div className="modal-actions">
                <button type="button" className="btn btn-ghost" onClick={() => setBudgetModal(null)}>
                  Cancel
                </button>
                <button type="submit" className="btn btn-primary" disabled={budgetLoading}>
                  {budgetLoading ? "Saving..." : "Save Budget"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Grants Modal */}
      {grantsModal && (
        <div className="modal-overlay" onClick={() => setGrantsModal(null)}>
          <div className="modal modal-wide" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Grants</h3>
              <button className="modal-close" onClick={() => setGrantsModal(null)}>×</button>
            </div>
            <div className="modal-body">
              <div className="grants-header">
                <p className="modal-subtitle">{grantsModal.email}</p>
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

              {/* Add grant form */}
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

              {/* Grants list */}
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
      )}

      {/* Delete User Modal */}
      {deleteModal && (
        <div className="modal-overlay" onClick={() => setDeleteModal(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h3 className="delete-modal-title">Delete User</h3>
              <button className="modal-close" onClick={() => setDeleteModal(null)}>×</button>
            </div>
            <form onSubmit={handleDelete} className="modal-body">
              <div className="delete-warning-banner">
                This will permanently delete <strong>{deleteModal.email}</strong> and all
                associated data (budgets, grants, audit log). This action cannot be undone.
              </div>
              <div className="form-group">
                <label htmlFor="delete-confirm">
                  Type the user&apos;s email to confirm
                </label>
                <input
                  id="delete-confirm"
                  type="email"
                  required
                  value={deleteConfirmEmail}
                  onChange={(e) => setDeleteConfirmEmail(e.target.value)}
                  placeholder={deleteModal.email}
                  className="form-input"
                  autoComplete="off"
                />
              </div>
              {deleteError && <div className="form-error">{deleteError}</div>}
              <div className="modal-actions">
                <button type="button" className="btn btn-ghost" onClick={() => setDeleteModal(null)}>
                  Cancel
                </button>
                <button
                  type="submit"
                  className="btn btn-danger"
                  disabled={deleteLoading || deleteConfirmEmail !== deleteModal.email}
                >
                  {deleteLoading ? "Deleting..." : "Delete Permanently"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
