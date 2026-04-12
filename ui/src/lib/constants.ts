/**
 * Base URL for the Candela backend API.
 * - Development: defaults to http://localhost:8181 (direct to Go server)
 * - Production: empty string = same origin (Next.js rewrites proxy to Go)
 */
export const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8181";

/**
 * Default project ID for single-project setup.
 * TODO: Multi-project support - replace with React Context when implementing project selection
 *
 * Currently hardcoded to "default" to match config.yaml proxy.project_id setting.
 *
 * Future evolution path for multi-project support:
 * 1. Create ProjectContext with React.createContext()
 * 2. Add project selector UI component in header
 * 3. Replace DEFAULT_PROJECT_ID references with useProject() hook
 * 4. Persist selected project in localStorage
 *
 * All API calls that include projectId parameter can be easily found by searching:
 * "DEFAULT_PROJECT_ID" in the codebase for future refactoring.
 */
export const DEFAULT_PROJECT_ID = "default";
