import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import {
  OnboardingWizard,
  ONBOARDING_COMPLETED_KEY,
  openOnboardingWizard,
} from "../OnboardingWizard";

const mockSignIn = vi.fn();
let mockUser: { email?: string; uid: string } | null = null;
let mockConfigured = false;

vi.mock("@/components/AuthProvider", () => ({
  useAuth: () => ({
    user: mockUser,
    configured: mockConfigured,
    signIn: mockSignIn,
    signOut: vi.fn(),
    getIdToken: vi.fn(),
    authError: null,
    clearAuthError: vi.fn(),
  }),
}));

describe("OnboardingWizard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    mockUser = null;
    mockConfigured = false;
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("opens automatically on first launch when not completed in localStorage", () => {
    render(<OnboardingWizard />);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText(/Authenticate Your Workspace/i)).toBeInTheDocument();
  });

  it("does not render when already completed in localStorage", () => {
    localStorage.setItem(ONBOARDING_COMPLETED_KEY, "true");
    const { container } = render(<OnboardingWizard />);
    expect(container.firstChild).toBeNull();
  });

  it("renders when forceOpen is true regardless of localStorage", () => {
    localStorage.setItem(ONBOARDING_COMPLETED_KEY, "true");
    render(<OnboardingWizard forceOpen={true} />);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("dismisses and persists completion flag when Skip is clicked", () => {
    render(<OnboardingWizard />);
    const skipButton = screen.getByRole("button", { name: /skip for now/i });
    fireEvent.click(skipButton);

    expect(localStorage.getItem(ONBOARDING_COMPLETED_KEY)).toBe("true");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("displays authenticated user email when signed in", () => {
    mockUser = { email: "alice@example.com", uid: "uid-alice" };
    render(<OnboardingWizard />);

    expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    expect(screen.getByText(/● Signed in/i)).toBeInTheDocument();
  });

  it("navigates through steps 1 to 4 using Next button", () => {
    render(<OnboardingWizard />);

    // Step 1: Auth
    expect(screen.getByText(/Authenticate Your Workspace/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /next →/i }));

    // Step 2: Server
    expect(screen.getByText(/Server Configuration/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /next →/i }));

    // Step 3: Proxy
    expect(screen.getByText(/Set Up Your Proxy Client/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /next →/i }));

    // Step 4: Verify
    expect(screen.getByText(/Verify Connectivity/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /finish setup ✓/i })).toBeInTheDocument();
  });

  it("switches snippet tabs and copies snippet in Step 3", async () => {
    const writeTextSpy = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, {
      clipboard: { writeText: writeTextSpy },
    });

    render(<OnboardingWizard />);
    // Jump to Step 3 (Proxy)
    fireEvent.click(screen.getByRole("button", { name: /proxy/i }));
    expect(screen.getByText(/Set Up Your Proxy Client/i)).toBeInTheDocument();

    // Default is Python SDK
    expect(screen.getByText(/import openai/i)).toBeInTheDocument();

    // Click cURL tab
    fireEvent.click(screen.getByRole("button", { name: /curl/i }));
    expect(screen.getAllByText(/curl/i).length).toBeGreaterThan(0);

    // Click Copy button
    const copyButton = screen.getByRole("button", { name: /copy/i });
    fireEvent.click(copyButton);

    await waitFor(() => {
      expect(screen.getByText(/copied!/i)).toBeInTheDocument();
    });
  });

  it("performs connectivity test and displays connected state in Step 4", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
    });
    vi.stubGlobal("fetch", mockFetch);

    render(<OnboardingWizard />);
    // Jump to Step 4 (Verify)
    fireEvent.click(screen.getByRole("button", { name: /verify/i }));

    const testButton = screen.getByRole("button", { name: /test connection/i });
    fireEvent.click(testButton);

    await waitFor(() => {
      expect(screen.getByText(/connected successfully!/i)).toBeInTheDocument();
    });

    // Clicking Finish Setup completes and closes wizard
    const finishButton = screen.getByRole("button", { name: /finish setup ✓/i });
    fireEvent.click(finishButton);

    expect(localStorage.getItem(ONBOARDING_COMPLETED_KEY)).toBe("true");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("opens via openOnboardingWizard() custom event", async () => {
    localStorage.setItem(ONBOARDING_COMPLETED_KEY, "true");
    render(<OnboardingWizard />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    act(() => {
      openOnboardingWizard();
    });

    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });
  });

  it("displays error state when connectivity probe fails", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 502,
    });
    vi.stubGlobal("fetch", mockFetch);

    render(<OnboardingWizard />);
    fireEvent.click(screen.getByRole("button", { name: /verify/i }));

    const testButton = screen.getByRole("button", { name: /test connection/i });
    fireEvent.click(testButton);

    await waitFor(() => {
      expect(screen.getByText(/server returned http 502/i)).toBeInTheDocument();
    });
  });
});
