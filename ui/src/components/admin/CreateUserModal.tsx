"use client";

import { useState } from "react";
import { userClient } from "@/lib/api";
import { UserRole } from "@/gen/candela/types/user_pb";
import { HelpTip } from "@/components/Tooltip";
import { useCreateUserValidation } from "@/hooks/useProtoValidation";

interface CreateUserModalProps {
  onClose: () => void;
  onCreated: () => void;
}

export function CreateUserModal({ onClose, onCreated }: CreateUserModalProps) {
  const [createForm, setCreateForm] = useState({ email: "", displayName: "", role: UserRole.DEVELOPER, budget: 0 });
  const [createError, setCreateError] = useState<string | null>(null);
  const { validate, getError, clearErrors } = useCreateUserValidation();

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
      clearErrors();
      onCreated();
      onClose();
    } catch (err: unknown) {
      setCreateError(err instanceof Error ? err.message : "Failed to create user");
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Create User</h3>
          <button type="button" className="modal-close" onClick={onClose}>×</button>
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
            <button type="button" className="btn btn-ghost" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" className="btn btn-primary" id="submit-create-user">
              Create User
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
