"use client";

import { useEffect, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { transport } from "@/lib/connect";
import { ProjectService } from "@/gen/v1/project_service_pb";

interface Project {
  id: string;
  name: string;
  description: string;
  createdAt: string;
}

export default function ProjectsPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const client = createClient(ProjectService, transport);
    client
      .listProjects({})
      .then((res) => {
        const mapped = (res.projects || []).map((p) => ({
          id: p.id,
          name: p.name,
          description: p.description,
          createdAt: p.createdAt
            ? new Date(Number(p.createdAt.seconds) * 1000).toLocaleDateString()
            : "—",
        }));
        setProjects(mapped);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  return (
    <>
      <header className="main-header">
        <h1>Projects</h1>
        <button className="btn btn-primary">+ New Project</button>
      </header>

      <div className="main-body">
        {error && (
          <div
            className="card animate-in"
            style={{
              borderColor: "var(--error)",
              marginBottom: 24,
              background: "rgba(248,113,113,0.05)",
            }}
          >
            <div className="card-title" style={{ color: "var(--error)" }}>
              Could not load projects
            </div>
            <div style={{ fontSize: 13, color: "var(--text-secondary)" }}>
              {error}
            </div>
          </div>
        )}

        <div className="table-container animate-in">
          <div className="table-header">
            <span className="table-title">
              {loading ? "Loading..." : `${projects.length} projects`}
            </span>
          </div>

          {projects.length === 0 && !loading ? (
            <div className="empty-state">
              <div className="empty-state-icon">📁</div>
              <div className="empty-state-title">No projects yet</div>
              <div className="empty-state-desc">
                Create a project to organize your traces and API keys by team or
                application.
              </div>
            </div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Description</th>
                  <th>Created</th>
                  <th>ID</th>
                </tr>
              </thead>
              <tbody>
                {projects.map((p) => (
                  <tr key={p.id} style={{ cursor: "pointer" }}>
                    <td style={{ fontWeight: 500 }}>{p.name}</td>
                    <td style={{ color: "var(--text-secondary)" }}>
                      {p.description || "—"}
                    </td>
                    <td style={{ color: "var(--text-secondary)" }}>
                      {p.createdAt}
                    </td>
                    <td>
                      <span className="mono">{p.id.slice(0, 12)}…</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </>
  );
}
