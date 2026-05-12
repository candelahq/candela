/**
 * Candela Enrichment SDK for TypeScript/JavaScript.
 *
 * Zero-dependency middleware for propagating tenant and job metadata
 * to Candela AI observability proxies via W3C Baggage headers.
 *
 * @example
 * ```ts
 * import { CandelaSession } from "@candelahq/sdk";
 *
 * const session = new CandelaSession({ tenantId: "acme", jobId: "run-42" });
 *
 * // With fetch
 * fetch(url, { headers: session.headers() });
 *
 * // With OpenAI SDK
 * const client = new OpenAI({
 *   defaultHeaders: session.headers(),
 * });
 * ```
 */

const ID_PATTERN = /^[a-zA-Z0-9\-._]{1,128}$/;

/** Validate a tenant or job ID. Throws if invalid. */
export function validateId(value: string, name: string = "id"): string {
  if (!ID_PATTERN.test(value)) {
    throw new Error(
      `candela: invalid ${name} "${value}" — must be 1-128 chars of [a-zA-Z0-9._-]`
    );
  }
  return value;
}

export interface EnrichmentOptions {
  tenantId?: string;
  jobId?: string;
}

/**
 * Inject Candela enrichment headers into a plain headers object.
 * Preserves existing Baggage entries.
 */
export function injectHeaders(
  headers: Record<string, string>,
  opts: EnrichmentOptions
): Record<string, string> {
  const parts: string[] = [];

  if (opts.tenantId) {
    validateId(opts.tenantId, "tenant_id");
    parts.push(`candela.tenant_id=${opts.tenantId}`);
    headers["X-Candela-Tenant-Id"] = opts.tenantId;
  }

  if (opts.jobId) {
    validateId(opts.jobId, "job_id");
    parts.push(`candela.job_id=${opts.jobId}`);
    headers["X-Candela-Job-Id"] = opts.jobId;
  }

  if (parts.length > 0) {
    const existing = headers["Baggage"] || "";
    const baggage = parts.join(",");
    headers["Baggage"] = existing ? `${existing},${baggage}` : baggage;
  }

  return headers;
}

/**
 * Reusable session that generates enrichment headers for all requests.
 */
export class CandelaSession {
  private readonly tenantId?: string;
  private readonly jobId?: string;

  constructor(opts: EnrichmentOptions) {
    if (opts.tenantId) validateId(opts.tenantId, "tenant_id");
    if (opts.jobId) validateId(opts.jobId, "job_id");
    this.tenantId = opts.tenantId;
    this.jobId = opts.jobId;
  }

  /** Return a fresh headers object with enrichment metadata. */
  headers(): Record<string, string> {
    return injectHeaders(
      {},
      { tenantId: this.tenantId, jobId: this.jobId }
    );
  }

  /**
   * Create a fetch wrapper that auto-injects enrichment headers.
   *
   * @example
   * ```ts
   * const session = new CandelaSession({ tenantId: "acme" });
   * const cfetch = session.wrapFetch(fetch);
   * const resp = await cfetch("https://api.openai.com/v1/chat/completions", {
   *   method: "POST",
   *   body: JSON.stringify(payload),
   * });
   * ```
   */
  wrapFetch(baseFetch: typeof fetch): typeof fetch {
    const enrichHeaders = this.headers();
    return (input: RequestInfo | URL, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      for (const [k, v] of Object.entries(enrichHeaders)) {
        const existing = headers.get(k);
        if (k === "Baggage" && existing) {
          headers.set(k, `${existing},${v}`);
        } else {
          headers.set(k, v);
        }
      }
      return baseFetch(input, { ...init, headers });
    };
  }
}
