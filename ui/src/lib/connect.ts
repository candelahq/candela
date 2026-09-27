import { createConnectTransport } from "@connectrpc/connect-web";
import { ConnectError, Code, type Interceptor } from "@connectrpc/connect";
import { API_BASE_URL } from "@/lib/constants";
import { firebaseAuth } from "@/lib/firebase";

/**
 * ConnectRPC auth interceptor — automatically injects Firebase ID tokens.
 * Handles token refresh failures gracefully: retries once, and throws a typed
 * Unauthenticated ConnectError instead of unhandled Firebase SDK exceptions.
 */
export const authInterceptor: Interceptor = (next) => async (req) => {
  const user = firebaseAuth?.currentUser;
  if (user) {
    let token: string;
    try {
      token = await user.getIdToken();
    } catch {
      // Retry once with forceRefresh in case of token cache invalidation or transient failure
      try {
        token = await user.getIdToken(true);
      } catch (err) {
        // If in browser and persistent auth failure occurs, redirect to login on expired/disabled session
        if (typeof window !== "undefined" && window.location.pathname !== "/login") {
          const isExpired =
            err &&
            typeof err === "object" &&
            "code" in err &&
            (err.code === "auth/user-token-expired" ||
              err.code === "auth/user-disabled" ||
              err.code === "auth/null-user");
          if (isExpired) {
            window.location.href = "/login";
          }
        }
        throw new ConnectError(
          `Authentication failed: unable to retrieve ID token (${err instanceof Error ? err.message : String(err)})`,
          Code.Unauthenticated,
          undefined,
          undefined,
          err,
        );
      }
    }
    req.header.set("Authorization", `Bearer ${token}`);
  }
  return next(req);
};

/** ConnectRPC transport — talks to the Candela backend.
 *  Automatically injects Firebase ID tokens for authenticated requests. */
export const transport = createConnectTransport({
  baseUrl: API_BASE_URL,
  interceptors: [authInterceptor],
});
