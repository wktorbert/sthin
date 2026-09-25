import { describe, expect, it } from "vitest";
import type { Initialize } from "@/serve/protocol";
import { checkSidecar } from "./sidecar-gate";

const init = (lean_version: string, schema_version = 1): Initialize => ({
  schema_version,
  lean_version,
  os: "darwin",
  platforms: [],
});

describe("checkSidecar", () => {
  it("accepts the same version and schema", () => {
    expect(checkSidecar("0.2.0", init("0.2.0"))).toEqual({ ok: true });
  });

  it("refuses a different Lean version and names both", () => {
    const r = checkSidecar("0.2.0", init("0.1.9"));
    expect(r.ok).toBe(false);
    expect(!r.ok && r.reason).toContain("0.2.0");
    expect(!r.ok && r.reason).toContain("0.1.9");
  });

  it("refuses a schema it does not speak", () => {
    const r = checkSidecar("0.2.0", init("0.2.0", 2));
    expect(r.ok).toBe(false);
    expect(!r.ok && r.reason).toContain("schema 2");
  });
});
