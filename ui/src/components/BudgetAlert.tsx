"use client";

import { useUsage } from "@/hooks/useUsage";
import { useEffect, useState } from "react";
import Link from "next/link";
import { BUDGET_ALERT_THRESHOLD } from "@/lib/constants";

/** Global alert banner that appears when a user is nearing their budget limit. */
export function BudgetAlert() {
  const { data, loading } = useUsage();
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (!loading && data?.budget) {
      const { percentUsed } = data.budget;
      // Show alert if threshold exceeded
      if (percentUsed > BUDGET_ALERT_THRESHOLD) {
        setVisible(true);
      }
    }
  }, [data, loading]);

  if (!visible || !data?.budget) return null;

  const { percentUsed, remainingUsd } = data.budget;
  const isOver = percentUsed >= 100;

  return (
    <div className={`budget-alert-banner ${isOver ? "over" : "near"}`}>
      <div className="alert-content">
        <span className="alert-icon">{isOver ? "⚠️" : "🔔"}</span>
        <span className="alert-text">
          {isOver
            ? "You have exhausted your budget for this period."
            : `You've used ${percentUsed.toFixed(0)}% of your budget. $${remainingUsd.toFixed(2)} remaining.`}
        </span>
        <Link href="/usage" className="alert-link">
          View Details
        </Link>
      </div>
      <button onClick={() => setVisible(false)} className="alert-close">
        &times;
      </button>

      <style jsx>{`
        .budget-alert-banner {
          width: 100%;
          padding: 8px 16px;
          display: flex;
          justify-content: center;
          align-items: center;
          font-size: 13px;
          font-weight: 500;
          color: white;
          animation: slideDown 0.3s ease-out;
          position: sticky;
          top: 0;
          z-index: 100;
        }
        .budget-alert-banner.near {
          background: linear-gradient(90deg, #f59e0b, #d97706);
        }
        .budget-alert-banner.over {
          background: linear-gradient(90deg, #ef4444, #dc2626);
        }
        .alert-content {
          display: flex;
          align-items: center;
          gap: 12px;
        }
        .alert-icon {
          font-size: 16px;
        }
        .alert-link {
          text-decoration: underline;
          margin-left: 8px;
          opacity: 0.9;
        }
        .alert-link:hover {
          opacity: 1;
        }
        .alert-close {
          background: none;
          border: none;
          color: white;
          font-size: 18px;
          cursor: pointer;
          margin-left: 20px;
          opacity: 0.7;
        }
        .alert-close:hover {
          opacity: 1;
        }
        @keyframes slideDown {
          from { transform: translateY(-100%); }
          to { transform: translateY(0); }
        }
      `}</style>
    </div>
  );
}
