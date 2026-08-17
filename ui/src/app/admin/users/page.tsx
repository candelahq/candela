"use client";

import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import { userClient } from "@/lib/api";
import { HelpTip } from "@/components/Tooltip";
import { useToast } from "@/components/Toast";
import type { User } from "@/gen/candela/types/user_pb";
import { UserRole, UserStatus } from "@/gen/candela/types/user_pb";
import { CreateUserModal } from "@/components/admin/CreateUserModal";
import { BudgetModal } from "@/components/admin/BudgetModal";
import { GrantsModal } from "@/components/admin/GrantsModal";
import { DeleteUserModal } from "@/components/admin/DeleteUserModal";

const PAGE_SIZE = 10;

interface UsersState {
  users: User[];
  total: number;
  nextPageToken: string;
  isLoading: boolean;
  error: string | null;
}

type Action =
  | { type: "loading" }
  | { type: "success"; users: User[]; total: number; nextPageToken: string }
  | { type: "error"; message: string };

function reducer(state: UsersState, action: Action): UsersState {
  switch (action.type) {
    case "loading":
      return { ...state, isLoading: true, error: null };
    case "success":
      return { users: action.users, total: action.total, nextPageToken: action.nextPageToken, isLoading: false, error: null };
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

export default function AdminUsersPage() {
  const [state, dispatch] = useReducer(reducer, {
    users: [], total: 0, nextPageToken: "", isLoading: true, error: null,
  });
  
  // Table action state
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const { toast } = useToast();

  // Pagination state
  const [currentPageToken, setCurrentPageToken] = useState("");
  const [pageTokenHistory, setPageTokenHistory] = useState<string[]>([]);
  const [statusFilter, setStatusFilter] = useState<UserStatus>(UserStatus.UNSPECIFIED);

  // Modals state
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [budgetModal, setBudgetModal] = useState<{ userId: string; email: string } | null>(null);
  const [grantsModal, setGrantsModal] = useState<{ userId: string; email: string } | null>(null);
  const [deleteModal, setDeleteModal] = useState<{ userId: string; email: string } | null>(null);

  // Stable ref for statusFilter to avoid re-creating fetchUsers on every filter change
  const statusFilterRef = useRef(statusFilter);
  statusFilterRef.current = statusFilter;

  const fetchUsers = useCallback(async (pageToken = ""): Promise<boolean> => {
    dispatch({ type: "loading" });
    try {
      const resp = await userClient.listUsers({
        pagination: { pageSize: PAGE_SIZE, pageToken },
        statusFilter: statusFilterRef.current,
      });
      dispatch({
        type: "success",
        users: resp.users,
        total: resp.pagination?.totalCount ?? 0,
        nextPageToken: resp.pagination?.nextPageToken ?? "",
      });
      return true;
    } catch (err: unknown) {
      dispatch({ type: "error", message: err instanceof Error ? err.message : "Failed to load users" });
      return false;
    }
  }, []);

  useEffect(() => {
    setCurrentPageToken("");
    setPageTokenHistory([]);
    fetchUsers("");
  }, [fetchUsers, statusFilter]);

  const handleNextPage = async () => {
    if (!state.nextPageToken) return;
    const success = await fetchUsers(state.nextPageToken);
    if (success) {
      setPageTokenHistory((prev) => [...prev, currentPageToken]);
      setCurrentPageToken(state.nextPageToken);
    }
  };

  const handlePrevPage = async () => {
    const prev = [...pageTokenHistory];
    const prevToken = prev.pop() ?? "";
    const success = await fetchUsers(prevToken);
    if (success) {
      setPageTokenHistory(prev);
      setCurrentPageToken(prevToken);
    }
  };

  const currentPage = pageTokenHistory.length + 1;
  const totalPages = Math.max(1, Math.ceil(state.total / PAGE_SIZE));

  const handleDeactivate = async (userId: string) => {
    setActionLoading(userId);
    try {
      await userClient.deactivateUser({ id: userId });
      fetchUsers(currentPageToken);
    } catch (err: unknown) {
      toast(err instanceof Error ? err.message : "Failed to deactivate user", "error");
    } finally {
      setActionLoading(null);
    }
  };

  const handleReactivate = async (userId: string) => {
    setActionLoading(userId);
    try {
      await userClient.reactivateUser({ id: userId });
      fetchUsers(currentPageToken);
    } catch (err: unknown) {
      toast(err instanceof Error ? err.message : "Failed to reactivate user", "error");
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
        <div className="admin-header-actions">
          <select
            className="form-input form-input-sm"
            value={statusFilter}
            onChange={(e) => setStatusFilter(Number(e.target.value) as UserStatus)}
            id="status-filter"
          >
            <option value={UserStatus.UNSPECIFIED}>All Statuses</option>
            <option value={UserStatus.PROVISIONED}>Provisioned</option>
            <option value={UserStatus.ACTIVE}>Active</option>
            <option value={UserStatus.INACTIVE}>Inactive</option>
          </select>
          <button
            className="btn btn-primary"
            onClick={() => setShowCreateModal(true)}
            id="create-user-btn"
          >
            + Create User
          </button>
        </div>
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
                <th>Last Seen <HelpTip text="Last web/dashboard login" /></th>
                <th>Last Active <HelpTip text="Last proxy/API token usage" /></th>
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
                      {user.lastSeenAt ? formatDate(user.lastSeenAt) : "Never"}
                    </td>
                    <td className="text-muted">
                      {user.lastActiveAt ? formatDate(user.lastActiveAt) : "Never"}
                    </td>
                    <td>
                      <div className="action-btn-group">
                        <button
                          className="btn btn-sm btn-ghost"
                          onClick={() => setBudgetModal({ userId: user.id, email: user.email })}
                          title="Manage budget"
                        >
                          Budget
                        </button>
                        <button
                          className="btn btn-sm btn-ghost"
                          onClick={() => setGrantsModal({ userId: user.id, email: user.email })}
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
                              onClick={() => setDeleteModal({ userId: user.id, email: user.email })}
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
                  <td colSpan={7} className="text-center text-muted admin-empty-state">
                    No users yet. Create one to get started.
                  </td>
                </tr>
              )}
            </tbody>
          </table>

          {/* Pagination controls */}
          {state.total > PAGE_SIZE && (
            <div className="pagination-controls">
              <button
                className="btn btn-sm btn-ghost"
                onClick={handlePrevPage}
                disabled={pageTokenHistory.length === 0}
                id="prev-page-btn"
              >
                ← Previous
              </button>
              <span className="pagination-info">
                Page {currentPage} of {totalPages}
              </span>
              <button
                className="btn btn-sm btn-ghost"
                onClick={handleNextPage}
                disabled={!state.nextPageToken}
                id="next-page-btn"
              >
                Next →
              </button>
            </div>
          )}
        </div>
      )}

      {showCreateModal && (
        <CreateUserModal
          onClose={() => setShowCreateModal(false)}
          onCreated={() => fetchUsers(currentPageToken)}
        />
      )}

      {budgetModal && (
        <BudgetModal
          userId={budgetModal.userId}
          email={budgetModal.email}
          onClose={() => setBudgetModal(null)}
          onUpdated={() => fetchUsers(currentPageToken)}
        />
      )}

      {grantsModal && (
        <GrantsModal
          userId={grantsModal.userId}
          email={grantsModal.email}
          onClose={() => setGrantsModal(null)}
        />
      )}

      {deleteModal && (
        <DeleteUserModal
          userId={deleteModal.userId}
          email={deleteModal.email}
          onClose={() => setDeleteModal(null)}
          onDeleted={() => fetchUsers(currentPageToken)}
        />
      )}
    </div>
  );
}
