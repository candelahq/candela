"use client";

import { useState } from "react";
import { userClient } from "@/lib/api";

interface DeleteUserModalProps {
  userId: string;
  email: string;
  onClose: () => void;
  onDeleted: () => void;
}

export function DeleteUserModal({ userId, email, onClose, onDeleted }: DeleteUserModalProps) {
  const [deleteConfirmEmail, setDeleteConfirmEmail] = useState("");
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const handleDelete = async (e: React.FormEvent) => {
    e.preventDefault();
    setDeleteError(null);
    setDeleteLoading(true);
    try {
      await userClient.deleteUser({
        id: userId,
        confirmEmail: deleteConfirmEmail,
      });
      onDeleted();
      onClose();
    } catch (err: unknown) {
      setDeleteError(err instanceof Error ? err.message : "Failed to delete user");
      setDeleteLoading(false);
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3 className="delete-modal-title">Delete User</h3>
          <button type="button" className="modal-close" onClick={onClose}>×</button>
        </div>
        <form onSubmit={handleDelete} className="modal-body">
          <div className="delete-warning-banner">
            This will permanently delete <strong>{email}</strong> and all
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
              placeholder={email}
              className="form-input"
              autoComplete="off"
            />
          </div>
          {deleteError && <div className="form-error">{deleteError}</div>}
          <div className="modal-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose}>
              Cancel
            </button>
            <button
              type="submit"
              className="btn btn-danger"
              disabled={deleteLoading || deleteConfirmEmail !== email}
            >
              {deleteLoading ? "Deleting..." : "Delete Permanently"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
