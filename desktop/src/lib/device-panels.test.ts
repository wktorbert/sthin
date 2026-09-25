import { describe, expect, it } from "vitest";
import type { Device, PlatformStatus } from "@/serve/protocol";
import { groupPanels } from "./device-panels";

const dev = (id: string, platform: Device["platform"], kind: Device["kind"]): Device => ({
  id,
  name: id,
  platform,
  kind,
  os_version: "1",
  state: "shutdown",
  slim: "stock",
  footprint_mb: null,
  warnings: [],
});

const both: PlatformStatus[] = [
  { platform: "ios", available: true },
  { platform: "android", available: true },
];

describe("groupPanels", () => {
  it("puts simulators, emulators and physical devices in their own panels", () => {
    const g = groupPanels(both, [dev("U1", "ios", "simulator"), dev("P1", "ios", "physical"), dev("A1", "android", "emulator"), dev("S1", "android", "physical")]);
    expect(g.panels.map((p) => [p.key, p.devices.map((d) => d.id)])).toEqual([
      ["ios", ["U1"]],
      ["android", ["A1"]],
      ["physical", ["P1", "S1"]],
    ]);
    expect(g.notices).toEqual([]);
  });

  it("hides an unavailable platform's panel and shows its reason once", () => {
    const g = groupPanels(
      [{ platform: "ios", available: false, reason: "iOS simulators need macOS" }, { platform: "android", available: true }],
      [dev("A1", "android", "emulator")],
    );
    expect(g.panels.map((p) => p.key)).toEqual(["android", "physical"]);
    expect(g.notices).toEqual(["iOS simulators need macOS"]);
  });

  it("reports a platform that failed to list", () => {
    const g = groupPanels([{ platform: "ios", available: true, error: "simctl timed out" }, both[1]], []);
    expect(g.panels.map((p) => p.key)).toEqual(["ios", "android", "physical"]);
    expect(g.notices).toEqual(["iOS: simctl timed out"]);
  });
});
