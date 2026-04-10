"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useCurrentUser } from "@/hooks/useCurrentUser";

const navItems = [
  {
    section: "Observe",
    items: [
      { href: "/", label: "Dashboard", icon: "📊" },
      { href: "/traces", label: "Traces", icon: "🔍" },
      { href: "/costs", label: "Costs", icon: "💰" },
    ],
  },
  {
    section: "Manage",
    items: [
      { href: "/projects", label: "Projects", icon: "📁" },
      { href: "/settings", label: "Settings", icon: "⚙️" },
    ],
  },
];

const adminItems = {
  section: "Admin",
  items: [
    { href: "/admin/users", label: "Users", icon: "👥" },
    { href: "/admin/budgets", label: "Budgets", icon: "💳" },
    { href: "/admin/audit", label: "Audit Log", icon: "📋" },
  ],
};

export function Sidebar() {
  const pathname = usePathname();
  const { user, isAdmin, isLoading } = useCurrentUser();

  const sections = isAdmin ? [...navItems, adminItems] : navItems;

  return (
    <aside className="sidebar">
      <div className="sidebar-header">
        <div className="sidebar-logo-icon">🕯</div>
        <span className="sidebar-logo">Candela</span>
      </div>

      <nav className="sidebar-nav">
        {sections.map((section) => (
          <div key={section.section}>
            <div className="nav-section-label">{section.section}</div>
            {section.items.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                className={`nav-item ${
                  pathname === item.href ||
                  (item.href !== "/" && pathname.startsWith(item.href))
                    ? "active"
                    : ""
                }`}
              >
                <span className="nav-item-icon">{item.icon}</span>
                {item.label}
              </Link>
            ))}
          </div>
        ))}
      </nav>

      <div className="sidebar-footer">
        {!isLoading && user && (
          <div className="sidebar-user">
            <div className="sidebar-user-avatar">
              {user.email.charAt(0).toUpperCase()}
            </div>
            <div className="sidebar-user-info">
              <span className="sidebar-user-name">
                {user.displayName || user.email.split("@")[0]}
              </span>
              <span className="sidebar-user-role">
                {isAdmin ? "Admin" : "Developer"}
              </span>
            </div>
          </div>
        )}
        <div className="sidebar-env">
          <span className="env-dot" />
          <span>Development</span>
          <span className="mono" style={{ marginLeft: "auto", color: "var(--text-muted)" }}>
            localhost:8181
          </span>
        </div>
      </div>
    </aside>
  );
}
