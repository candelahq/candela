"use client";

import { HelpTip } from "@/components/Tooltip";

export default function AdminBudgetsPage() {
  return (
    <div className="admin-page">
      <div className="admin-page-header">
        <div>
          <h2 className="admin-page-title">
            Budgets & Grants
            <HelpTip text="Configure daily spending limits and one-time grants. Grants are consumed before the daily budget (grant-first waterfall)." />
          </h2>
          <p className="admin-page-subtitle">Select a user from the Users page to manage budgets</p>
        </div>
      </div>

      <div className="admin-card">
        <h3 className="admin-card-title">
          How Budget Enforcement Works
          <HelpTip text="Budget checks happen at proxy time — before and after each LLM call." />
        </h3>
        <div className="budget-explainer">
          <div className="budget-step">
            <div className="budget-step-number">1</div>
            <div>
              <strong>Pre-flight Check</strong>
              <p className="text-muted">Before each LLM call, the proxy checks if the user has remaining budget or grant balance.</p>
            </div>
          </div>
          <div className="budget-step">
            <div className="budget-step-number">2</div>
            <div>
              <strong>Grant-First Waterfall</strong>
              <HelpTip text="Active grants are consumed first. Once grants are depleted, spending comes from the daily budget." />
              <p className="text-muted">Spending deducts from active grants first, then daily budget.</p>
            </div>
          </div>
          <div className="budget-step">
            <div className="budget-step-number">3</div>
            <div>
              <strong>Post-flight Deduction</strong>
              <p className="text-muted">After each call, actual cost (based on tokens used) is deducted from the user&apos;s balance.</p>
            </div>
          </div>
          <div className="budget-step">
            <div className="budget-step-number">4</div>
            <div>
              <strong>Period Reset</strong>
              <HelpTip text="Daily budgets reset at midnight UTC. Grants have their own expiry dates." />
              <p className="text-muted">Daily budgets auto-reset at midnight UTC. Expired grants are no longer available.</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
