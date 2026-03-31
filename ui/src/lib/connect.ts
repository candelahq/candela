import { createConnectTransport } from "@connectrpc/connect-web";

/** ConnectRPC transport — talks to the Candela backend. */
export const transport = createConnectTransport({
  baseUrl: process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080",
});
