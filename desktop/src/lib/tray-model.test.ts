import { describe, expect, it } from "vitest";
import type { Device, PlatformStatus } from "@/serve/protocol";
import { parseTrayAction, trayModel } from "./tray-model";

const dev = (over: Partial<Device>): Device => ({
  id: "U1",
  name: "iPhone 17",
  platform: "ios",
  kind: "simulator",
  os_version: "26.5",
  state: "shutdown",
  slim: "stock",
  footprint_mb: null,
  warnings: [],
  ...over,
});
const both: PlatformStatus[] = [
  { platform: "ios", available: true },
  { platform: "android", available: true },
];

describe("trayModel", () => {
  it("lists virtual devices per platform with a booted marker, never physical ones", () => {
    const m = trayModel(both, [dev({ state: "booted", slim: "slim" }), dev({ id: "P1", name: "Phone", kind: "physical" }), dev({ id: "A1", name: "Pixel_7", platform: "android", kind: "emulator", os_version: "16" })], new Set());
    const texts = m.map((e) => ("text" in e ? e.text : e.type));
    expect(texts).toEqual(["iOS Simulators", "● iPhone 17 · 26.5", "Android Emulators", "○ Pixel_7 · 16", "separator", "Show Sthin", "Doctor", "Quit Sthin"]);
  });

  it("enables only the actions that make sense for the device's state", () => {
    const m = trayModel(both, [dev({ state: "booted", slim: "slim" }), dev({ id: "U2", name: "iPad", slim: "stock" })], new Set());
    const items = (id: string) => m.flatMap((e) => (e.type === "device" && e.id === id ? e.items : []));
    expect(items("U1").map((a) => [a.id, a.enabled])).toEqual([
      ["boot:U1", false],
      ["shutdown:U1", true],
      ["restore:U1", true],
    ]);
    expect(items("U2").map((a) => [a.id, a.enabled])).toEqual([
      ["boot:U2", true],
      ["shutdown:U2", false],
      ["restore:U2", false],
    ]);
  });

  it("offers only progress for a device that is busy", () => {
    const m = trayModel(both, [dev({})], new Set(["U1"]));
    const d = m.find((e) => e.type === "device");
    expect(d && d.type === "device" && d.items).toEqual([{ id: "progress:U1", text: "Working… show progress", enabled: true }]);
  });

  it("says when a platform has no devices and omits unavailable platforms", () => {
    const m = trayModel([{ platform: "ios", available: false, reason: "no macOS" }, both[1]], [], new Set());
    expect(m.slice(0, 2).map((e) => ("text" in e ? [e.type, e.text] : e.type))).toEqual([
      ["header", "Android Emulators"],
      ["empty", "No emulators"],
    ]);
  });
});

describe("parseTrayAction", () => {
  it("splits the verb from an id that may itself contain colons", () => {
    expect(parseTrayAction("boot:emulator-5554")).toEqual({ verb: "boot", id: "emulator-5554" });
    expect(parseTrayAction("restore:10.0.0.2:5555")).toEqual({ verb: "restore", id: "10.0.0.2:5555" });
    expect(parseTrayAction("quit")).toEqual({ verb: "quit", id: "" });
  });
});
