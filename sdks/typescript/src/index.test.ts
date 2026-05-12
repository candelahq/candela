import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { CandelaSession, injectHeaders, validateId } from "./index";

describe("validateId", () => {
  it("accepts valid IDs", () => {
    assert.equal(validateId("acme-corp"), "acme-corp");
    assert.equal(validateId("run_42"), "run_42");
    assert.equal(validateId("a.b.c"), "a.b.c");
  });

  it("rejects empty string", () => {
    assert.throws(() => validateId(""), /invalid/);
  });

  it("rejects spaces", () => {
    assert.throws(() => validateId("has spaces"), /invalid/);
  });

  it("rejects too-long IDs", () => {
    assert.throws(() => validateId("A".repeat(129)), /invalid/);
  });
});

describe("injectHeaders", () => {
  it("sets both IDs and baggage", () => {
    const h = injectHeaders({}, { tenantId: "acme", jobId: "run-1" });
    assert.equal(h["X-Candela-Tenant-Id"], "acme");
    assert.equal(h["X-Candela-Job-Id"], "run-1");
    assert.ok(h["Baggage"].includes("candela.tenant_id=acme"));
    assert.ok(h["Baggage"].includes("candela.job_id=run-1"));
  });

  it("preserves existing baggage", () => {
    const h = injectHeaders(
      { Baggage: "foo=bar" },
      { tenantId: "t1" }
    );
    assert.ok(h["Baggage"].startsWith("foo=bar,"));
    assert.ok(h["Baggage"].includes("candela.tenant_id=t1"));
  });

  it("is a no-op with no options", () => {
    const h = injectHeaders({}, {});
    assert.equal(h["Baggage"], undefined);
  });

  it("rejects invalid IDs", () => {
    assert.throws(() => injectHeaders({}, { tenantId: "bad id!" }));
  });
});

describe("CandelaSession", () => {
  it("generates headers", () => {
    const s = new CandelaSession({ tenantId: "acme", jobId: "j1" });
    const h = s.headers();
    assert.equal(h["X-Candela-Tenant-Id"], "acme");
    assert.equal(h["X-Candela-Job-Id"], "j1");
    assert.ok(h["Baggage"].includes("candela.tenant_id=acme"));
  });

  it("rejects invalid IDs on construction", () => {
    assert.throws(() => new CandelaSession({ tenantId: "bad spaces" }));
  });

  it("returns fresh headers each time", () => {
    const s = new CandelaSession({ tenantId: "t1" });
    const h1 = s.headers();
    const h2 = s.headers();
    assert.notEqual(h1, h2);
    assert.deepEqual(h1, h2);
  });
});
