"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

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

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="sidebar">
      <div className="sidebar-header">
        <div className="sidebar-logo-icon">🕯</div>
        <span className="sidebar-logo">Candela</span>
      </div>

      <nav className="sidebar-nav">
        {navItems.map((section) => (
          <div key={section.section}>
            <div className="nav-section-label">{section.section}</div>
            {section.items.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                className={`nav-item ${
                  pathname === item.href ? "active" : ""
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
