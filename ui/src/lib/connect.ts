import { createConnectTransport } from "@connectrpc/connect-web";
import { API_BASE_URL } from "@/lib/constants";
import { firebaseAuth } from "@/lib/firebase";

/** ConnectRPC transport — talks to the Candela backend.
 *  Automatically injects Firebase ID tokens for authenticated requests. */
export const transport = createConnectTransport({
  baseUrl: API_BASE_URL,
  interceptors: [
    (next) => async (req) => {
      const user = firebaseAuth?.currentUser;
      if (user) {
        const token = await user.getIdToken();
        req.header.set("Authorization", `Bearer ${token}`);
      }
      return next(req);
    },
  ],
});
