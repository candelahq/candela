"use client";

import { useState, useEffect } from "react";
import { userClient } from "@/lib/api";
import { BudgetPeriod } from "@/gen/candela/types/user_pb";
import type { UserBudget } from "@/gen/candela/types/user_pb";
import { HelpTip } from "@/components/Tooltip";

interface BudgetModalProps {
  userId: string;
  email: string;
  onClose: () => void;
  onUpdated: () => void;
}

export function BudgetModal({ userId, email, onClose, onUpdated }: BudgetModalProps) {
  const [budgetForm, setBudgetForm] = useState({ limitUsd: 0 });
  const [currentBudget, setCurrentBudget] = useState<UserBudget | null>(null);
  const [budgetLoading, setBudgetLoading] = useState(false);
  const [budgetError, setBudgetError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    const fetchBudget = async () => {
      setBudgetError(null);
      setBudgetLoading(true);
      try {
        const resp = await userClient.getBudget({ userId });
        if (active) {
          setCurrentBudget(resp.budget ?? null);
          setBudgetForm({ limitUsd: resp.budget?.limitUsd ?? 0 });
        }
      } catch {
        if (active) {
          setCurrentBudget(null);
          setBudgetForm({ limitUsd: 0 });
        }
      } finally {
        if (active) {
          setBudgetLoading(false);
        }
      }
    };
    fetchBudget();
    return () => {
      active = false;
    };
  }, [userId]);

  const handleSetBudget = async (e: React.FormEvent) => {
    e.preventDefault();
    setBudgetError(null);
    setBudgetLoading(true);
    try {
      await userClient.setBudget({
        userId,
        limitUsd: budgetForm.limitUsd,
        periodType: BudgetPeriod.DAILY,
      });
      onUpdated();
      onClose();
    } catch (err: unknown) {
      setBudgetError(err instanceof Error ? err.message : "Failed to set budget");
      setBudgetLoading(false);
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Daily Budget</h3>
          <button type="button" className="modal-close" onClick={onClose}>×</button>
        </div>
        <form onSubmit={handleSetBudget} className="modal-body">
          <p className="modal-subtitle">{email}</p>

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
            <button type="button" className="btn btn-ghost" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" className="btn btn-primary" disabled={budgetLoading}>
              {budgetLoading ? "Saving..." : "Save Budget"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
