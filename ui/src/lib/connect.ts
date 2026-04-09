import { createConnectTransport } from "@connectrpc/connect-web";
import { API_BASE_URL } from "@/lib/constants";

/** ConnectRPC transport — talks to the Candela backend. */
export const transport = createConnectTransport({
  baseUrl: API_BASE_URL,
});
