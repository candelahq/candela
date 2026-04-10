"use client";

import { useCallback, useEffect, useReducer, useState } from "react";
import { userClient } from "@/lib/api";
import { HelpTip } from "@/components/Tooltip";
import type { User } from "@/gen/types/user_pb";

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

const roleLabel = (role: number) => {
  switch (role) {
    case 1: return "Admin";
    case 2: return "Developer";
    default: return "Unknown";
  }
};

const statusLabel = (status: number) => {
  switch (status) {
    case 1: return { label: "Provisioned", className: "status-badge status-provisioned" };
    case 2: return { label: "Active", className: "status-badge status-active" };
    case 3: return { label: "Inactive", className: "status-badge status-inactive" };
    default: return { label: "Unknown", className: "status-badge" };
  }
};

export default function AdminUsersPage() {
  const [state, dispatch] = useReducer(reducer, {
    users: [], total: 0, isLoading: true, error: null,
  });
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [createForm, setCreateForm] = useState({ email: "", displayName: "", role: 2, budget: 0 });
  const [createError, setCreateError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);

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
    try {
      await userClient.createUser({
        email: createForm.email,
        displayName: createForm.displayName,
        role: createForm.role,
        monthlyBudgetUsd: createForm.budget,
      });
      setShowCreateModal(false);
      setCreateForm({ email: "", displayName: "", role: 2, budget: 0 });
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
                      {user.status === 3 ? (
                        <button
                          className="btn btn-sm btn-success"
                          onClick={() => handleReactivate(user.id)}
                          disabled={actionLoading === user.id}
                        >
                          {actionLoading === user.id ? "..." : "Reactivate"}
                        </button>
                      ) : (
                        <button
                          className="btn btn-sm btn-danger"
                          onClick={() => handleDeactivate(user.id)}
                          disabled={actionLoading === user.id}
                        >
                          {actionLoading === user.id ? "..." : "Deactivate"}
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
              {state.users.length === 0 && (
                <tr>
                  <td colSpan={6} className="text-center text-muted" style={{ padding: "2rem" }}>
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
                  <option value={2}>Developer</option>
                  <option value={1}>Admin</option>
                </select>
              </div>
              <div className="form-group">
                <label htmlFor="create-budget">
                  Monthly Budget (USD)
                  <HelpTip text="Optional spending limit. Set to 0 for no budget. Resets at the start of each period." />
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
    </div>
  );
}
