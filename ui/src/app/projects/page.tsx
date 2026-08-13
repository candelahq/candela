"use client";

import { useEffect, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { transport } from "@/lib/connect";
import { ErrorBanner } from "@/components/ErrorBanner";
import { ProjectService } from "@/gen/candela/v1/project_service_pb";

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

  const [showModal, setShowModal] = useState(false);
  const [newName, setNewName] = useState("");
  const [newDesc, setNewDesc] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  const handleCloseModal = () => {
    setShowModal(false);
    setNewName("");
    setNewDesc("");
    setCreateError(null);
  };

  const handleCreateProject = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newName.trim()) return;
    setCreating(true);
    setCreateError(null);
    try {
      const client = createClient(ProjectService, transport);
      const res = await client.createProject({
        name: newName,
        description: newDesc,
      });
      const newP = res.project;
      if (newP) {
        setProjects((prev) => [
          ...prev,
          {
            id: newP.id,
            name: newP.name,
            description: newP.description,
            createdAt: newP.createdAt
              ? new Date(Number(newP.createdAt.seconds) * 1000).toLocaleDateString()
              : "—",
          }
        ]);
      }
      handleCloseModal();
    } catch (err: any) {
      setCreateError(err.message || "Failed to create project");
    } finally {
      setCreating(false);
    }
  };

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
        <button className="btn btn-primary" onClick={() => setShowModal(true)}>+ New Project</button>
      </header>

      <div className="main-body">
        {error && (
          <ErrorBanner title="Could not load projects">
            {error}
          </ErrorBanner>
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
                  <tr key={p.id} className="clickable-row">
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

      {showModal && (
        <div className="modal-overlay" onClick={handleCloseModal} role="dialog" aria-modal="true" aria-label="New Project">
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h3>New Project</h3>
              <button type="button" className="modal-close" onClick={handleCloseModal} aria-label="Close dialog">×</button>
            </div>
            <form onSubmit={handleCreateProject} className="modal-body">
              {createError && <ErrorBanner title="Error">{createError}</ErrorBanner>}
              <div className="form-group">
                <label>Name</label>
                <input
                  type="text"
                  required
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder="e.g. Production Backend"
                  autoFocus
                />
              </div>
              <div className="form-group">
                <label>Description</label>
                <textarea
                  value={newDesc}
                  onChange={(e) => setNewDesc(e.target.value)}
                  placeholder="Optional context about this project"
                  rows={3}
                />
              </div>
              <div className="modal-footer">
                <button type="button" className="btn" onClick={handleCloseModal}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={creating || !newName.trim()}>
                  {creating ? "Creating..." : "Create Project"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  );
}
