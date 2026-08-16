import { describe, expect, it } from "vitest";
import { loadTestOperationID } from "@/tests/load/idempotency";

describe("load-test idempotency keys", () => {
  it("are stable for an exact operation", () => {
    const first = loadTestOperationID("31868949811-1", 20, 1499);
    const replay = loadTestOperationID("31868949811-1", 20, 1499);

    expect(replay).toBe(first);
    expect(first).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-8[0-9a-f]{3}-[0-9a-f]{12}$/);
  });

  it("cannot collide across workflow runs or rerun attempts", () => {
    const keys = new Set([
      loadTestOperationID("31868949811-1", 1, 0),
      loadTestOperationID("31868949811-2", 1, 0),
      loadTestOperationID("31868949812-1", 1, 0),
      loadTestOperationID("31868949812-1", 2, 0),
      loadTestOperationID("31868949812-1", 1, 1),
    ]);

    expect(keys.size).toBe(5);
  });

  it("rejects values that cannot fit safely in the UUID", () => {
    expect(() => loadTestOperationID("run-without-digits", 1, 1)).toThrow(/runID/);
    expect(() => loadTestOperationID("1", 1000, 1)).toThrow(/virtualUserID/);
  });
});
